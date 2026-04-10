package utils

import (
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"sort"
	"strings"

	"github.com/AshokShau/gotdbot"
)

type Shortcut struct {
	Title       string
	Description string
	Text        string
}

var shortcuts = map[string]Shortcut{
	"nohello": {
		Title:       "No Hello",
		Description: "Please don't just say hello in chat.",
		Text:        "Please don't just say hello in chat! It's better to just ask your question directly.\n\nMore info: https://nohello.net/en/",
	},
	"xy": {
		Title:       "XY Problem",
		Description: "What is the XY problem?",
		Text:        "<b>XY problem</b> (https://xyproblem.info/)!\n\nPlease provide:\n• A clear description of your problem\n• What you already tried\n• Any logs or error messages\n• Additional relevant information",
	},
	"anyone": {
		Title:       "Don't ask to ask",
		Description: "Just ask your question directly.",
		Text:        "Don't ask to ask, just ask! Mentioning that you have a question without actually asking it often leads to delays.\n\nMore info: https://dontasktoask.com/",
	},
	"rtfm": {
		Title:       "RTFM",
		Description: "Read The F***ing Manual.",
		Text:        "<b>RTFM: Read The F***ing Manual!</b>\n\nMost questions are already answered in the official documentation. Please take a look at it before asking.\n\nDocumentation: https://pytgcalls.github.io/",
	},
	"paste": {
		Title:       "Use a Pastebin",
		Description: "Avoid sending large blocks of code or logs in chat.",
		Text:        "Please use a pastebin service for long code snippets or logs to keep the chat clean.\n\nSuggested services:\n• https://paste.rs/\n• https://bin.sc/\n• https://pastebin.com/",
	},
	"permissions": {
		Title:       "File Permissions",
		Description: "Check your file permissions.",
		Text:        "Make sure your files have the correct permissions. You can check them with <code>ls -l</code> and change them with <code>chmod</code> or <code>chown</code>.",
	},
	"error": {
		Title:       "Reporting Errors",
		Description: "How to report an error effectively.",
		Text:        "When reporting an error, please provide:\n• The <b>full</b> error message/traceback\n• The <b>command</b> you ran\n• Your <b>environment</b> (OS, Python version, etc.)",
	},
	"best": {
		Title:       "What is the 'best'?",
		Description: "There is no 'best'.",
		Text:        "There is no 'best' tool or distribution. There are only tools that are better suited for specific tasks or preferences. Choose what fits your needs!",
	},
	"pp": {
		Title:       "Personal Preference",
		Description: "It's all about personal preference.",
		Text:        "This often comes down to <b>Personal Preference</b>. There is no right or wrong answer; it depends on what you prefer and what works for you.",
	},
	"crashed": {
		Title:       "Crash Analysis",
		Description: "How to capture a backtrace using GDB.",
		Text:        "Hi,\nIt looks like your program is crashing or encountering unexpected behavior.\n\nTo help diagnose the issue, please capture a backtrace using GDB:\n<pre><code class=\"language-bash\">gdb -q -iex \"set confirm off\" \\\n      -iex \"set pagination off\" \\\n      -iex \"handle SIGPIPE pass nostop noprint\" \\\n      -iex \"handle SIGINT pass nostop noprint\" \\\n      -iex \"set logging enabled on\" \\\n      -iex \"set debuginfod enabled on\" \\\n      -ex run --args python3 YOUR_SCRIPT.py</code></pre>\n\nAfter the crash occurs, type:\n<code>bt</code>\n\nThen upload the full output to a paste service such as PasteBin (https://pastebin.com/) or BatBin (https://batbin.me/),\nand share the link here so others can help analyze it.",
	},
	"wrong": {
		Title:       "Wrong Package",
		Description: "Fix for installing the wrong package.",
		Text:        "It seems you have installed the wrong package. Please run the following command to fix it:\n\n<code>pip uninstall pytgcalls && pip install py-tgcalls --force-reinstall</code>",
	},
	"lib": {
		Title:       "Library Links",
		Description: "Links to our libraries and documentation.",
		Text:        "Find more about our libraries:\n• https://github.com/pytgcalls/pytgcalls\n• https://github.com/pytgcalls/ntgcalls\n\nDocs:\n• https://pytgcalls.github.io",
	},
	"ntgcalls": {
		Title:       "NTgCalls Required",
		Description: "Install ntgcalls properly.",
		Text:        "<b>NTgCalls is required.</b>\n\nPlease install it properly:\n\n<code>pip install ntgcalls</code>\n\nIf already installed, try reinstalling:\n<code>pip install -U --force-reinstall ntgcalls</code>",
	},
	"pytgcallsinstall": {
		Title:       "PyTgCalls Install",
		Description: "Install PyTgCalls correctly.",
		Text:        "Install PyTgCalls using:\n\n<code>pip install py-tgcalls</code>\n\nDo NOT install <code>pytgcalls</code> (wrong package).",
	},
	"ffmpeg": {
		Title:       "FFmpeg Required",
		Description: "FFmpeg must be installed.",
		Text:        "<b>FFmpeg is required</b> for PyTgCalls.\n\nUbuntu/Debian:\n<code>sudo apt install ffmpeg</code>\n\nArch:\n<code>sudo pacman -S ffmpeg</code>\n\nMac:\n<code>brew install ffmpeg</code>",
	},
	"updatecalls": {
		Title:       "Update Calls Libraries",
		Description: "Update both libs.",
		Text:        "Update both libraries:\n\n<code>pip install -U py-tgcalls ntgcalls</code>",
	},
	"outdated": {
		Title:       "Outdated Library",
		Description: "Library too old.",
		Text:        "Your PyTgCalls/NTgCalls version is outdated.\n\nUpdate using:\n<code>pip install -U py-tgcalls ntgcalls</code>",
	},
	"docs": {
		Title:       "Documentation",
		Description: "Official docs links.",
		Text:        "Documentation:\n• https://pytgcalls.github.io\n\nRepositories:\n• https://github.com/pytgcalls/pytgcalls\n• https://github.com/pytgcalls/ntgcalls",
	},
}

func SearchShortcuts(c *gotdbot.Client, query string) []gotdbot.InputInlineQueryResult {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil
	}

	var keys []string
	for key := range shortcuts {
		if strings.Contains(key, query) {
			keys = append(keys, key)
		}
	}

	sort.Strings(keys)

	var results []gotdbot.InputInlineQueryResult
	for _, key := range keys {
		s := shortcuts[key]
		formatted, err := gotdbot.GetFormattedText(c, s.Text, nil, "HTML")
		if err != nil {
			slog.Warn("Failed to format shortcut text", "key", key, "error", err)
			continue
		}

		hash := sha256.Sum256([]byte("shortcut_" + key))
		id := hex.EncodeToString(hash[:16])

		results = append(results, &gotdbot.InputInlineQueryResultArticle{
			Id:          "shortcut_" + id,
			Title:       "📑 " + s.Title,
			Description: s.Description,
			InputMessageContent: &gotdbot.InputMessageText{
				Text: formatted,
				LinkPreviewOptions: &gotdbot.LinkPreviewOptions{
					IsDisabled: true,
				},
			},
		})
	}

	return results
}
