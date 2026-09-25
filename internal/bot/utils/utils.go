package utils

import (
	"ashokshau/pytgdocs/internal/docs"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html"
	"log/slog"
	"regexp"
	"strings"

	"github.com/AshokShau/gotdbot"
)

func GetDefaultLang(e *docs.DocEntry) string {
	if len(e.Tabs) > 0 {
		for _, tab := range e.Tabs {
			if tab.ID == "python" {
				return "python"
			}
		}
		return e.Tabs[0].ID
	}
	return ""
}

func GetOptLang(e *docs.DocEntry, opts []string) string {
	if len(opts) > 0 && opts[0] != "" {
		return opts[0]
	}
	return GetDefaultLang(e)
}

func GetDetailsForLang(e *docs.DocEntry, lang string) *docs.Details {
	if lang != "" && e.LangDetails != nil {
		if dt, ok := e.LangDetails[lang]; ok && dt != nil {
			return dt
		}
	}
	return &e.Details
}

func GetExampleForLang(e *docs.DocEntry, lang string) *docs.Example {
	if lang != "" && e.Examples != nil {
		if ex, ok := e.Examples[lang]; ok && ex != nil {
			return ex
		}
	}
	if e.Example != nil {
		return e.Example
	}
	if len(e.Examples) > 0 {
		for _, ex := range e.Examples {
			return ex
		}
	}
	return nil
}

func FormatEntry(e *docs.DocEntry, opts ...string) string {
	lang := GetOptLang(e, opts)
	details := GetDetailsForLang(e, lang)

	var sb strings.Builder
	title := html.EscapeString(e.Title)
	lib := html.EscapeString(e.Lib)
	kind := html.EscapeString(e.Kind)

	if e.Kind == "example" {
		sb.WriteString(fmt.Sprintf("💻 <b>%s</b>\n\n", title))
	} else {
		sb.WriteString(fmt.Sprintf("<b>%s</b> (%s %s)\n\n", title, lib, kind))
	}
	sb.WriteString(strings.TrimSpace(e.Description))

	if details.Signature != nil {
		sig := strings.TrimSpace(*details.Signature)
		if sig != "" {
			sb.WriteString(fmt.Sprintf("\n\n<code>%s</code>", sig))
		}
	}

	if e.Kind == "example" {
		sb.WriteString(fmt.Sprintf("\n\n<a href=\"%s\">View Source on GitHub</a>", e.DocURL))
	} else {
		sb.WriteString(fmt.Sprintf("\n\n<a href=\"%s\">View Online</a>", e.DocURL))
	}
	return sb.String()
}

func FormatExample(e *docs.DocEntry, opts ...string) string {
	lang := GetOptLang(e, opts)
	ex := GetExampleForLang(e, lang)

	var sb strings.Builder
	title := html.EscapeString(e.Title)
	sb.WriteString(fmt.Sprintf("<b>Code Example for %s</b>\n\n", title))
	if ex != nil {
		language := html.EscapeString(strings.TrimSpace(ex.Language))
		code := html.EscapeString(strings.TrimSpace(ex.Code))
		sb.WriteString(fmt.Sprintf("<pre><code class=\"language-%s\">%s</code></pre>", language, code))
	} else {
		sb.WriteString("No example available.")
	}
	return sb.String()
}

func FormatParameters(e *docs.DocEntry, opts ...string) string {
	lang := GetOptLang(e, opts)
	details := GetDetailsForLang(e, lang)

	var sb strings.Builder
	title := html.EscapeString(e.Title)
	sb.WriteString(fmt.Sprintf("<b>Parameters for %s</b>\n\n", title))

	hasParams := false
	if len(details.Parameters) > 0 {
		hasParams = true
		for _, p := range details.Parameters {
			name := strings.TrimSpace(p.Name)
			desc := strings.TrimSpace(p.Description)
			if name == "" && desc == "" {
				continue
			}
			typ := ""
			if p.Type != nil {
				t := *p.Type
				if !strings.HasPrefix(t, "<") {
					typ = " (<code>" + t + "</code>)"
				} else {
					typ = " (" + t + ")"
				}
			}
			if name != "" {
				if !strings.HasPrefix(name, "<") {
					sb.WriteString(fmt.Sprintf("- <code>%s</code>%s: %s\n", name, typ, desc))
				} else {
					sb.WriteString(fmt.Sprintf("- %s%s: %s\n", name, typ, desc))
				}
			} else {
				sb.WriteString(fmt.Sprintf("- %s %s\n", typ, desc))
			}
		}
	}

	for _, s := range details.Sections {
		if strings.Contains(strings.ToUpper(s.Title), "PARAMETERS") {
			hasParams = true
			for _, item := range s.Items {
				name := strings.TrimSpace(item.Name)
				desc := strings.TrimSpace(item.Description)
				typ := ""
				if item.Type != nil {
					t := *item.Type
					if !strings.HasPrefix(t, "<") {
						typ = " (<code>" + t + "</code>)"
					} else {
						typ = " (" + t + ")"
					}
				}
				if name != "" {
					if !strings.HasPrefix(name, "<") {
						sb.WriteString(fmt.Sprintf("- <code>%s</code>%s: %s\n", name, typ, desc))
					} else {
						sb.WriteString(fmt.Sprintf("- %s%s: %s\n", name, typ, desc))
					}
				} else {
					sb.WriteString(fmt.Sprintf("- %s %s\n", typ, desc))
				}
			}
		}
	}

	if !hasParams {
		return "No parameters documented."
	}
	return strings.TrimSpace(sb.String())
}

func FormatRaises(e *docs.DocEntry, opts ...string) string {
	lang := GetOptLang(e, opts)
	details := GetDetailsForLang(e, lang)

	var sb strings.Builder
	title := html.EscapeString(e.Title)
	sb.WriteString(fmt.Sprintf("<b>Exceptions for %s</b>\n\n", title))

	hasRaises := false
	for _, s := range details.Sections {
		if strings.Contains(strings.ToUpper(s.Title), "RAISES") {
			hasRaises = true
			for _, item := range s.Items {
				name := strings.TrimSpace(item.Name)
				desc := strings.TrimSpace(item.Description)

				if strings.HasPrefix(name, "exception ") {
					excName := strings.TrimSpace(name[len("exception "):])
					sb.WriteString(fmt.Sprintf("exception <b>%s</b> : %s\n\n", excName, desc))
				} else if name != "" {
					sb.WriteString(fmt.Sprintf("<b>%s</b> : %s\n\n", name, desc))
				} else {
					if strings.HasPrefix(desc, "exception ") {
						parts := strings.SplitN(desc, "\n", 2)
						if len(parts) == 2 {
							excLine := strings.TrimSpace(parts[0])
							content := strings.TrimSpace(parts[1])
							excName := strings.TrimSpace(excLine[len("exception "):])
							sb.WriteString(fmt.Sprintf("exception <b>%s</b> : %s\n\n", excName, content))
						} else {
							sb.WriteString(fmt.Sprintf("- %s\n\n", desc))
						}
					} else {
						sb.WriteString(fmt.Sprintf("- %s\n\n", desc))
					}
				}
			}
		}
	}

	if !hasRaises {
		return "No exceptions documented."
	}
	return strings.TrimSpace(sb.String())
}

func FormatOtherDetails(e *docs.DocEntry, opts ...string) string {
	lang := GetOptLang(e, opts)
	details := GetDetailsForLang(e, lang)

	var sb strings.Builder
	title := html.EscapeString(e.Title)
	sb.WriteString(fmt.Sprintf("<b>Details for %s</b>\n\n", title))

	hasAny := false

	if len(details.Members) > 0 {
		hasAny = true
		sb.WriteString("<b>MEMBERS:</b>\n")
		for _, m := range details.Members {
			val := ""
			if m.Value != nil {
				val = " = " + *m.Value
			}
			name := m.Name
			if !strings.HasPrefix(name, "<") {
				name = "<code>" + name + "</code>"
			}
			sb.WriteString(fmt.Sprintf("- %s%s: %s\n", name, val, strings.TrimSpace(m.Description)))
		}
		sb.WriteString("\n")
	}

	if len(details.Properties) > 0 {
		hasAny = true
		sb.WriteString("<b>PROPERTIES:</b>\n")
		for _, p := range details.Properties {
			typ := ""
			if p.Type != nil {
				t := *p.Type
				if !strings.HasPrefix(t, "<") {
					typ = " (<code>" + t + "</code>)"
				} else {
					typ = " (" + t + ")"
				}
			}
			name := p.Name
			if !strings.HasPrefix(name, "<") {
				name = "<code>" + name + "</code>"
			}
			sb.WriteString(fmt.Sprintf("- %s%s: %s\n", name, typ, strings.TrimSpace(p.Description)))
		}
		sb.WriteString("\n")
	}

	if len(details.Methods) > 0 {
		hasAny = true
		sb.WriteString("<b>METHODS:</b>\n")
		for _, m := range details.Methods {
			typ := ""
			if m.Type != nil {
				t := *m.Type
				if !strings.HasPrefix(t, "<") {
					typ = " (<code>" + t + "</code>)"
				} else {
					typ = " (" + t + ")"
				}
			}
			name := m.Name
			if !strings.HasPrefix(name, "<") {
				name = "<code>" + name + "</code>"
			}
			sb.WriteString(fmt.Sprintf("- %s%s: %s\n", name, typ, strings.TrimSpace(m.Description)))
		}
		sb.WriteString("\n")
	}

	for _, s := range details.Sections {
		title := strings.ToUpper(s.Title)
		if strings.Contains(title, "PARAMETERS") || strings.Contains(title, "RAISES") {
			continue
		}
		hasAny = true
		sb.WriteString(fmt.Sprintf("<b>%s:</b>\n", title))
		for _, item := range s.Items {
			name := strings.TrimSpace(item.Name)
			desc := strings.TrimSpace(item.Description)

			line := ""
			if name != "" {
				if item.URL != nil {
					line = fmt.Sprintf("- <a href=\"%s\">%s</a>: %s\n", *item.URL, name, desc)
				} else if !strings.HasPrefix(name, "<") {
					line = fmt.Sprintf("- <code>%s</code>: %s\n", name, desc)
				} else {
					line = fmt.Sprintf("- %s: %s\n", name, desc)
				}
			} else {
				line = fmt.Sprintf("- %s\n", desc)
			}
			sb.WriteString(line)
		}
		sb.WriteString("\n")
	}

	if !hasAny {
		return "No additional details available."
	}

	return strings.TrimSpace(sb.String())
}

func GetEntryKeyboard(e *docs.DocEntry, currentView string, opts ...string) *gotdbot.ReplyMarkupInlineKeyboard {
	hash := sha256.Sum256([]byte(e.Path))
	pathHash := hex.EncodeToString(hash[:16])

	lang := GetOptLang(e, opts)
	details := GetDetailsForLang(e, lang)

	kb := &gotdbot.ReplyMarkupInlineKeyboard{
		Rows: [][]gotdbot.InlineKeyboardButton{},
	}

	if len(e.Tabs) > 1 {
		var tabRow []gotdbot.InlineKeyboardButton
		for _, tab := range e.Tabs {
			label := tab.Label
			if tab.ID == lang {
				label = "• " + label + " •"
			}
			cbd := fmt.Sprintf("%s:%s:%s", currentView, pathHash, tab.ID)
			tabRow = append(tabRow, gotdbot.InlineKeyboardButton{
				Text: label,
				Type: &gotdbot.InlineKeyboardButtonTypeCallback{Data: []byte(cbd)},
			})
		}
		kb.Rows = append(kb.Rows, tabRow)
	}

	var viewButtons []gotdbot.InlineKeyboardButton

	cbdSuffix := pathHash
	if lang != "" {
		cbdSuffix = pathHash + ":" + lang
	}

	if currentView != "main" {
		viewButtons = append(viewButtons, gotdbot.InlineKeyboardButton{
			Text: "Description",
			Type: &gotdbot.InlineKeyboardButtonTypeCallback{Data: []byte("main:" + cbdSuffix)},
		})
	}

	hasEx := GetExampleForLang(e, lang) != nil
	if hasEx && currentView != "example" {
		viewButtons = append(viewButtons, gotdbot.InlineKeyboardButton{
			Text: "Example",
			Type: &gotdbot.InlineKeyboardButtonTypeCallback{Data: []byte("example:" + cbdSuffix)},
		})
	}

	hasParams := len(details.Parameters) > 0
	if !hasParams {
		for _, s := range details.Sections {
			if strings.Contains(strings.ToUpper(s.Title), "PARAMETERS") {
				hasParams = true
				break
			}
		}
	}
	if hasParams && currentView != "params" {
		viewButtons = append(viewButtons, gotdbot.InlineKeyboardButton{
			Text: "Parameters",
			Type: &gotdbot.InlineKeyboardButtonTypeCallback{Data: []byte("params:" + cbdSuffix)},
		})
	}

	hasRaises := false
	for _, s := range details.Sections {
		if strings.Contains(strings.ToUpper(s.Title), "RAISES") {
			hasRaises = true
			break
		}
	}
	if hasRaises && currentView != "raises" {
		viewButtons = append(viewButtons, gotdbot.InlineKeyboardButton{
			Text: "Raises",
			Type: &gotdbot.InlineKeyboardButtonTypeCallback{Data: []byte("raises:" + cbdSuffix)},
		})
	}

	hasOthers := len(details.Members) > 0 || len(details.Properties) > 0 || len(details.Methods) > 0
	if !hasOthers {
		for _, s := range details.Sections {
			title := strings.ToUpper(s.Title)
			if !strings.Contains(title, "PARAMETERS") && !strings.Contains(title, "RAISES") {
				hasOthers = true
				break
			}
		}
	}
	if hasOthers && currentView != "details" {
		viewButtons = append(viewButtons, gotdbot.InlineKeyboardButton{
			Text: "Details",
			Type: &gotdbot.InlineKeyboardButtonTypeCallback{Data: []byte("details:" + cbdSuffix)},
		})
	}

	viewButtons = append(viewButtons, gotdbot.InlineKeyboardButton{
		Text: "🌐",
		Type: &gotdbot.InlineKeyboardButtonTypeUrl{Url: e.DocURL},
	})

	for i := 0; i < len(viewButtons); i += 2 {
		end := min(i+2, len(viewButtons))
		kb.Rows = append(kb.Rows, viewButtons[i:end])
	}

	return kb
}

func SearchGitHub(c *gotdbot.Client, query string) []gotdbot.InputInlineQueryResult {
	var results []gotdbot.InputInlineQueryResult

	re := regexp.MustCompile(`(nt)?#(\d+)`)
	matches := re.FindAllStringSubmatch(query, -1)

	for _, match := range matches {
		isNT := match[1] == "nt"
		num := match[2]

		if isNT {
			results = append(results, createGitHubResult(c, "ntgcalls", num))
		} else {
			results = append(results, createGitHubResult(c, "pytgcalls", num))
			results = append(results, createGitHubResult(c, "ntgcalls", num))
		}
	}

	return results
}

func createGitHubResult(c *gotdbot.Client, repo, num string) *gotdbot.InputInlineQueryResultArticle {
	url := fmt.Sprintf("https://github.com/pytgcalls/%s/pull/%s", repo, num)
	title := fmt.Sprintf("[%s] PR/Issue #%s", repo, num)

	text, err := c.GetFormattedText(fmt.Sprintf("<a href=\"%s\">%s</a>", url, title), nil, "HTML")
	if err != nil {
		slog.Warn("Error getting github result:", "error", err)
		return nil
	}

	return &gotdbot.InputInlineQueryResultArticle{
		Id:                  fmt.Sprintf("gh_%s_%s", repo, num),
		Title:               title,
		InputMessageContent: &gotdbot.InputMessageText{Text: text},
	}
}

func HandleCustomText(query string, docData docs.Documentation, c *gotdbot.Client) []gotdbot.InputInlineQueryResult {
	re := regexp.MustCompile(`\+([^+]+)\+`)
	matches := re.FindAllStringSubmatch(query, -1)

	if len(matches) == 0 {
		return nil
	}

	var results []gotdbot.InputInlineQueryResult

	for _, match := range matches {
		fullMatch := match[0]
		docTitle := match[1]

		docResults := docData.Search(docTitle, 2)
		if len(docResults) > 0 {
			var entry *docs.DocEntry
			for _, r := range docResults {
				if r.Lib == "PyTgCalls" {
					entry = r
					break
				}
			}
			if entry == nil {
				entry = docResults[0]
			}

			link := fmt.Sprintf("<a href=\"%s\">%s</a>", entry.DocURL, entry.Title)
			replacedText := strings.ReplaceAll(query, fullMatch, link)
			for _, m := range matches {
				if m[0] == fullMatch {
					continue
				}
				otherDocResults := docData.Search(m[1], 1)
				if len(otherDocResults) > 0 {
					otherEntry := otherDocResults[0]
					otherLink := fmt.Sprintf("<a href=\"%s\">%s</a>", otherEntry.DocURL, otherEntry.Title)
					replacedText = strings.ReplaceAll(replacedText, m[0], otherLink)
				}
			}

			formatted, err := c.GetFormattedText(replacedText, nil, "HTML")
			if err != nil {
				slog.Warn("Error getting custom text:", "error", err)
				continue
			}

			hash := sha256.Sum256([]byte(query + entry.Path))
			results = append(results, &gotdbot.InputInlineQueryResultArticle{
				Id:          "custom_" + hex.EncodeToString(hash[:16]),
				Title:       entry.Title,
				Description: entry.Description,
				InputMessageContent: &gotdbot.InputMessageText{
					Text: formatted,
				},
			})
		}
	}

	return results
}
