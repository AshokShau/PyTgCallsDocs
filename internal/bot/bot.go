package bot

import (
	"ashokshau/pytgdocs/internal/docs"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"

	"github.com/AshokShau/gotdbot"
)

type Bot struct {
	Client  *gotdbot.Client
	Docs    docs.Documentation
	HashMap map[string]*docs.DocEntry

	Mu           sync.RWMutex
	ClickHistory map[int64][]time.Time
	Bans         map[int64]time.Time
}

func New(client *gotdbot.Client, docData docs.Documentation) *Bot {
	b := &Bot{
		Client:       client,
		Docs:         docData,
		HashMap:      make(map[string]*docs.DocEntry),
		ClickHistory: make(map[int64][]time.Time),
		Bans:         make(map[int64]time.Time),
	}

	for p, entry := range docData {
		hash := sha256.Sum256([]byte(p))
		pathHash := hex.EncodeToString(hash[:16])
		b.HashMap[pathHash] = entry
	}

	return b
}
