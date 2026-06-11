package main

//go:generate go run github.com/AshokShau/gotdbot/scripts/tools

import (
	"ashokshau/pytgdocs/config"
	"ashokshau/pytgdocs/internal/bot"
	"ashokshau/pytgdocs/internal/bot/handlers"
	"ashokshau/pytgdocs/internal/docs"
	"log"
	"log/slog"
	"os"

	"github.com/AshokShau/gotdbot"

	_ "github.com/joho/godotenv/autoload"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	docData, err := docs.Load("./docs.json")
	if err != nil {
		slog.Error("Failed to load docs.json", "error", err)
		os.Exit(1)
	}

	clientConfig := &gotdbot.ClientOpts{
		LibraryPath: "./libtdjson.so.1.8.65",
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

	client.Idle()
}
