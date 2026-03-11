package main

import (
	"ashokshau/pytgdocs/internal/docs"
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strings"
)

type ConfigXML struct {
	Options []Option `xml:"option"`
}

type Option struct {
	ID      string `xml:"id,attr"`
	Content string `xml:",innerxml"`
}

func main() {
	out := flag.String("out", "docs.json", "Output JSON file path")
	flag.Parse()

	slog.Info("Generating docs...")

	configMap, err := parseConfig("https://raw.githubusercontent.com/pytgcalls/docsdata/master/config.xml")
	if err != nil {
		slog.Error("Failed to parse config", "error", err)
		os.Exit(1)
	}

	documentation, err := parseMap("https://raw.githubusercontent.com/pytgcalls/docsdata/master/map.json", configMap)
	if err != nil {
		slog.Error("Failed to parse map", "error", err)
		os.Exit(1)
	}

	err = saveDocs(documentation, *out)
	if err != nil {
		slog.Error("Failed to save docs", "error", err)
		os.Exit(1)
	}

	slog.Info("Done", "out", *out)
}

func readURL(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func parseConfig(url string) (map[string]string, error) {
	data, err := readURL(url)
	if err != nil {
		return nil, err
	}

	var configXML ConfigXML
	err = xml.Unmarshal(data, &configXML)
	if err != nil {
		return nil, err
	}

	rawOptions := make(map[string]string)
	for _, opt := range configXML.Options {
		rawOptions[opt.ID] = opt.Content
	}

	resolvedConfig := make(map[string]string)
	for id := range rawOptions {
		resolvedConfig[id] = resolveConfig(id, rawOptions, make(map[string]bool))
	}

	return resolvedConfig, nil
}

func resolveConfig(id string, rawOptions map[string]string, seen map[string]bool) string {
	if seen[id] {
		return ""
	}
	seen[id] = true
	defer delete(seen, id)

	content, ok := rawOptions[id]
	if !ok {
		return fmt.Sprintf("[UNRESOLVED:%s]", id)
	}

	return resolveContent(content, rawOptions, seen)
}

func resolveContent(content string, rawOptions map[string]string, seen map[string]bool) string {
	decoder := xml.NewDecoder(strings.NewReader("<root>" + content + "</root>"))
	var parts []string

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}

		switch t := token.(type) {
		case xml.CharData:
			parts = append(parts, string(t))
		case xml.StartElement:
			if t.Name.Local == "config" {
				var refID string
				for _, attr := range t.Attr {
					if attr.Name.Local == "id" {
						refID = attr.Value
						break
					}
				}
				if refID != "" {
					parts = append(parts, resolveConfig(refID, rawOptions, seen))
				}
			} else if t.Name.Local == "br" {
				parts = append(parts, "\n")
			}
		}
	}

	joined := strings.Join(parts, "")
	lines := strings.Split(joined, "\n")
	var cleanLines []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			cleanLines = append(cleanLines, trimmed)
		}
	}
	return strings.Join(cleanLines, "\n")
}

func parseMap(url string, configMap map[string]string) (docs.Documentation, error) {
	data, err := readURL(url)
	if err != nil {
		return nil, err
	}

	var rawMap map[string]string
	err = json.Unmarshal(data, &rawMap)
	if err != nil {
		return nil, err
	}

	documentation := make(docs.Documentation)
	for path, pageXML := range rawMap {
		documentation[path] = parsePage(path, pageXML, configMap)
	}

	return documentation, nil
}

type XMLNode struct {
	XMLName xml.Name
	Attrs   []xml.Attr `xml:",any,attr"`
	Content string     `xml:",innerxml"`
	Nodes   []XMLNode  `xml:",any"`
	Text    string     `xml:",chardata"`
}

func parsePage(path, pageXML string, configMap map[string]string) *docs.DocEntry {
	decoder := xml.NewDecoder(strings.NewReader(pageXML))
	var root XMLNode
	_ = decoder.Decode(&root)

	title := ""
	for _, node := range root.Nodes {
		if node.XMLName.Local == "h1" {
			title = strings.TrimSpace(collectText(node.Content))
			break
		}
	}

	lib := "Unknown"
	pathSuffix := path
	if strings.HasPrefix(path, "/NTgCalls/") {
		lib = "NTgCalls"
		pathSuffix = path[len("/NTgCalls/"):]
	} else if strings.HasPrefix(path, "/PyTgCalls/") {
		lib = "PyTgCalls"
		pathSuffix = path[len("/PyTgCalls/"):]
	}

	kind := "misc"
	if strings.Contains(path, "Available Enums") {
		kind = "enum"
	} else if strings.Contains(path, "Methods") {
		kind = "method"
	} else if strings.Contains(path, "Available Structs") {
		kind = "struct"
	} else if strings.Contains(path, "Available Types") || strings.Contains(path, "Advanced Types") {
		kind = "type"
	} else if strings.Contains(path, "Stream Descriptors") {
		kind = "descriptor"
	}

	description := ""
	if kind == "misc" {
		description = extractFullDescription(root, configMap)
	} else {
		description = extractDescription(root, configMap)
	}

	var example *docs.Example
	for _, node := range root.Nodes {
		if node.XMLName.Local == "syntax-highlight" {
			lang := "python"
			for _, attr := range node.Attrs {
				if attr.Name.Local == "language" {
					lang = attr.Value
				}
			}
			example = &docs.Example{
				Language: lang,
				Code:     dedent(collectText(node.Content)),
			}
			break
		}
	}

	details := parseDetails(root, configMap)

	if description == "" {
		description = findFirstDescription(root, configMap)
	}

	if strings.HasSuffix(pathSuffix, ".xml") {
		pathSuffix = pathSuffix[:len(pathSuffix)-4]
	}
	docURL := fmt.Sprintf("https://pytgcalls.github.io/%s/%s", lib, pathSuffix)

	return &docs.DocEntry{
		Title:       title,
		Lib:         lib,
		Kind:        kind,
		Description: cleanDescription(description),
		Example:     example,
		Details:     details,
		DocURL:      docURL,
	}
}

func extractFullDescription(node XMLNode, configMap map[string]string) string {
	var parts []string
	var process func(n XMLNode)
	process = func(n XMLNode) {
		if strings.TrimSpace(n.Text) != "" {
			parts = append(parts, strings.TrimSpace(n.Text))
		}
		for _, child := range n.Nodes {
			if child.XMLName.Local == "config" {
				id := getAttr(child, "id")
				if id != "" {
					parts = append(parts, configMap[id])
				}
			} else if child.XMLName.Local == "docs-ref" {
				link := getAttr(child, "link")
				text := strings.TrimSpace(collectText(child.Content))
				if link == "/PyTgCalls" {
					parts = append(parts, "Py-TgCalls")
				} else if text != "" {
					parts = append(parts, text)
				} else {
					parts = append(parts, filepathBase(link))
				}
			} else if child.XMLName.Local == "text" || child.XMLName.Local == "subtext" {
				process(child)
			} else if child.XMLName.Local == "br" {
				parts = append(parts, "\n")
			}
		}
	}
	process(node)
	return strings.Join(parts, " ")
}

func filepathBase(path string) string {
	parts := strings.Split(path, "/")
	return parts[len(parts)-1]
}

func extractDescription(node XMLNode, configMap map[string]string) string {
	for _, child := range node.Nodes {
		if child.XMLName.Local == "config" {
			id := getAttr(child, "id")
			if id != "" {
				return configMap[id]
			}
		} else if child.XMLName.Local == "text" {
			return strings.TrimSpace(collectText(child.Content))
		}
	}
	return ""
}

func findFirstDescription(node XMLNode, configMap map[string]string) string {
	for _, sub := range node.Nodes {
		if sub.XMLName.Local == "subtext" {
			for _, txt := range sub.Nodes {
				if txt.XMLName.Local == "text" {
					content := strings.TrimSpace(collectText(txt.Content))
					if content != "" {
						return content
					}
				}
				if txt.XMLName.Local == "config" {
					id := getAttr(txt, "id")
					if id != "" {
						return configMap[id]
					}
				}
			}
		}
	}
	return ""
}

func cleanDescription(s string) string {
	s = strings.TrimSpace(s)
	re := regexp.MustCompile(`\s+`)
	return re.ReplaceAllString(s, " ")
}

func parseDetails(root XMLNode, configMap map[string]string) docs.Details {
	var details docs.Details

	// Signature
	for _, node := range root.Nodes {
		if node.XMLName.Local == "category-title" {
			sig := strings.TrimSpace(collectText(node.Content))
			details.Signature = &sig
			break
		}
	}

	// Sections
	var sections []docs.Section
	var findCategories func(n XMLNode)
	findCategories = func(n XMLNode) {
		if n.XMLName.Local == "category" {
			sectionTitle := ""
			var rawItems []map[string]string
			for _, child := range n.Nodes {
				if child.XMLName.Local == "pg-title" {
					sectionTitle = strings.TrimSpace(collectText(child.Content))
				} else if child.XMLName.Local == "subtext" {
					for _, item := range child.Nodes {
						if item.XMLName.Local == "config" {
							id := getAttr(item, "id")
							rawItems = append(rawItems, map[string]string{"config_id": id, "resolved": configMap[id]})
						} else if item.XMLName.Local == "category-title" {
							rawItems = append(rawItems, map[string]string{"raw": strings.TrimSpace(collectText(item.Content))})
						} else if item.XMLName.Local == "text" {
							content := strings.TrimSpace(collectText(item.Content))
							if content != "" {
								rawItems = append(rawItems, map[string]string{"text": content})
							}
						} else if item.XMLName.Local == "subtext" {
							content := strings.TrimSpace(collectText(item.Content))
							if content != "" {
								rawItems = append(rawItems, map[string]string{"sub_text": content})
							}
						}
					}
				}
			}
			if items := normalizeItems(rawItems); len(items) > 0 {
				sections = append(sections, docs.Section{Title: sectionTitle, Items: items})
			}
		}
		for _, child := range n.Nodes {
			findCategories(child)
		}
	}
	findCategories(root)
	details.Sections = sections

	// Members, Properties, Parameters (from <subtext> blocks with <pg-title>)
	for _, sub := range root.Nodes {
		if sub.XMLName.Local == "subtext" {
			pgTitle := ""
			for _, child := range sub.Nodes {
				if child.XMLName.Local == "pg-title" {
					pgTitle = strings.ToUpper(collectText(child.Content))
					break
				}
			}

			if pgTitle != "" {
				if strings.Contains(pgTitle, "PARAMETERS") {
					for _, inner := range sub.Nodes {
						if inner.XMLName.Local == "subtext" {
							details.Parameters = append(details.Parameters, parseItemBlock(inner, configMap)...)
						}
					}
				} else if strings.Contains(pgTitle, "ENUMERATION MEMBERS") {
					for _, inner := range sub.Nodes {
						if inner.XMLName.Local == "subtext" {
							details.Members = append(details.Members, parseMemberBlock(inner, configMap)...)
						}
					}
				} else if strings.Contains(pgTitle, "PROPERTIES") {
					for _, inner := range sub.Nodes {
						if inner.XMLName.Local == "subtext" {
							details.Properties = append(details.Properties, parsePropertyBlock(inner, configMap)...)
						}
					}
				}
			}
		}
	}

	return details
}

func collectText(innerXML string) string {
	decoder := xml.NewDecoder(strings.NewReader("<root>" + innerXML + "</root>"))
	var sb strings.Builder
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		switch t := token.(type) {
		case xml.CharData:
			sb.WriteString(string(t))
		}
	}
	return sb.String()
}

func getAttr(n XMLNode, name string) string {
	for _, attr := range n.Attrs {
		if attr.Name.Local == name {
			return attr.Value
		}
	}
	return ""
}

func normalizeItems(rawItems []map[string]string) []docs.DocItem {
	var result []docs.DocItem
	for _, item := range rawItems {
		if resolved, ok := item["resolved"]; ok {
			text := strings.TrimSpace(resolved)

			// Try to split resolved text into multiple items if it contains multiple "exception " prefixes
			if strings.Contains(text, "\nexception ") {
				parts := strings.Split(text, "\nexception ")
				for i, p := range parts {
					content := p
					if i > 0 {
						content = "exception " + p
					}
					content = strings.TrimSpace(content)
					if content == "" {
						continue
					}

					lines := strings.SplitN(content, "\n", 2)
					firstLine := lines[0]
					desc := ""
					if len(lines) > 1 {
						desc = strings.TrimSpace(lines[1])
					}

					configID := item["config_id"]
					if strings.Contains(firstLine, ":") {
						parts := strings.SplitN(firstLine, ":", 2)
						name := strings.TrimSpace(parts[0])
						typ := strings.TrimSpace(parts[1])
						result = append(result, docs.DocItem{Name: name, Type: &typ, Description: desc, SourceConfig: &configID})
					} else {
						result = append(result, docs.DocItem{Name: firstLine, Description: desc, SourceConfig: &configID})
					}
				}
			} else {
				lines := strings.SplitN(text, "\n", 2)
				firstLine := lines[0]
				desc := ""
				if len(lines) > 1 {
					desc = strings.TrimSpace(lines[1])
				}
				configID := item["config_id"]
				if strings.Contains(firstLine, ":") {
					parts := strings.SplitN(firstLine, ":", 2)
					name := strings.TrimSpace(parts[0])
					typ := strings.TrimSpace(parts[1])
					result = append(result, docs.DocItem{Name: name, Type: &typ, Description: desc, SourceConfig: &configID})
				} else if len(result) > 0 && !strings.HasPrefix(firstLine, "exception ") {
					result[len(result)-1].Description += "\n" + text
				} else {
					result = append(result, docs.DocItem{Name: firstLine, Description: desc, SourceConfig: &configID})
				}
			}
		} else if raw, ok := item["raw"]; ok {
			rawText := raw
			var name string
			var typ *string
			if strings.Contains(rawText, ":") {
				parts := strings.SplitN(rawText, ":", 2)
				name = strings.TrimSpace(parts[0])
				t := strings.TrimSpace(parts[1])
				typ = &t
			} else {
				name = strings.TrimSpace(rawText)
			}
			result = append(result, docs.DocItem{Name: name, Type: typ})
		} else if text, ok := item["text"]; ok {
			if len(result) > 0 {
				result[len(result)-1].Description += "\n" + strings.TrimSpace(text)
			} else {
				result = append(result, docs.DocItem{Name: "", Description: strings.TrimSpace(text)})
			}
		} else if subText, ok := item["sub_text"]; ok {
			if len(result) > 0 {
				result[len(result)-1].Description += "\n" + strings.TrimSpace(subText)
			}
		}
	}
	return result
}

func parseMemberBlock(block XMLNode, configMap map[string]string) []docs.DocItem {
	var items []docs.DocItem
	var current *docs.DocItem

	for _, child := range block.Nodes {
		if child.XMLName.Local == "category-title" {
			raw := strings.TrimSpace(collectText(child.Content))
			var item docs.DocItem
			if strings.Contains(raw, "=") {
				parts := strings.SplitN(raw, "=", 2)
				val := strings.TrimSpace(parts[1])
				item = docs.DocItem{Name: strings.TrimSpace(parts[0]), Value: &val}
			} else {
				item = docs.DocItem{Name: raw}
			}
			items = append(items, item)
			current = &items[len(items)-1]
		} else if child.XMLName.Local == "subtext" || child.XMLName.Local == "text" {
			desc := strings.TrimSpace(collectText(child.Content))
			if desc != "" {
				if current != nil {
					if current.Description != "" {
						current.Description += "\n" + desc
					} else {
						current.Description = desc
					}
				}
			}
		} else if child.XMLName.Local == "config" {
			id := getAttr(child, "id")
			text := strings.TrimSpace(configMap[id])
			if text != "" {
				if current != nil {
					if current.Description != "" {
						current.Description += "\n" + text
					} else {
						current.Description = text
					}
				}
			}
		}
	}
	return items
}

func parsePropertyBlock(block XMLNode, configMap map[string]string) []docs.DocItem {
	var items []docs.DocItem
	var current *docs.DocItem

	for _, child := range block.Nodes {
		if child.XMLName.Local == "category-title" {
			raw := strings.TrimSpace(collectText(child.Content))
			name := raw
			var typeText *string
			if strings.Contains(raw, "->") {
				parts := strings.SplitN(raw, "->", 2)
				name = strings.TrimSpace(parts[0])
				t := strings.TrimSpace(parts[1])
				typeText = &t
			} else {
				// Search for docs-ref inside category-title
				for _, gc := range child.Nodes {
					if gc.XMLName.Local == "docs-ref" {
						t := strings.TrimSpace(collectText(gc.Content))
						typeText = &t
						break
					}
				}
			}
			items = append(items, docs.DocItem{Name: strings.TrimSpace(name), Type: typeText})
			current = &items[len(items)-1]
		} else if child.XMLName.Local == "subtext" || child.XMLName.Local == "text" {
			desc := strings.TrimSpace(collectText(child.Content))
			if desc != "" {
				if current != nil {
					if current.Description != "" {
						current.Description += "\n" + desc
					} else {
						current.Description = desc
					}
				}
			}
		} else if child.XMLName.Local == "config" {
			id := getAttr(child, "id")
			text := strings.TrimSpace(configMap[id])
			if text != "" {
				if current != nil {
					if current.Description != "" {
						current.Description += "\n" + text
					} else {
						current.Description = text
					}
				}
			}
		}
	}
	return items
}

func parseItemBlock(inner XMLNode, configMap map[string]string) []docs.DocItem {
	var items []docs.DocItem
	var current *docs.DocItem

	for _, child := range inner.Nodes {
		if child.XMLName.Local == "category-title" || child.XMLName.Local == "config" {
			var rawItems []map[string]string
			if child.XMLName.Local == "category-title" {
				rawItems = append(rawItems, map[string]string{"raw": strings.TrimSpace(collectText(child.Content))})
			} else {
				id := getAttr(child, "id")
				rawItems = append(rawItems, map[string]string{"config_id": id, "resolved": configMap[id]})
			}

			newItems := normalizeItems(rawItems)
			items = append(items, newItems...)
			if len(items) > 0 {
				current = &items[len(items)-1]
			}
		} else if child.XMLName.Local == "subtext" || child.XMLName.Local == "text" {
			desc := strings.TrimSpace(collectText(child.Content))
			if desc != "" {
				if current != nil {
					if current.Description != "" {
						current.Description += "\n" + desc
					} else {
						current.Description = desc
					}
				}
			}
		}
	}
	return items
}

func dedent(s string) string {
	lines := strings.Split(s, "\n")
	minIndent := -1
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		indent := 0
		for _, r := range line {
			if r == ' ' {
				indent++
			} else {
				break
			}
		}
		if minIndent == -1 || indent < minIndent {
			minIndent = indent
		}
	}

	if minIndent <= 0 {
		return strings.TrimSpace(s)
	}

	var result []string
	for _, line := range lines {
		if len(line) >= minIndent {
			result = append(result, line[minIndent:])
		} else {
			result = append(result, "")
		}
	}
	return strings.TrimSpace(strings.Join(result, "\n"))
}

func saveDocs(documentation docs.Documentation, out string) error {
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()

	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	return encoder.Encode(documentation)
}
