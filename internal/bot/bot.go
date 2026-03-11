package bot

import (
	"ashokshau/pytgdocs/internal/docs"
	"crypto/sha256"
	"encoding/hex"
	"github.com/AshokShau/gotdbot"
)

type Bot struct {
	Client  *gotdbot.Client
	Docs    docs.Documentation
	HashMap map[string]*docs.DocEntry
}

func New(client *gotdbot.Client, docData docs.Documentation) *Bot {
	b := &Bot{
		Client:  client,
		Docs:    docData,
		HashMap: make(map[string]*docs.DocEntry),
	}

	for p, entry := range docData {
		hash := sha256.Sum256([]byte(p))
		pathHash := hex.EncodeToString(hash[:8])
		b.HashMap[pathHash] = entry
	}

	return b
}
