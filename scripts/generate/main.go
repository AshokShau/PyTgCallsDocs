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
	configRe := regexp.MustCompile(`<config\s+id="([^"]+)"\s*/>`)
	for {
		found := false
		content = configRe.ReplaceAllStringFunc(content, func(m string) string {
			match := configRe.FindStringSubmatch(m)
			if len(match) > 1 {
				found = true
				return resolveConfig(match[1], rawOptions, seen)
			}
			return m
		})
		if !found {
			break
		}
	}
	return content
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
		if path == "/PyTgCalls/Examples.xml" {
			examples := parseExamplesPage(path, pageXML)
			for _, ex := range examples {
				documentation[ex.Path] = ex
			}
		} else if path == "/PyTgCalls/Changelogs.xml" {
			changelogs := parseChangelogsPage(path, pageXML, configMap)
			for _, cl := range changelogs {
				documentation[cl.Path] = cl
			}
		}
		documentation[path] = parsePage(path, pageXML, configMap)
	}

	return documentation, nil
}

func parseChangelogsPage(path, pageXML string, configMap map[string]string) []*docs.DocEntry {
	var entries []*docs.DocEntry

	categoryRegex := regexp.MustCompile(`(?s)<category>(.*?)</category>`)
	bannerRegex := regexp.MustCompile(`(?s)<banner\s+(.*?)/>`)
	subtextRegex := regexp.MustCompile(`(?s)<subtext>(.*?)</subtext>`)
	attrRegex := regexp.MustCompile(`(\w+)="([^"]*)"`)

	categories := categoryRegex.FindAllStringSubmatch(pageXML, -1)
	for _, catMatch := range categories {
		catContent := catMatch[1]

		bannerMatch := bannerRegex.FindStringSubmatch(catContent)
		if len(bannerMatch) < 2 {
			continue
		}

		attrs := make(map[string]string)
		attrMatches := attrRegex.FindAllStringSubmatch(bannerMatch[1], -1)
		for _, am := range attrMatches {
			attrs[am[1]] = am[2]
		}

		version := attrs["version"]
		if version == "" {
			version = "Unknown"
		}

		title := attrs["bigtitle"]
		if title == "" {
			title = "Changelog"
		}

		description := attrs["description"]

		var contentParts []string
		if description != "" {
			contentParts = append(contentParts, fmt.Sprintf("<b>%s</b>", description))
		}

		subtexts := subtextRegex.FindAllStringSubmatch(catContent, -1)
		for _, stMatch := range subtexts {
			contentParts = append(contentParts, collectFormattedText(stMatch[1], configMap))
		}

		fullContent := strings.Join(contentParts, "\n\n")

		entries = append(entries, &docs.DocEntry{
			Path:        fmt.Sprintf("/PyTgCalls/Changelogs/%s.xml", version),
			Title:       fmt.Sprintf("Changelog %s: %s", version, title),
			Lib:         "PyTgCalls",
			Kind:        "misc",
			Description: strings.TrimSpace(fullContent),
			DocURL:      fmt.Sprintf("https://pytgcalls.github.io/PyTgCalls/Changelogs#%s", strings.ReplaceAll(version, ".", "")),
		})
	}

	return entries
}

func collectFormattedText(innerXML string, configMap map[string]string) string {
	if !strings.Contains(innerXML, "<") && !strings.Contains(innerXML, "&") {
		re := regexp.MustCompile(`\s+`)
		return strings.TrimSpace(re.ReplaceAllString(innerXML, " "))
	}
	decoder := xml.NewDecoder(strings.NewReader("<root>" + innerXML + "</root>"))
	var sb strings.Builder

	var inCode int
	var traverse func()
	traverse = func() {
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
				text := string(t)
				if inCode == 0 {
					text = strings.ReplaceAll(text, "\n", " ")
					re := regexp.MustCompile(`\s+`)
					text = re.ReplaceAllString(text, " ")
				}
				sb.WriteString(text)
			case xml.StartElement:
				switch t.Name.Local {
				case "b", "strong":
					sb.WriteString("<b>")
					traverse()
					sb.WriteString("</b>")
				case "i", "em":
					sb.WriteString("<i>")
					traverse()
					sb.WriteString("</i>")
				case "u", "ins":
					sb.WriteString("<u>")
					traverse()
					sb.WriteString("</u>")
				case "a":
					var href string
					for _, attr := range t.Attr {
						if attr.Name.Local == "href" {
							href = attr.Value
						}
					}
					sb.WriteString(fmt.Sprintf("<a href=\"%s\">", href))
					traverse()
					sb.WriteString("</a>")
				case "code":
					sb.WriteString("<code>")
					inCode++
					traverse()
					inCode--
					sb.WriteString("</code>")
				case "br":
					sb.WriteString("\n")
					traverse()
				case "list":
					sb.WriteString("\n")
					traverse()
				case "item":
					sb.WriteString("\n- ")
					traverse()
					sb.WriteString("\n")
				case "h3":
					sb.WriteString("\n<b>")
					traverse()
					sb.WriteString("</b>\n")
				case "alert":
					sb.WriteString("\n<blockquote>")
					traverse()
					sb.WriteString("</blockquote>\n")
				case "config":
					var id string
					for _, attr := range t.Attr {
						if attr.Name.Local == "id" {
							id = attr.Value
						}
					}
					if val, ok := configMap[id]; ok {
						sb.WriteString(collectFormattedText(val, configMap))
					}
					traverse()
				case "syntax-highlight":
					sb.WriteString("\n<pre><code>")
					inCode++
					traverse()
					inCode--
					sb.WriteString("</code></pre>\n")
				case "multisyntax":
					isBlame := false
					for _, attr := range t.Attr {
						if attr.Name.Local == "as-blame" && attr.Value == "true" {
							isBlame = true
							break
						}
					}
					if isBlame {
						var codes []string
						for {
							innerToken, _ := decoder.Token()
							if innerToken == nil {
								break
							}
							if se, ok := innerToken.(xml.StartElement); ok && se.Name.Local == "syntax-highlight" {
								codes = append(codes, collectTokenContent(decoder))
							} else if ee, ok := innerToken.(xml.EndElement); ok && ee.Name.Local == "multisyntax" {
								break
							}
						}
						if len(codes) >= 2 {
							sb.WriteString("\n<pre><code>")
							sb.WriteString(generateDiff(codes[0], codes[1]))
							sb.WriteString("</code></pre>\n")
						}
					} else {
						traverse()
					}
				case "docs-ref":
					var link string
					for _, attr := range t.Attr {
						if attr.Name.Local == "link" {
							link = attr.Value
						}
					}
					if link != "" {
						if strings.HasPrefix(link, "/") {
							link = "https://pytgcalls.github.io" + link
						}
						sb.WriteString(fmt.Sprintf("<a href=\"%s\">", link))
						traverse()
						sb.WriteString("</a>")
					} else {
						traverse()
					}
				case "shi", "shi-inline":
					sb.WriteString("<code>")
					inCode++
					traverse()
					inCode--
					sb.WriteString("</code>")
				case "sb":
					sb.WriteString("<b>")
					traverse()
					sb.WriteString("</b>")
				case "ref", "text", "subtext":
					traverse()
				default:
					traverse()
				}
			case xml.EndElement:
				return
			}
		}
	}

	traverse()
	return cleanResult(sb.String())
}

func cleanResult(s string) string {
	preRegex := regexp.MustCompile(`(?s)<pre><code>(.*?)</code></pre>`)
	codeRegex := regexp.MustCompile(`(?s)<code>(.*?)</code>`)

	placeholders := make(map[string]string)
	counter := 0

	processed := preRegex.ReplaceAllStringFunc(s, func(m string) string {
		placeholder := fmt.Sprintf("___PRE_CODE_%d___", counter)
		placeholders[placeholder] = m
		counter++
		return "\n" + placeholder + "\n"
	})

	processed = codeRegex.ReplaceAllStringFunc(processed, func(m string) string {
		placeholder := fmt.Sprintf("___CODE_%d___", counter)
		placeholders[placeholder] = m
		counter++
		return placeholder
	})

	lines := strings.Split(processed, "\n")
	var result []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "___PRE_CODE_") || strings.HasPrefix(trimmed, "___CODE_") {
			if val, ok := placeholders[trimmed]; ok {
				if strings.HasPrefix(trimmed, "___PRE_CODE_") {
					content := val[len("<pre><code>") : len(val)-len("</code></pre>")]
					result = append(result, "<pre><code>"+dedent(content)+"</code></pre>")
				} else {
					result = append(result, val)
				}
				continue
			}
		}

		if trimmed != "" {
			result = append(result, trimmed)
		} else if len(result) > 0 && result[len(result)-1] != "" {
			result = append(result, "")
		}
	}

	final := strings.TrimSpace(strings.Join(result, "\n"))
	for ph, val := range placeholders {
		if !strings.HasPrefix(ph, "___PRE_CODE_") {
			final = strings.ReplaceAll(final, ph, val)
		}
	}

	return final
}

func parseExamplesPage(path, pageXML string) []*docs.DocEntry {
	var entries []*docs.DocEntry

	tableRegex := regexp.MustCompile(`(?s)<table>(.*?)</table>`)
	itemRegex := regexp.MustCompile(`(?s)<item>(.*?)</item>`)
	columnRegex := regexp.MustCompile(`(?s)<column>(.*?)</column>`)
	refShiRegex := regexp.MustCompile(`<ref-shi url="(.*?)">(.*?)</ref-shi>`)

	tables := tableRegex.FindAllString(pageXML, -1)
	for _, table := range tables {
		items := itemRegex.FindAllString(table, -1)
		for _, item := range items {
			columns := columnRegex.FindAllStringSubmatch(item, -1)
			if len(columns) >= 2 {
				nameMatch := refShiRegex.FindStringSubmatch(columns[0][1])
				if len(nameMatch) >= 3 {
					url := nameMatch[1]
					title := nameMatch[2]
					description := strings.TrimSpace(columns[1][1])

					fullURL := "https://github.com/pytgcalls/pytgcalls/tree/master/" + url
					filename := filepathBase(url)

					entries = append(entries, &docs.DocEntry{
						Path:        fmt.Sprintf("/PyTgCalls/Examples/%s.xml", title),
						Title:       fmt.Sprintf("Example: %s (%s)", title, filename),
						Lib:         "PyTgCalls",
						Kind:        "example",
						Description: description,
						DocURL:      fullURL,
					})
				}
			}
		}
	}

	return entries
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

	for i := range details.Parameters {
		details.Parameters[i].Name = collectFormattedText(details.Parameters[i].Name, configMap)
		if details.Parameters[i].Type != nil {
			t := collectFormattedText(*details.Parameters[i].Type, configMap)
			details.Parameters[i].Type = &t
		}
		details.Parameters[i].Description = collectFormattedText(details.Parameters[i].Description, configMap)
	}
	for i := range details.Sections {
		for j := range details.Sections[i].Items {
			details.Sections[i].Items[j].Name = collectFormattedText(details.Sections[i].Items[j].Name, configMap)
			if details.Sections[i].Items[j].Type != nil {
				t := collectFormattedText(*details.Sections[i].Items[j].Type, configMap)
				details.Sections[i].Items[j].Type = &t
			}
			details.Sections[i].Items[j].Description = collectFormattedText(details.Sections[i].Items[j].Description, configMap)
		}
	}
	for i := range details.Members {
		details.Members[i].Name = collectFormattedText(details.Members[i].Name, configMap)
		details.Members[i].Description = collectFormattedText(details.Members[i].Description, configMap)
	}
	for i := range details.Properties {
		details.Properties[i].Name = collectFormattedText(details.Properties[i].Name, configMap)
		details.Properties[i].Description = collectFormattedText(details.Properties[i].Description, configMap)
	}
	for i := range details.Methods {
		details.Methods[i].Name = collectFormattedText(details.Methods[i].Name, configMap)
		details.Methods[i].Description = collectFormattedText(details.Methods[i].Description, configMap)
	}

	// Handle tables in details
	tableRegex := regexp.MustCompile(`(?s)<table>(.*?)</table>`)
	itemRegex := regexp.MustCompile(`(?s)<item>(.*?)</item>`)
	columnRegex := regexp.MustCompile(`(?s)<column>(.*?)</column>`)
	refShiRegex := regexp.MustCompile(`<ref-shi url="(.*?)">(.*?)</ref-shi>`)

	tables := tableRegex.FindAllString(pageXML, -1)
	for _, table := range tables {
		var section docs.Section
		section.Title = "LIST"

		items := itemRegex.FindAllString(table, -1)
		for _, item := range items {
			columns := columnRegex.FindAllStringSubmatch(item, -1)
			if len(columns) >= 2 {
				col1 := columns[0][1]
				col2 := columns[1][1]

				name := col1
				var url *string
				nameMatch := refShiRegex.FindStringSubmatch(col1)
				if len(nameMatch) >= 3 {
					u := nameMatch[1]
					if !strings.HasPrefix(u, "http") {
						u = "https://github.com/pytgcalls/pytgcalls/tree/master/" + u
					}
					url = &u
					name = nameMatch[2]
				}

				section.Items = append(section.Items, docs.DocItem{
					Name:        name,
					Description: strings.TrimSpace(col2),
					URL:         url,
				})
			}
		}
		if len(section.Items) > 0 {
			details.Sections = append(details.Sections, section)
		}
	}

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
				return collectFormattedText(configMap[id], configMap)
			}
		} else if child.XMLName.Local == "text" {
			return collectFormattedText(child.Content, configMap)
		}
	}
	return ""
}

func findFirstDescription(node XMLNode, configMap map[string]string) string {
	for _, sub := range node.Nodes {
		if sub.XMLName.Local == "subtext" {
			for _, txt := range sub.Nodes {
				if txt.XMLName.Local == "text" {
					content := collectFormattedText(txt.Content, configMap)
					if content != "" {
						return content
					}
				}
				if txt.XMLName.Local == "config" {
					id := getAttr(txt, "id")
					if id != "" {
						return collectFormattedText(configMap[id], configMap)
					}
				}
			}
		}
	}
	return ""
}

func cleanDescription(s string) string {
	s = strings.TrimSpace(s)
	// Don't collapse whitespace if it contains HTML tags that might need it (like blockquote or pre)
	if strings.Contains(s, "<") {
		return s
	}
	re := regexp.MustCompile(`\s+`)
	return re.ReplaceAllString(s, " ")
}

func parseDetails(root XMLNode, configMap map[string]string) docs.Details {
	var details docs.Details

	// Signature
	for _, node := range root.Nodes {
		if node.XMLName.Local == "category-title" {
			sig := collectFormattedText(node.Content, configMap)
			details.Signature = &sig
			break
		}
	}

	// Sections
	var sections []docs.Section
	var findCategories func(n XMLNode)
	findCategories = func(n XMLNode) {
		if n.XMLName.Local == "category" {
			currentSectionTitle := ""
			var rawItems []map[string]string

			var processNodes func(nodes []XMLNode)
			processNodes = func(nodes []XMLNode) {
				for _, child := range nodes {
					if child.XMLName.Local == "pg-title" {
						// If we already have items for a previous title, save them
						if len(rawItems) > 0 {
							if items := normalizeItems(rawItems, configMap); len(items) > 0 {
								sections = append(sections, docs.Section{Title: currentSectionTitle, Items: items})
							}
							rawItems = nil
						}
						currentSectionTitle = strings.TrimSpace(collectText(child.Content))
					} else if child.XMLName.Local == "config" {
						id := getAttr(child, "id")
						rawItems = append(rawItems, map[string]string{"config_id": id, "resolved": configMap[id]})
					} else if child.XMLName.Local == "category-title" {
						rawItems = append(rawItems, map[string]string{"raw": "<category-title>" + child.Content + "</category-title>"})
					} else if child.XMLName.Local == "text" || child.XMLName.Local == "alert" || child.XMLName.Local == "br" || child.XMLName.Local == "list" || child.XMLName.Local == "item" {
						tag := child.XMLName.Local
						rawItems = append(rawItems, map[string]string{"text": "<" + tag + ">" + child.Content + "</" + tag + ">"})
					} else if child.XMLName.Local == "subtext" {
						processNodes(child.Nodes)
					}
				}
			}

			processNodes(n.Nodes)
			if len(rawItems) > 0 {
				if items := normalizeItems(rawItems, configMap); len(items) > 0 {
					sections = append(sections, docs.Section{Title: currentSectionTitle, Items: items})
				}
			}
			return // Don't recurse into this category's children to avoid double processing
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
			var currentPGTitle string
			var rawItems []map[string]string

			var processSubtextNodes func(nodes []XMLNode)
			processSubtextNodes = func(nodes []XMLNode) {
				for _, child := range nodes {
					if child.XMLName.Local == "pg-title" {
						if len(rawItems) > 0 {
							appendItemsByTitle(&details, currentPGTitle, normalizeItems(rawItems, configMap))
							rawItems = nil
						}
						currentPGTitle = strings.ToUpper(collectText(child.Content))
					} else if child.XMLName.Local == "config" {
						id := getAttr(child, "id")
						rawItems = append(rawItems, map[string]string{"config_id": id, "resolved": configMap[id]})
					} else if child.XMLName.Local == "category-title" {
						rawItems = append(rawItems, map[string]string{"raw": "<category-title>" + child.Content + "</category-title>"})
					} else if child.XMLName.Local == "text" || child.XMLName.Local == "alert" || child.XMLName.Local == "br" || child.XMLName.Local == "list" || child.XMLName.Local == "item" {
						tag := child.XMLName.Local
						rawItems = append(rawItems, map[string]string{"text": "<" + tag + ">" + child.Content + "</" + tag + ">"})
					} else if child.XMLName.Local == "subtext" {
						processSubtextNodes(child.Nodes)
					}
				}
			}

			processSubtextNodes(sub.Nodes)
			if len(rawItems) > 0 {
				appendItemsByTitle(&details, currentPGTitle, normalizeItems(rawItems, configMap))
			}
		}
	}

	return details
}

func appendItemsByTitle(d *docs.Details, title string, items []docs.DocItem) {
	if strings.Contains(title, "PARAMETERS") {
		d.Parameters = append(d.Parameters, items...)
	} else if strings.Contains(title, "ENUMERATION MEMBERS") {
		d.Members = append(d.Members, items...)
	} else if strings.Contains(title, "PROPERTIES") {
		d.Properties = append(d.Properties, items...)
	} else if strings.Contains(title, "METHODS") {
		d.Methods = append(d.Methods, items...)
	}
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

func normalizeItems(rawItems []map[string]string, configMap map[string]string) []docs.DocItem {
	var result []docs.DocItem
	for _, item := range rawItems {
		configID := item["config_id"]
		if resolved, ok := item["resolved"]; ok {
			// Detect multiple items in a single config block
			if strings.Contains(resolved, "<category-title>") {
				parts := strings.Split(resolved, "<category-title>")
				for _, p := range parts {
					if strings.TrimSpace(p) == "" {
						continue
					}
					p = "<category-title>" + p
					// Handle cases like <category-title>exception <ref>...</ref></category-title>
					itemRe := regexp.MustCompile(`(?s)<category-title>(.*?)(?:<ref>|<docs-ref[^>]*>)(.*?)(?:</ref>|</docs-ref>)(?:\s*:\s*(.*?))?\s*</category-title>(.*)`)
					match := itemRe.FindStringSubmatch(p)
					if match != nil {
						prefix := strings.TrimSpace(collectFormattedText(match[1], configMap))
						name := strings.TrimSpace(collectFormattedText(match[2], configMap))
						if prefix != "" {
							name = prefix + " " + name
						}
						var typ *string
						typeContent := strings.TrimSpace(match[3])
						if strings.HasPrefix(typeContent, "<shi>") && strings.HasSuffix(typeContent, "</shi>") {
							typeContent = typeContent[len("<shi>") : len(typeContent)-len("</shi>")]
						}
						typeText := strings.TrimSpace(collectFormattedText(typeContent, configMap))
						if typeText != "" {
							typ = &typeText
						}
						desc := match[4] // Preserve tags for later processing
						result = append(result, docs.DocItem{Name: name, Type: typ, Description: strings.TrimSpace(desc), SourceConfig: &configID})
					} else {
						// Fallback if regex fails but it's a category-title part
						desc := collectFormattedText(p, configMap)
						if len(result) > 0 {
							result[len(result)-1].Description = strings.TrimSpace(result[len(result)-1].Description + "\n" + desc)
						} else {
							result = append(result, docs.DocItem{Description: desc, SourceConfig: &configID})
						}
					}
				}
				continue
			}

			// Special case for description-only configs
			if !strings.Contains(resolved, "<category-title>") && (strings.Contains(resolved, "<subtext>") || strings.Contains(resolved, "<text>") || strings.Contains(resolved, "<config") || strings.Contains(resolved, "exception ")) {
				desc := collectFormattedText(resolved, configMap)
				
				// Handle multi-exception description-only blocks
				if strings.Contains(desc, "exception ") || strings.Contains(desc, "#NTG_") {
					var parts []string
					if strings.Contains(desc, "exception ") {
						parts = strings.Split(desc, "exception ")
						for i := 1; i < len(parts); i++ {
							parts[i] = "exception " + parts[i]
						}
					} else {
						// Split by #NTG_ if no "exception" but contains error codes
						re := regexp.MustCompile(`(<b><code>#NTG_[A-Z_]+</code></b>)`)
						parts = re.Split(desc, -1)
						delims := re.FindAllString(desc, -1)
						if len(parts) > 0 {
							firstPart := parts[0]
							newParts := []string{firstPart}
							for i, d := range delims {
								newParts = append(newParts, d+parts[i+1])
							}
							parts = newParts
						}
					}

					for i, p := range parts {
						p = strings.TrimSpace(p)
						if p == "" {
							continue
						}
						if i == 0 && !strings.Contains(p, "exception ") && !strings.Contains(p, "#NTG_") {
							if len(result) > 0 {
								result[len(result)-1].Description = strings.TrimSpace(result[len(result)-1].Description + "\n" + p)
							} else {
								result = append(result, docs.DocItem{Description: p, SourceConfig: &configID})
							}
							continue
						}

						// Try to extract name: rem
						var name, rem string
						if strings.HasPrefix(p, "exception ") {
							lines := strings.SplitN(p, "\n", 2)
							name = lines[0]
							if len(lines) > 1 {
								rem = lines[1]
							}
						} else {
							// Handle error code blocks like <b><code>#NTG_...</code></b> <code>-1</code> description
							re := regexp.MustCompile(`(?s)^(<b><code>#NTG_[A-Z_]+</code></b>(?:\s+<code>-?\d+</code>)?)(.*)`)
							match := re.FindStringSubmatch(p)
							if match != nil {
								name = match[1]
								rem = match[2]
							} else {
								name = p
							}
						}
						result = append(result, docs.DocItem{Name: strings.TrimSpace(name), Description: strings.TrimSpace(rem), SourceConfig: &configID})
					}
					continue
				}

				// If the description contains a name: type pattern at the start, try to parse it
				// This handles cases like <b>chat_id</b>: <code>int</code>
				colonRe := regexp.MustCompile(`(?s)^<b>(.*?)(?:</b>|</u>|</i>)(?::| \-)\s*(?:<code>)?(.*?)(?:</code>)?(?:\n\s*|$)`)
				match := colonRe.FindStringSubmatch(desc)
				if match != nil {
					name := strings.TrimSpace(match[1])
					var typ *string
					if match[2] != "" {
						t := strings.TrimSpace(match[2])
						typ = &t
					}
					rem := strings.TrimSpace(desc[len(match[0]):])
					result = append(result, docs.DocItem{Name: name, Type: typ, Description: rem, SourceConfig: &configID})
				} else if len(result) > 0 {
					result[len(result)-1].Description = strings.TrimSpace(result[len(result)-1].Description + "\n" + desc)
				} else {
					result = append(result, docs.DocItem{Description: desc, SourceConfig: &configID})
				}
				continue
			}

			// Strip outer <subtext> if any
			if strings.HasPrefix(resolved, "<subtext>") && strings.HasSuffix(resolved, "</subtext>") {
				resolved = resolved[len("<subtext>") : len(resolved)-len("</subtext>")]
			}
			text := strings.TrimSpace(resolved)

			// Detect parameters like "name: type\ndescription"
			paramRe := regexp.MustCompile(`(?m)^(?:<category-title>.*?<ref>)?([a-zA-Z0-9_]+)(?:</ref>)?(?:\s*:\s*(?:<shi>)?(.*?)(?:</shi>)?)?(?:</category-title>)?$`)
			matches := paramRe.FindAllStringSubmatchIndex(text, -1)

			if len(matches) > 0 {
				lastIdx := 0
				for i, m := range matches {
					// Text before this match belongs to previous item's description
					if i > 0 {
						result[len(result)-1].Description = strings.TrimSpace(result[len(result)-1].Description + "\n" + collectFormattedText(text[lastIdx:m[0]], configMap))
					} else if m[0] > 0 {
						prefix := strings.TrimSpace(text[:m[0]])
						if prefix != "" {
							if len(result) > 0 {
								result[len(result)-1].Description = strings.TrimSpace(result[len(result)-1].Description + "\n" + collectFormattedText(prefix, configMap))
							} else {
								result = append(result, docs.DocItem{Description: collectFormattedText(prefix, configMap)})
							}
						}
					}

					name := text[m[2]:m[3]]
					var typ *string
					typeText := strings.TrimSpace(collectFormattedText(text[m[4]:m[5]], configMap))
					if typeText != "" {
						typ = &typeText
					}
					result = append(result, docs.DocItem{Name: name, Type: typ, SourceConfig: &configID})
					lastIdx = m[1]
				}
				if lastIdx < len(text) {
					result[len(result)-1].Description = collectFormattedText(text[lastIdx:], configMap)
				}
				continue
			}

			if strings.Contains(text, "exception ") {
				parts := strings.Split(text, "exception ")
				for i, p := range parts {
					if i == 0 {
						if strings.TrimSpace(p) != "" {
							if len(result) > 0 {
								result[len(result)-1].Description = strings.TrimSpace(result[len(result)-1].Description + "\n" + p)
							} else {
								result = append(result, docs.DocItem{Description: strings.TrimSpace(p)})
							}
						}
						continue
					}
					content := "exception " + p
					lines := strings.SplitN(strings.TrimSpace(content), "\n", 2)
					firstLine := lines[0]
					desc := ""
					if len(lines) > 1 {
						desc = strings.TrimSpace(lines[1])
					}

					if strings.Contains(firstLine, ":") {
						idx := strings.LastIndex(firstLine, ":")
						name := strings.TrimSpace(firstLine[:idx])
						typ := strings.TrimSpace(firstLine[idx+1:])
						result = append(result, docs.DocItem{Name: name, Type: &typ, Description: desc, SourceConfig: &configID})
					} else {
						result = append(result, docs.DocItem{Name: firstLine, Description: desc, SourceConfig: &configID})
					}
				}
				continue
			}

			lines := strings.SplitN(text, "\n", 2)
			firstLine := lines[0]
			desc := ""
			if len(lines) > 1 {
				desc = strings.TrimSpace(lines[1])
			}

			if strings.Contains(firstLine, ":") && !strings.Contains(firstLine, "(") {
				idx := strings.LastIndex(firstLine, ":")
				name := strings.TrimSpace(firstLine[:idx])
				typ := strings.TrimSpace(firstLine[idx+1:])
				result = append(result, docs.DocItem{Name: name, Type: &typ, Description: desc, SourceConfig: &configID})
			} else if len(result) > 0 {
				result[len(result)-1].Description = strings.TrimSpace(result[len(result)-1].Description + "\n" + text)
			} else {
				result = append(result, docs.DocItem{Name: firstLine, Description: desc, SourceConfig: &configID})
			}
		} else if raw, ok := item["raw"]; ok {
			rawText := raw
			var name string
			var typ *string

			if strings.Contains(rawText, ":") {
				// Don't split at colons that are part of a URL or inside parentheses
				colonRe := regexp.MustCompile(`([a-zA-Z0-9_]+)\s*:\s*([^:]+)`)
				match := colonRe.FindStringSubmatch(rawText)
				if match != nil && !strings.Contains(rawText, "://") && !strings.Contains(rawText, "(") {
					name = strings.TrimSpace(match[1])
					t := strings.TrimSpace(match[2])
					typ = &t
				} else {
					name = strings.TrimSpace(rawText)
				}
			} else {
				name = strings.TrimSpace(rawText)
			}
			result = append(result, docs.DocItem{Name: name, Type: typ})
		} else if text, ok := item["text"]; ok {
			if len(result) > 0 {
				result[len(result)-1].Description = strings.TrimSpace(result[len(result)-1].Description + "\n" + strings.TrimSpace(text))
			} else {
				result = append(result, docs.DocItem{Name: "", Description: strings.TrimSpace(text)})
			}
		} else if subText, ok := item["sub_text"]; ok {
			if len(result) > 0 {
				result[len(result)-1].Description = strings.TrimSpace(result[len(result)-1].Description + "\n" + strings.TrimSpace(subText))
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

			newItems := normalizeItems(rawItems, configMap)
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

func collectTokenContent(decoder *xml.Decoder) string {
	var sb strings.Builder
	depth := 1
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		switch t := token.(type) {
		case xml.StartElement:
			depth++
		case xml.EndElement:
			depth--
			if depth == 0 {
				return sb.String()
			}
		case xml.CharData:
			sb.WriteString(string(t))
		}
	}
	return sb.String()
}

func generateDiff(oldCode, newCode string) string {
	oldLines := strings.Split(dedent(oldCode), "\n")
	newLines := strings.Split(dedent(newCode), "\n")

	var sb strings.Builder
	maxLen := len(oldLines)
	if len(newLines) > maxLen {
		maxLen = len(newLines)
	}

	for i := 0; i < maxLen; i++ {
		if i < len(oldLines) && i < len(newLines) {
			if strings.TrimSpace(oldLines[i]) == strings.TrimSpace(newLines[i]) {
				sb.WriteString("  " + oldLines[i] + "\n")
			} else {
				sb.WriteString("- " + oldLines[i] + "\n")
				sb.WriteString("+ " + newLines[i] + "\n")
			}
		} else if i < len(oldLines) {
			sb.WriteString("- " + oldLines[i] + "\n")
		} else if i < len(newLines) {
			sb.WriteString("+ " + newLines[i] + "\n")
		}
	}
	return strings.TrimSpace(sb.String())
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
