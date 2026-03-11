package utils

import (
	"ashokshau/pytgdocs/internal/docs"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/AshokShau/gotdbot"
)

func FormatEntry(e *docs.DocEntry) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("<b>%s</b> (%s %s)\n\n", e.Title, e.Lib, e.Kind))
	sb.WriteString(strings.TrimSpace(e.Description))
	if e.Details.Signature != nil {
		sig := strings.TrimSpace(*e.Details.Signature)
		if sig != "" {
			sb.WriteString(fmt.Sprintf("\n\n<code>%s</code>", sig))
		}
	}
	sb.WriteString(fmt.Sprintf("\n\n<a href=\"%s\">View Online</a>", e.DocURL))
	return sb.String()
}

func FormatExample(e *docs.DocEntry) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("<b>Code Example for %s</b>\n\n", e.Title))
	if e.Example != nil {
		code := strings.TrimSpace(e.Example.Code)
		sb.WriteString(fmt.Sprintf("<pre><code class=\"language-%s\">%s</code></pre>", e.Example.Language, code))
	} else {
		sb.WriteString("No example available.")
	}
	return sb.String()
}

func FormatParameters(e *docs.DocEntry) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("<b>Parameters for %s</b>\n\n", e.Title))

	hasParams := false
	if len(e.Details.Parameters) > 0 {
		hasParams = true
		for _, p := range e.Details.Parameters {
			name := strings.TrimSpace(p.Name)
			desc := strings.TrimSpace(p.Description)
			if name == "" && desc == "" {
				continue
			}
			typ := ""
			if p.Type != nil {
				typ = " (" + *p.Type + ")"
			}
			if name != "" {
				sb.WriteString(fmt.Sprintf("- <code>%s</code>%s: %s\n", name, typ, desc))
			} else {
				sb.WriteString(fmt.Sprintf("- %s %s\n", typ, desc))
			}
		}
	}

	for _, s := range e.Details.Sections {
		if strings.Contains(strings.ToUpper(s.Title), "PARAMETERS") {
			hasParams = true
			for _, item := range s.Items {
				name := strings.TrimSpace(item.Name)
				desc := strings.TrimSpace(item.Description)
				if name != "" {
					sb.WriteString(fmt.Sprintf("- <code>%s</code>: %s\n", name, desc))
				} else {
					sb.WriteString(fmt.Sprintf("- %s\n", desc))
				}
			}
		}
	}

	if !hasParams {
		return "No parameters documented."
	}
	return strings.TrimSpace(sb.String())
}

func FormatRaises(e *docs.DocEntry) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("<b>Exceptions for %s</b>\n\n", e.Title))

	hasRaises := false
	for _, s := range e.Details.Sections {
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

func FormatOtherDetails(e *docs.DocEntry) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("<b>Details for %s</b>\n\n", e.Title))

	hasAny := false

	if len(e.Details.Members) > 0 {
		hasAny = true
		sb.WriteString("<b>MEMBERS:</b>\n")
		for _, m := range e.Details.Members {
			val := ""
			if m.Value != nil {
				val = " = " + *m.Value
			}
			sb.WriteString(fmt.Sprintf("- <code>%s</code>%s: %s\n", m.Name, val, strings.TrimSpace(m.Description)))
		}
		sb.WriteString("\n")
	}

	if len(e.Details.Properties) > 0 {
		hasAny = true
		sb.WriteString("<b>PROPERTIES:</b>\n")
		for _, p := range e.Details.Properties {
			typ := ""
			if p.Type != nil {
				typ = " (" + *p.Type + ")"
			}
			sb.WriteString(fmt.Sprintf("- <code>%s</code>%s: %s\n", p.Name, typ, strings.TrimSpace(p.Description)))
		}
		sb.WriteString("\n")
	}

	for _, s := range e.Details.Sections {
		title := strings.ToUpper(s.Title)
		if strings.Contains(title, "PARAMETERS") || strings.Contains(title, "RAISES") {
			continue
		}
		hasAny = true
		sb.WriteString(fmt.Sprintf("<b>%s:</b>\n", title))
		for _, item := range s.Items {
			name := strings.TrimSpace(item.Name)
			desc := strings.TrimSpace(item.Description)
			if name != "" {
				sb.WriteString(fmt.Sprintf("- <code>%s</code>: %s\n", name, desc))
			} else {
				sb.WriteString(fmt.Sprintf("- %s\n", desc))
			}
		}
		sb.WriteString("\n")
	}

	if !hasAny {
		return "No additional details available."
	}

	return strings.TrimSpace(sb.String())
}

func GetEntryKeyboard(e *docs.DocEntry, currentView string) *gotdbot.ReplyMarkupInlineKeyboard {
	hash := sha256.Sum256([]byte(e.Path))
	pathHash := hex.EncodeToString(hash[:16])

	var row1 []gotdbot.InlineKeyboardButton
	var row2 []gotdbot.InlineKeyboardButton

	if currentView != "main" {
		row1 = append(row1, gotdbot.InlineKeyboardButton{
			Text: "📖 Description",
			Type: &gotdbot.InlineKeyboardButtonTypeCallback{Data: []byte("main:" + pathHash)},
		})
	}

	if e.Example != nil && currentView != "example" {
		row1 = append(row1, gotdbot.InlineKeyboardButton{
			Text: "💻 Example",
			Type: &gotdbot.InlineKeyboardButtonTypeCallback{Data: []byte("example:" + pathHash)},
		})
	}

	hasParams := len(e.Details.Parameters) > 0
	for _, s := range e.Details.Sections {
		if strings.Contains(strings.ToUpper(s.Title), "PARAMETERS") {
			hasParams = true
		}
	}
	if hasParams && currentView != "params" {
		row2 = append(row2, gotdbot.InlineKeyboardButton{
			Text: "📝 Parameters",
			Type: &gotdbot.InlineKeyboardButtonTypeCallback{Data: []byte("params:" + pathHash)},
		})
	}

	hasRaises := false
	for _, s := range e.Details.Sections {
		if strings.Contains(strings.ToUpper(s.Title), "RAISES") {
			hasRaises = true
		}
	}
	if hasRaises && currentView != "raises" {
		row2 = append(row2, gotdbot.InlineKeyboardButton{
			Text: "⚠️ Raises",
			Type: &gotdbot.InlineKeyboardButtonTypeCallback{Data: []byte("raises:" + pathHash)},
		})
	}

	hasOthers := len(e.Details.Members) > 0 || len(e.Details.Properties) > 0
	for _, s := range e.Details.Sections {
		title := strings.ToUpper(s.Title)
		if !strings.Contains(title, "PARAMETERS") && !strings.Contains(title, "RAISES") {
			hasOthers = true
		}
	}
	if hasOthers && currentView != "details" {
		row2 = append(row2, gotdbot.InlineKeyboardButton{
			Text: "🔍 Details",
			Type: &gotdbot.InlineKeyboardButtonTypeCallback{Data: []byte("details:" + pathHash)},
		})
	}

	kb := &gotdbot.ReplyMarkupInlineKeyboard{
		Rows: [][]gotdbot.InlineKeyboardButton{},
	}
	if len(row1) > 0 {
		kb.Rows = append(kb.Rows, row1)
	}
	if len(row2) > 0 {
		kb.Rows = append(kb.Rows, row2)
	}

	kb.Rows = append(kb.Rows, []gotdbot.InlineKeyboardButton{
		{
			Text: "🌐 View Online",
			Type: &gotdbot.InlineKeyboardButtonTypeUrl{Url: e.DocURL},
		},
	})

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

	text, err := gotdbot.GetFormattedText(c, fmt.Sprintf("<a href=\"%s\">%s</a>", url, title), nil, "HTML")
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

func HandleCustomText(query string, docData docs.Documentation, c *gotdbot.Client) *gotdbot.InputInlineQueryResultArticle {
	re := regexp.MustCompile(`\+([^+]+)\+`)
	matches := re.FindAllStringSubmatch(query, -1)

	if len(matches) == 0 {
		return nil
	}

	// TODO: Add support for ntgcalls + pytgcalls with better Title and Description

	replacedText := query
	for _, match := range matches {
		fullMatch := match[0]
		docTitle := match[1]

		results := docData.Search(docTitle, 1)
		if len(results) > 0 {
			entry := results[0]
			link := fmt.Sprintf("<a href=\"%s\">%s</a>", entry.DocURL, entry.Title)
			replacedText = strings.Replace(replacedText, fullMatch, link, 1)
		}
	}

	formatted, err := gotdbot.GetFormattedText(c, replacedText, nil, "HTML")
	if err != nil {
		slog.Warn("Error getting custom text:", "error", err)
		return nil
	}

	return &gotdbot.InputInlineQueryResultArticle{
		Id:    "custom_" + hex.EncodeToString([]byte(query))[:16],
		Title: "Custom text with doc links",
		InputMessageContent: &gotdbot.InputMessageText{
			Text: formatted,
		},
	}
}
