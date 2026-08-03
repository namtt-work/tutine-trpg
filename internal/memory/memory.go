package memory

import "context"

type Memory struct {
	ID         string
	SaveID     string
	CampaignID string
	Turn       int
	Type       string
	Scope      string
	Importance int
	Text       string
	Summary    string
	Entities   []string
	Tags       []string
	FactsJSON  string
	LocationID string
	QuestID    string
	NPCID      string
}

type Query struct {
	SaveID     string
	Entities   []string
	Tags       []string
	Types      []string
	Locations  []string
	QuestIDs   []string
	Keywords   []string
	MaxResults int
}

type Hit struct {
	Memory Memory
	Score  float64
}

type Store interface {
	Add(ctx context.Context, memory Memory) error
	Search(ctx context.Context, query Query) ([]Hit, error)
	Close() error
}
