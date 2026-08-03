package llm

import (
	"context"

	"github.com/namtt/tutine-trpg/internal/game"
)

type Client interface {
	PlanRetrieval(ctx context.Context, req PlannerRequest) (RetrievalPlan, error)
	Narrate(ctx context.Context, req NarratorRequest) (NarratorResponse, error)
	ExtractMemories(ctx context.Context, req ExtractorRequest) ([]MemoryDraft, error)
}

type PlannerRequest struct {
	PlayerAction string
	SceneID      string
	AllowedTags  []string
	NearbyIDs    []string
}

type RetrievalPlan struct {
	Intent      string
	Entities    []string
	Tags        []string
	MemoryTypes []string
	Locations   []string
	QuestIDs    []string
	Keywords    []string
	TimeScope   string
	MaxResults  int
}

type NarratorRequest struct {
	PlayerAction     string
	SceneID          string
	StateSummary     string
	RetrievedContext []string
	AllowedEffects   []string
}

type DialogueLine struct {
	NPCID string `json:"npc_id"`
	Text  string `json:"text"`
}

type NarratorResponse struct {
	Narration            string
	NPCDialogue          []DialogueLine
	ProposedEffects      []game.Effect
	SuggestedNextOptions []string
}

type ExtractorRequest struct {
	PlayerAction    string
	Narration       string
	ResolvedChanges []game.StateChangeView
	AllowedTags     []string
}

type MemoryDraft struct {
	Type       string
	Importance int
	Entities   []string
	Tags       []string
	FactsJSON  string
	Text       string
}
