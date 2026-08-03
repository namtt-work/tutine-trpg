package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/namtt/tutine-trpg/internal/game"
	"github.com/namtt/tutine-trpg/internal/llm"
	"github.com/namtt/tutine-trpg/internal/memory"
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
	allowedTags []string
}

func NewSession(save game.SaveGame, client llm.Client, memories memory.Store, allowedTags []string) *Session {
	return &Session{save: save, client: client, memories: memories, allowedTags: append([]string{}, allowedTags...)}
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

	narration, err := s.client.Narrate(ctx, llm.NarratorRequest{PlayerAction: input.Text, SceneID: s.save.CurrentScene, StateSummary: fmt.Sprintf("%s %s stage %d", s.save.Player.Name, s.save.Player.Realm, s.save.Player.Stage), RetrievedContext: contextLines, AllowedEffects: []string{game.EffectEnergyDelta, game.EffectRelationshipDelta, game.EffectGrantItem}})
	if err != nil {
		return nil, err
	}

	changes, err := game.ApplyEffects(&s.save, narration.ProposedEffects)
	if err != nil {
		return nil, err
	}
	s.save.CurrentTurn++

	warnings := []string{}
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

	return &game.TurnResult{Narration: narration.Narration, StateChanges: changes, SuggestedActions: narration.SuggestedNextOptions, Warnings: warnings}, nil
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
