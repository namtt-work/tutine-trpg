package llm

import (
	"context"
	"encoding/json"

	"github.com/namtt/tutine-trpg/internal/game"
)

type Client interface {
	PlanRetrieval(ctx context.Context, req PlannerRequest) (RetrievalPlan, error)
	Narrate(ctx context.Context, req NarratorRequest) (NarratorResponse, error)
	ExtractMemories(ctx context.Context, req ExtractorRequest) ([]MemoryDraft, error)
}

// ToolDefinition describes one engine-backed action the narrator model may
// call mid-turn (e.g. rolling a probability check) before it writes
// narration, instead of inventing an uncertain outcome itself.
type ToolDefinition struct {
	Name        string
	Description string
	// Parameters is a JSON Schema object describing the call's arguments.
	Parameters json.RawMessage
}

type ToolCall struct {
	ID        string
	Name      string
	Arguments json.RawMessage
}

type ToolResult struct {
	ToolCallID string
	Content    string
}

// ToolExecutor runs one tool call against the authoritative game engine and
// returns its result. The LLM never mutates state directly: exec is owned by
// the orchestrator, which is the only layer allowed to touch game.SaveGame.
type ToolExecutor func(ctx context.Context, call ToolCall) (ToolResult, error)

// ToolCapableClient is implemented by providers that support iterative
// function/tool calling. Session prefers this over plain Narrate when the
// configured client supports it; providers that don't implement it (like
// FakeClient) fall back to the single-shot structured-output flow.
type ToolCapableClient interface {
	NarrateWithTools(ctx context.Context, req NarratorRequest, tools []ToolDefinition, exec ToolExecutor) (NarratorResponse, error)
}

type PlannerRequest struct {
	PlayerAction string
	SceneID      string
	AllowedTags  []string
	NearbyIDs    []string
}

type RetrievalPlan struct {
	Intent      string   `json:"intent"`
	Entities    []string `json:"entities"`
	Tags        []string `json:"tags"`
	MemoryTypes []string `json:"memory_types"`
	Locations   []string `json:"locations"`
	QuestIDs    []string `json:"quest_ids"`
	Keywords    []string `json:"keywords"`
	TimeScope   string   `json:"time_scope"`
	MaxResults  int      `json:"max_results"`
}

type NarratorRequest struct {
	PlayerAction       string        `json:"player_action"`
	SceneID            string        `json:"scene_id"`
	StateSummary       string        `json:"state_summary,omitempty"`
	AuthoritativeState NarratorState `json:"authoritative_state"`
	RecentTurns        []RecentTurn  `json:"recent_turns"`
	RetrievedContext   []string      `json:"retrieved_context"`
	AllowedEffects     []string      `json:"allowed_effects"`
}

type NarratorState struct {
	CurrentTurn  int                 `json:"current_turn"`
	CurrentScene string              `json:"current_scene"`
	Player       NarratorPlayerState `json:"player"`
	Inventory    map[string]int      `json:"inventory"`
	WorldFlags   map[string]bool     `json:"world_flags"`
	Cooldowns    map[string]int      `json:"cooldowns"`
}

type NarratorPlayerState struct {
	Identity        string         `json:"identity"`
	Name            string         `json:"name"`
	Traits          []string       `json:"traits"`
	Realm           string         `json:"realm"`
	Stage           int            `json:"stage"`
	HP              int            `json:"hp"`
	MaxHP           int            `json:"max_hp"`
	SpiritualEnergy int            `json:"spiritual_energy"`
	MaxEnergy       int            `json:"max_spiritual_energy"`
	Attack          int            `json:"attack"`
	Defense         int            `json:"defense"`
	Speed           int            `json:"speed"`
	Comprehension   int            `json:"comprehension"`
	Luck            int            `json:"luck"`
	Techniques      []string       `json:"techniques"`
	Artifacts       []string       `json:"artifacts"`
	Relationships   map[string]int `json:"relationships"`
}

type RecentTurn struct {
	PlayerAction    string                 `json:"player_action"`
	Narration       string                 `json:"narration"`
	ResolvedChanges []game.StateChangeView `json:"resolved_changes"`
}

type DialogueLine struct {
	NPCID string `json:"npc_id" yaml:"npc_id"`
	Text  string `json:"text" yaml:"text"`
}

type NarratorResponse struct {
	Narration            string         `json:"narration" yaml:"narration"`
	NPCDialogue          []DialogueLine `json:"npc_dialogue" yaml:"npc_dialogue"`
	ProposedEffects      []game.Effect  `json:"proposed_effects" yaml:"proposed_effects"`
	SuggestedNextOptions []string       `json:"suggested_next_options" yaml:"suggested_next_options"`
}

func (r *NarratorResponse) UnmarshalJSON(data []byte) error {
	type narratorResponseJSON struct {
		Narration            string          `json:"narration"`
		NPCDialogue          json.RawMessage `json:"npc_dialogue"`
		ProposedEffects      []game.Effect   `json:"proposed_effects"`
		SuggestedNextOptions json.RawMessage `json:"suggested_next_options"`
	}
	var raw narratorResponseJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	dialogue, err := parseDialogueLines(raw.NPCDialogue)
	if err != nil {
		return err
	}
	options, err := parseSuggestedNextOptions(raw.SuggestedNextOptions)
	if err != nil {
		return err
	}
	r.Narration = raw.Narration
	r.NPCDialogue = dialogue
	r.ProposedEffects = raw.ProposedEffects
	r.SuggestedNextOptions = options
	return nil
}

func parseDialogueLines(data json.RawMessage) ([]DialogueLine, error) {
	if len(data) == 0 || string(data) == "null" {
		return nil, nil
	}
	var lines []DialogueLine
	if err := json.Unmarshal(data, &lines); err == nil {
		return lines, nil
	}
	var texts []string
	if err := json.Unmarshal(data, &texts); err == nil {
		lines := make([]DialogueLine, len(texts))
		for i, text := range texts {
			lines[i] = DialogueLine{Text: text}
		}
		return lines, nil
	}
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return nil, err
	}
	if text == "" {
		return nil, nil
	}
	return []DialogueLine{{Text: text}}, nil
}

func parseSuggestedNextOptions(data json.RawMessage) ([]string, error) {
	if len(data) == 0 || string(data) == "null" {
		return nil, nil
	}
	var options []string
	if err := json.Unmarshal(data, &options); err == nil {
		return options, nil
	}
	var wrapped struct {
		Options []string `json:"options"`
	}
	if err := json.Unmarshal(data, &wrapped); err != nil {
		return nil, err
	}
	return wrapped.Options, nil
}

type ExtractorRequest struct {
	PlayerAction    string
	Narration       string
	ResolvedChanges []game.StateChangeView
	AllowedTags     []string
}

type MemoryDraft struct {
	Type       string   `json:"type"`
	Importance int      `json:"importance"`
	Entities   []string `json:"entities"`
	Tags       []string `json:"tags"`
	FactsJSON  string   `json:"facts_json"`
	Text       string   `json:"text"`
}
