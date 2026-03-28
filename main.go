package main

//go:generate go run github.com/AshokShau/gotdbot/scripts/tools@latest

import (
	"ashokshau/pytgdocs/config"
	"ashokshau/pytgdocs/internal/bot"
	"ashokshau/pytgdocs/internal/bot/handlers"
	"ashokshau/pytgdocs/internal/docs"
	"fmt"
	"log"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/AshokShau/gotdbot"

	_ "github.com/joho/godotenv/autoload"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	logger := slog.New(
		slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level:     slog.LevelInfo,
			AddSource: true,
			ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
				if a.Key == slog.TimeKey {
					t := a.Value.Time()
					a.Value = slog.StringValue(t.Format("2006-01-02 15:04:05"))
				}

				if a.Key == slog.SourceKey {
					source := a.Value.Any().(*slog.Source)
					a.Value = slog.StringValue(fmt.Sprintf("%s:%d", filepath.Base(source.File), source.Line))
				}

				return a
			},
		}),
	)

	slog.SetDefault(logger)
	docData, err := docs.Load("./docs.json")
	if err != nil {
		slog.Error("Failed to load docs.json", "error", err)
		os.Exit(1)
	}

	clientConfig := &gotdbot.ClientOpts{
		LibraryPath: "./libtdjson.so.1.8.62",
		Logger:      logger,
	}

	client, err := gotdbot.NewClient(int32(cfg.ApiID), cfg.ApiHash, cfg.Token, clientConfig)
	if err != nil {
		slog.Error("gotdbot.NewClient error", "error", err)
		os.Exit(1)
	}

	b := bot.New(client, docData)
	handlers.Register(b)

	if err = client.Start(); err != nil {
		slog.Error("gotdbot.Start() error", "error", err)
		os.Exit(1)
	}

	me, _ := client.GetMe()
	username := ""
	if me.Usernames != nil && len(me.Usernames.ActiveUsernames) > 0 {
		username = me.Usernames.ActiveUsernames[0]
	}

	slog.Info("Bot started as", "username", username, "ID", me.Id)
	client.Idle()
}
