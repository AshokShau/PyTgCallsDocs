package handlers

import (
	"ashokshau/pytgdocs/internal/bot"
	"ashokshau/pytgdocs/internal/bot/utils"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"runtime"
	"strings"
	"time"

	"github.com/AshokShau/gotdbot"
	"github.com/AshokShau/gotdbot/handlers"
)

var startTime = time.Now()

const (
	spamWindow    = 4 * time.Second
	spamMaxClicks = 4
	banDuration   = 10 * time.Minute
)

func Register(b *bot.Bot) {
	d := b.Client.Dispatcher

	d.AddHandler(handlers.NewCommand("ping", pingHandler))

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
		customResults := utils.HandleCustomText(query, b.Docs, c)
		if len(customResults) > 0 {
			inlineResults = append(inlineResults, customResults...)
		}
	}

	docResults := b.Docs.Search(query, 15)
	for _, entry := range docResults {
		text := utils.FormatEntry(entry)
		if len(text) > 1500 {
			text = fmt.Sprintf("<blockquote expandable>%s</blockquote>", text)
		}
		formatted, err := gotdbot.GetFormattedText(c, text, nil, "HTML")
		if err != nil {
			return err
		}

		hash := sha256.Sum256([]byte(entry.Path))
		id := hex.EncodeToString(hash[:16])

		inlineResults = append(inlineResults, &gotdbot.InputInlineQueryResultArticle{
			Id:          "doc_" + id,
			Title:       fmt.Sprintf("[%s] %s", entry.Lib, entry.Title),
			Description: entry.Description,
			InputMessageContent: &gotdbot.InputMessageText{
				Text: formatted,
				LinkPreviewOptions: &gotdbot.LinkPreviewOptions{
					IsDisabled: true,
				},
			},
			ReplyMarkup: utils.GetEntryKeyboard(entry, "main"),
		})
	}

	return c.AnswerInlineQuery(300, iq.Id, "", inlineResults, nil)
}

func handleInlineCallbackQuery(b *bot.Bot, c *gotdbot.Client, ctx *gotdbot.Context) error {
	cq := ctx.Update.UpdateNewInlineCallbackQuery

	userId := cq.SenderUserId
	if userId == 0 {
		slog.Warn("Inline callback without sender user id", "callback_id", cq.Id)
		return nil
	}

	now := time.Now()

	b.Mu.Lock()
	if banUntil, ok := b.Bans[userId]; ok {
		if now.Before(banUntil) {
			b.Mu.Unlock()
			slog.Info("Blocked banned user callback", "user_id", userId, "ban_until", banUntil)
			text := fmt.Sprintf("You are still banned for %d seconds!", int(banUntil.Sub(now).Seconds()))
			_ = c.AnswerCallbackQuery(0, cq.Id, text, "", &gotdbot.AnswerCallbackQueryOpts{ShowAlert: true})
			return gotdbot.EndGroups
		} else {
			delete(b.Bans, userId)
			slog.Info("Expired user ban removed", "user_id", userId)
		}
	}

	history := b.ClickHistory[userId]
	history = append(history, now)

	threshold := now.Add(-spamWindow)
	var newHistory []time.Time
	for _, t := range history {
		if !t.Before(threshold) {
			newHistory = append(newHistory, t)
		}
	}
	b.ClickHistory[userId] = newHistory

	if len(newHistory) >= spamMaxClicks {
		banUntil := now.Add(banDuration)
		b.Bans[userId] = banUntil
		delete(b.ClickHistory, userId)
		b.Mu.Unlock()
		slog.Info("User banned for callback spam", "user_id", userId, "clicks", len(newHistory), "window", spamWindow.String(), "ban_until", banUntil)
		_ = c.AnswerCallbackQuery(0, cq.Id, "You are spamming! You are banned from using the bot for 10 minutes.", "", &gotdbot.AnswerCallbackQueryOpts{ShowAlert: true})
		return gotdbot.EndGroups
	}
	b.Mu.Unlock()

	var dataBy []byte
	if p, ok := cq.Payload.(*gotdbot.CallbackQueryPayloadData); ok {
		dataBy = p.Data
	}

	data := string(dataBy)
	if data == "" {
		slog.Warn("Empty callback data received")
		return nil
	}

	_ = c.AnswerCallbackQuery(1, cq.Id, "loading ...", "", nil)
	parts := strings.SplitN(data, ":", 2)
	if len(parts) < 2 {
		slog.Warn("Invalid callback data format", "data", data)
		return nil
	}

	view := parts[0]
	pathHash := parts[1]

	entry, ok := b.HashMap[pathHash]
	if !ok {
		slog.Warn("Entry not found in HashMap", "pathHash", pathHash, "data", data)
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
	default:
		slog.Warn("Unknown view type in callback", "view", view, "data", data)
		return nil
	}

	if len(text) > 1500 {
		text = fmt.Sprintf("<blockquote expandable>%s</blockquote>", text)
	}

	formatted, err := gotdbot.GetFormattedText(c, text, nil, "HTML")
	if err != nil {
		slog.Error("Failed to get formatted text", "error", err, "view", view, "entry", entry.Title)
		return err
	}

	kb := utils.GetEntryKeyboard(entry, view)
	err = c.EditInlineMessageText(cq.InlineMessageId, gotdbot.InputMessageText{
		Text: formatted,
		LinkPreviewOptions: &gotdbot.LinkPreviewOptions{
			IsDisabled: true,
		},
	}, &gotdbot.EditInlineMessageTextOpts{
		ReplyMarkup: kb,
	})

	if err != nil {
		if strings.Contains(err.Error(), "MESSAGE_NOT_MODIFIED") {
			return nil
		}
		slog.Error("Failed to edit inline message text", "error", err, "view", view, "entry", entry.Title)
	}

	return gotdbot.EndGroups
}

// pingHandler handles the /ping command.
func pingHandler(c *gotdbot.Client, ctx *gotdbot.Context) error {
	m := ctx.EffectiveMessage
	start := time.Now()

	msg, err := m.ReplyText(c, "⏱️ Pinging...", nil)
	if err != nil {
		return err
	}

	latency := time.Since(start).Milliseconds()
	uptime := getFormattedDuration(time.Since(startTime))

	response := fmt.Sprintf(
		"<b>📊 System Performance Metrics</b>\n\n"+
			"⏱️ <b>Bot Latency:</b> <code>%d ms</code>\n"+
			"🕒 <b>Uptime:</b> <code>%s</code>\n"+
			"⚙️ <b>Go Routines:</b> <code>%d</code>\n",
		latency, uptime, runtime.NumGoroutine(),
	)

	_, err = msg.EditText(c, response, &gotdbot.EditTextMessageOpts{ParseMode: "HTML"})
	return err
}
