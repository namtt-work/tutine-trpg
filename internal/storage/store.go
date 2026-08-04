package storage

import (
	"context"
	"time"

	"github.com/namtt/tutine-trpg/internal/game"
)

// Store is the persistence boundary for a save's durable state: the
// snapshot used to resume, the append-only turn history, save discovery for
// auto-resume, and the per-save advisory lock. internal/orchestrator and
// cmd/tu-tien-cli depend on this interface, never on FileStore directly.
type Store interface {
	SaveSnapshot(ctx context.Context, save game.SaveGame) error
	LoadSnapshot(ctx context.Context, saveID string) (game.SaveGame, error)
	AppendEvent(ctx context.Context, saveID string, event Event) error
	ListSaves(ctx context.Context, campaignID string) ([]SaveSummary, error)
	AcquireLock(ctx context.Context, saveID string) (Lock, error)
}

// Lock is held for the lifetime of a session on one save; Release removes it.
type Lock interface {
	Release() error
}

// EventTypeTurnResolved is the Event.Type written for the roleplay/combat
// turns orchestrator.Session.HandleTurn resolves in this phase.
const EventTypeTurnResolved = "turn_resolved"

// Event is one append-only entry in a save's events.jsonl: audit and
// debugging history. state.json, not this, is what resume reads.
type Event struct {
	Turn            int                    `json:"turn"`
	Type            string                 `json:"type"`
	PlayerAction    string                 `json:"player_action"`
	ResolvedEffects []game.StateChangeView `json:"resolved_effects"`
	Narration       string                 `json:"narration"`
	Warnings        []string               `json:"warnings,omitempty"`
	CreatedAt       time.Time              `json:"created_at"`
}

// SaveSummary describes one discoverable save for ListSaves without the
// caller loading the full game.SaveGame.
type SaveSummary struct {
	SaveID       string
	CampaignID   string
	PlayerName   string
	CurrentTurn  int
	CurrentScene string
	UpdatedAt    time.Time
}
