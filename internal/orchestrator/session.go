package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/namtt/tutine-trpg/internal/game"
	"github.com/namtt/tutine-trpg/internal/llm"
	"github.com/namtt/tutine-trpg/internal/memory"
	"github.com/namtt/tutine-trpg/internal/storage"
)

type PlayerInput struct {
	Text string `json:"text"`
}

type GameSession interface {
	HandleTurn(ctx context.Context, input PlayerInput) (*game.TurnResult, error)
	Save() game.SaveGame
}

type Session struct {
	save        game.SaveGame
	client      llm.Client
	memories    memory.Store
	storage     storage.Store
	allowedTags []string
	recentTurns []llm.RecentTurn
	rollFunc    func() int
}

const maxRecentNarratorTurns = 12

func NewSession(save game.SaveGame, client llm.Client, memories memory.Store, store storage.Store, allowedTags []string) *Session {
	return &Session{save: save, client: client, memories: memories, storage: store, allowedTags: append([]string{}, allowedTags...), rollFunc: defaultRoll}
}

func defaultRoll() int {
	return rand.Intn(100) + 1
}

// narratorTools are the engine-backed actions the narrator model may call
// mid-turn via a ToolCapableClient, before it writes narration.
var narratorTools = []llm.ToolDefinition{
	{
		Name:        "roll_check",
		Description: "Yêu cầu engine thực hiện một lần kiểm tra xác suất dựa trên chỉ số nhân vật trước khi mô tả một hành động có kết quả không chắc chắn.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"stat": {"type": "string", "enum": ["attack", "defense", "speed", "comprehension", "luck"]},
				"difficulty": {"type": "integer", "minimum": 0, "maximum": 20}
			},
			"required": ["stat", "difficulty"]
		}`),
	},
}

// narrate calls Narrate through the tool-calling loop when the configured
// client supports it (ToolCapableClient), so the model can call roll_check
// and see its result before writing narration. Clients without tool support
// (e.g. FakeClient) fall back to the plain single-shot call.
func (s *Session) narrate(ctx context.Context, req llm.NarratorRequest) (llm.NarratorResponse, error) {
	if toolClient, ok := s.client.(llm.ToolCapableClient); ok {
		return toolClient.NarrateWithTools(ctx, req, narratorTools, s.executeTool)
	}
	return s.client.Narrate(ctx, req)
}

func (s *Session) executeTool(_ context.Context, call llm.ToolCall) (llm.ToolResult, error) {
	switch call.Name {
	case "roll_check":
		var args struct {
			Stat       string `json:"stat"`
			Difficulty int    `json:"difficulty"`
		}
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			return llm.ToolResult{}, fmt.Errorf("invalid roll_check arguments: %w", err)
		}
		result, err := game.ResolveCheck(s.save, game.CheckRequest{Stat: args.Stat, Difficulty: args.Difficulty}, s.rollFunc())
		if err != nil {
			return llm.ToolResult{}, err
		}
		payload, err := json.Marshal(result)
		if err != nil {
			return llm.ToolResult{}, err
		}
		return llm.ToolResult{ToolCallID: call.ID, Content: string(payload)}, nil
	default:
		return llm.ToolResult{}, fmt.Errorf("unknown tool %q", call.Name)
	}
}

func (s *Session) Save() game.SaveGame {
	return s.save.Clone()
}

func (s *Session) HandleTurn(ctx context.Context, input PlayerInput) (*game.TurnResult, error) {
	input.Text = strings.TrimSpace(input.Text)
	if input.Text == "" {
		return nil, errors.New("player input is required")
	}

	plan, err := s.client.PlanRetrieval(ctx, llm.PlannerRequest{PlayerAction: input.Text, SceneID: s.save.CurrentScene, AllowedTags: s.allowedTags, NearbyIDs: []string{"player"}})
	if err != nil {
		return nil, err
	}

	hits, err := s.memories.Search(ctx, memory.Query{SaveID: s.save.SaveID, Entities: plan.Entities, Tags: plan.Tags, Types: plan.MemoryTypes, Locations: plan.Locations, QuestIDs: plan.QuestIDs, Keywords: plan.Keywords, MaxResults: plan.MaxResults})
	if err != nil {
		return nil, err
	}
	contextLines := make([]string, 0, len(hits))
	for _, hit := range hits {
		contextLines = append(contextLines, hit.Memory.Summary)
	}

	narration, err := s.narrate(ctx, llm.NarratorRequest{PlayerAction: input.Text, SceneID: s.save.CurrentScene, AuthoritativeState: narratorState(s.save), RecentTurns: copyRecentTurns(s.recentTurns), RetrievedContext: contextLines, AllowedEffects: []string{game.EffectEnergyDelta, game.EffectRelationshipDelta, game.EffectGrantItem}})
	if err != nil {
		return nil, err
	}

	changes, warnings := s.applyProposedEffects(narration.ProposedEffects)
	s.save.CurrentTurn++
	s.rememberTurn(llm.RecentTurn{PlayerAction: input.Text, Narration: narration.Narration, ResolvedChanges: changes})

	drafts, err := s.client.ExtractMemories(ctx, llm.ExtractorRequest{PlayerAction: input.Text, Narration: narration.Narration, ResolvedChanges: changes, AllowedTags: s.allowedTags})
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("memory extraction failed: %v", err))
	} else {
		for i, draft := range drafts {
			memoryID := fmt.Sprintf("%s_turn_%d_%d", s.save.SaveID, s.save.CurrentTurn, i)
			if err := s.memories.Add(ctx, memory.Memory{ID: memoryID, SaveID: s.save.SaveID, CampaignID: s.save.CampaignID, Turn: s.save.CurrentTurn, Type: draft.Type, Importance: draft.Importance, Text: draft.Text, Summary: draft.Text, Entities: draft.Entities, Tags: filterTags(draft.Tags, s.allowedTags), FactsJSON: draft.FactsJSON}); err != nil {
				warnings = append(warnings, fmt.Sprintf("memory persistence failed: %v", err))
			}
		}
	}

	if err := s.storage.SaveSnapshot(ctx, s.save); err != nil {
		warnings = append(warnings, fmt.Sprintf("save persistence failed: %v", err))
	}
	if err := s.storage.AppendEvent(ctx, s.save.SaveID, storage.Event{
		Turn:            s.save.CurrentTurn,
		Type:            storage.EventTypeTurnResolved,
		PlayerAction:    input.Text,
		ResolvedEffects: changes,
		Narration:       narration.Narration,
		Warnings:        warnings,
		CreatedAt:       time.Now().UTC(),
	}); err != nil {
		warnings = append(warnings, fmt.Sprintf("event log write failed: %v", err))
	}

	return &game.TurnResult{Narration: narration.Narration, StateChanges: changes, SuggestedActions: narration.SuggestedNextOptions, Warnings: warnings}, nil
}

func (s *Session) rememberTurn(turn llm.RecentTurn) {
	if strings.TrimSpace(turn.Narration) == "" {
		return
	}
	s.recentTurns = append(s.recentTurns, turn)
	if len(s.recentTurns) > maxRecentNarratorTurns {
		s.recentTurns = s.recentTurns[len(s.recentTurns)-maxRecentNarratorTurns:]
	}
}

func narratorState(save game.SaveGame) llm.NarratorState {
	snapshot := save.Clone()
	return llm.NarratorState{
		CurrentTurn:  snapshot.CurrentTurn,
		CurrentScene: snapshot.CurrentScene,
		Player: llm.NarratorPlayerState{
			Identity:        playerIdentity(snapshot),
			Name:            snapshot.Player.Name,
			Traits:          append([]string(nil), snapshot.Player.Traits...),
			Realm:           displayRealm(snapshot.Player.Realm),
			Stage:           snapshot.Player.Stage,
			HP:              snapshot.Player.HP,
			MaxHP:           snapshot.Player.MaxHP,
			SpiritualEnergy: snapshot.Player.SpiritualEnergy,
			MaxEnergy:       snapshot.Player.MaxEnergy,
			Attack:          snapshot.Player.Stats.Attack,
			Defense:         snapshot.Player.Stats.Defense,
			Speed:           snapshot.Player.Stats.Speed,
			Comprehension:   snapshot.Player.Stats.Comprehension,
			Luck:            snapshot.Player.Stats.Luck,
			Techniques:      displayTechniques(snapshot.Player.Techniques),
			Artifacts:       snapshot.Player.Artifacts,
			Relationships:   snapshot.Player.Relationships,
		},
		Inventory:  snapshot.Inventory,
		WorldFlags: snapshot.WorldFlags,
		Cooldowns:  snapshot.Cooldowns,
	}
}

func playerIdentity(save game.SaveGame) string {
	if save.CurrentScene == "loc_outer_gate" {
		return "Người mới cầu nhập môn tại Thanh Vân Tông"
	}
	return "Thân phận chưa có thay đổi nào được engine xác nhận"
}

func displayRealm(realm string) string {
	switch realm {
	case "qi_refining":
		return "Luyện Khí"
	default:
		return realm
	}
}

func displayTechniques(techniques []string) []string {
	display := make([]string, 0, len(techniques))
	for _, technique := range techniques {
		switch technique {
		case "basic_strike":
			display = append(display, "Đòn cơ bản")
		default:
			display = append(display, technique)
		}
	}
	return display
}

func copyRecentTurns(turns []llm.RecentTurn) []llm.RecentTurn {
	copyTurns := make([]llm.RecentTurn, len(turns))
	for i, turn := range turns {
		copyTurns[i] = turn
		copyTurns[i].ResolvedChanges = append([]game.StateChangeView(nil), turn.ResolvedChanges...)
	}
	return copyTurns
}

func (s *Session) applyProposedEffects(effects []game.Effect) ([]game.StateChangeView, []string) {
	changes := []game.StateChangeView{}
	warnings := []string{}
	for _, effect := range effects {
		effectChanges, err := game.ApplyEffects(&s.save, []game.Effect{effect})
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("rejected llm effect %q: %v", effect.Type, err))
			continue
		}
		changes = append(changes, effectChanges...)
	}
	return changes, warnings
}

func filterTags(tags []string, allowed []string) []string {
	allowedSet := map[string]bool{}
	for _, tag := range allowed {
		allowedSet[tag] = true
	}
	out := []string{}
	for _, tag := range tags {
		if allowedSet[tag] {
			out = append(out, tag)
		}
	}
	return out
}
