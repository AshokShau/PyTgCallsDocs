package handlers

import (
	"ashokshau/pytgdocs/internal/bot"
	"ashokshau/pytgdocs/internal/bot/utils"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/AshokShau/gotdbot"
	"github.com/AshokShau/gotdbot/handlers"
)

func Register(b *bot.Bot) {
	d := b.Client.Dispatcher

	d.AddHandler(handlers.NewCommand("start", func(c *gotdbot.Client, ctx *gotdbot.Context) error {
		me, _ := c.GetMe()
		botUsername := me.Usernames.EditableUsername
		welcomeText := fmt.Sprintf(`👋 <b>Welcome to PyTgCalls Documentation Bot!</b>

I can help you find information about PyTgCalls and NTgCalls methods, classes, and more.

• Use the 🔍 <b>Search</b> button to search the documentation
• Or type your query directly in the chat
• Visit our <a href="https://pytgcalls.github.io/">Documentation</a> for detailed guides

• <code>@%s Quick start</code>
• <code>@%s First, take a look at +Quick start+. To play in a voice chat, use the +play+ method.</code>

• <code>@%s #10</code>: Bot shows results for pytgcalls/pytgcalls and pytgcalls/ntgcalls
• <code>@%s nt#10</code>: Bot shows results for pytgcalls/ntgcalls


Made with ❤️ by @AshokShau`, botUsername, botUsername, botUsername, botUsername)

		_, err := ctx.EffectiveMessage.ReplyText(c, welcomeText, &gotdbot.SendTextMessageOpts{ParseMode: gotdbot.ParseModeHTML})
		return err
	}))

	d.AddHandler(handlers.NewUpdateNewInlineQuery(nil, func(c *gotdbot.Client, ctx *gotdbot.Context) error {
		return handleInlineQuery(b, c, ctx)
	}))

	d.AddHandler(handlers.NewUpdateNewInlineCallbackQuery(nil, func(c *gotdbot.Client, ctx *gotdbot.Context) error {
		return handleInlineCallbackQuery(b, c, ctx)
	}))
}

func handleInlineQuery(b *bot.Bot, c *gotdbot.Client, ctx *gotdbot.Context) error {
	iq := ctx.Update.UpdateNewInlineQuery
	if b.Docs == nil {
		return nil
	}

	query := iq.Query
	if query == "" {
		return nil
	}

	var inlineResults []gotdbot.InputInlineQueryResult

	if strings.Contains(query, "#") {
		issueResults := utils.SearchGitHub(c, query)
		inlineResults = append(inlineResults, issueResults...)
	}

	if strings.Contains(query, "+") {
		customResult := utils.HandleCustomText(query, b.Docs, c)
		if customResult != nil {
			inlineResults = append(inlineResults, customResult)
		}
	}

	docResults := b.Docs.Search(query, 15)
	for _, entry := range docResults {
		text := utils.FormatEntry(entry)
		formatted, _ := gotdbot.GetFormattedText(c, text, nil, "HTML")

		hash := sha256.Sum256([]byte(entry.Path))
		id := hex.EncodeToString(hash[:8])

		inlineResults = append(inlineResults, &gotdbot.InputInlineQueryResultArticle{
			Id:          "doc_" + id,
			Title:       fmt.Sprintf("[%s] %s", entry.Lib, entry.Title),
			Description: entry.Description,
			InputMessageContent: &gotdbot.InputMessageText{
				Text: formatted,
			},
			ReplyMarkup: utils.GetEntryKeyboard(entry, "main"),
		})
	}

	return c.AnswerInlineQuery(0, iq.Id, "", inlineResults, nil)
}

func handleInlineCallbackQuery(b *bot.Bot, c *gotdbot.Client, ctx *gotdbot.Context) error {
	cq := ctx.Update.UpdateNewInlineCallbackQuery
	var dataBy []byte
	if p, ok := cq.Payload.(*gotdbot.CallbackQueryPayloadData); ok {
		dataBy = p.Data
	}

	data := string(dataBy)
	if data == "" {
		return nil
	}

	_ = c.AnswerCallbackQuery(300, cq.Id, "...", "", nil)
	parts := strings.SplitN(data, ":", 2)
	if len(parts) < 2 {
		return nil
	}

	view := parts[0]
	pathHash := parts[1]

	entry, ok := b.HashMap[pathHash]
	if !ok {
		return nil
	}

	var text string
	switch view {
	case "main":
		text = utils.FormatEntry(entry)
	case "example":
		text = utils.FormatExample(entry)
	case "params":
		text = utils.FormatParameters(entry)
	case "raises":
		text = utils.FormatRaises(entry)
	case "details":
		text = utils.FormatOtherDetails(entry)
	}

	formatted, err := gotdbot.GetFormattedText(c, text, nil, "HTML")
	if err != nil {
		return err
	}

	kb := utils.GetEntryKeyboard(entry, view)
	return c.EditInlineMessageText(cq.InlineMessageId, gotdbot.InputMessageText{
		Text: formatted,
	}, &gotdbot.EditInlineMessageTextOpts{
		ReplyMarkup: kb,
	})
}
