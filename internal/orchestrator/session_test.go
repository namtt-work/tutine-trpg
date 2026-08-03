package orchestrator

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/namtt/tutine-trpg/internal/game"
	"github.com/namtt/tutine-trpg/internal/llm"
	"github.com/namtt/tutine-trpg/internal/memory"
)

type extractorFailingClient struct {
	llm.FakeClient
}

func (extractorFailingClient) ExtractMemories(context.Context, llm.ExtractorRequest) ([]llm.MemoryDraft, error) {
	return nil, errors.New("extractor unavailable")
}

type plannedRetrievalClient struct {
	llm.FakeClient
	plan llm.RetrievalPlan
}

func (c plannedRetrievalClient) PlanRetrieval(context.Context, llm.PlannerRequest) (llm.RetrievalPlan, error) {
	return c.plan, nil
}

type invalidEffectsClient struct {
	llm.FakeClient
}

func (invalidEffectsClient) Narrate(context.Context, llm.NarratorRequest) (llm.NarratorResponse, error) {
	return llm.NarratorResponse{
		Narration: "Bạn bước tới cổng môn.",
		ProposedEffects: []game.Effect{
			{Type: game.EffectEnergyDelta, Amount: 1},
			{Type: game.EffectRelationshipDelta, Amount: 1},
		},
		SuggestedNextOptions: []string{"Quan sát xung quanh"},
	}, nil
}

type continuityClient struct {
	llm.FakeClient
	requests []llm.NarratorRequest
}

func (c *continuityClient) Narrate(_ context.Context, req llm.NarratorRequest) (llm.NarratorResponse, error) {
	c.requests = append(c.requests, req)
	if len(c.requests) == 1 {
		return llm.NarratorResponse{Narration: "Người gác cổng yêu cầu lệnh bài.", ProposedEffects: []game.Effect{{Type: game.EffectEnergyDelta, TargetID: "player", Amount: -1}}}, nil
	}
	return llm.NarratorResponse{Narration: "Người gác cổng chờ bạn trả lời."}, nil
}

type recordingStore struct {
	query memory.Query
}

func (s *recordingStore) Add(context.Context, memory.Memory) error {
	return nil
}

func (s *recordingStore) Search(_ context.Context, query memory.Query) ([]memory.Hit, error) {
	s.query = query
	return nil, nil
}

func (*recordingStore) Close() error {
	return nil
}

func TestHandleTurnReturnsNarrationAndAdvancesTurn(t *testing.T) {
	ctx := context.Background()
	store, err := memory.NewSQLiteStore(ctx, filepath.Join(t.TempDir(), "game.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	save := game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})
	session := NewSession(save, llm.FakeClient{}, store, []string{"trust", "secret"})

	result, err := session.HandleTurn(ctx, PlayerInput{Text: "ta quan sat cong mon"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Narration == "" {
		t.Fatal("expected narration")
	}
	if session.Save().CurrentTurn != 1 {
		t.Fatalf("turn = %d, want 1", session.Save().CurrentTurn)
	}
}

func TestHandleTurnProvidesRecentNarrationToNextTurn(t *testing.T) {
	client := &continuityClient{}
	session := NewSession(game.NewStarterSave(game.NewGameRequest{Name: "Nam"}), client, &recordingStore{}, nil)

	if _, err := session.HandleTurn(context.Background(), PlayerInput{Text: "ta bước tới"}); err != nil {
		t.Fatalf("first HandleTurn returned error: %v", err)
	}
	if _, err := session.HandleTurn(context.Background(), PlayerInput{Text: "ta trình lệnh bài"}); err != nil {
		t.Fatalf("second HandleTurn returned error: %v", err)
	}

	if got, want := client.requests[1].RecentTurns, []llm.RecentTurn{{PlayerAction: "ta bước tới", Narration: "Người gác cổng yêu cầu lệnh bài.", ResolvedChanges: []game.StateChangeView{{Type: game.EffectEnergyDelta, TargetID: "player", Amount: -1, Message: "linh lực thay đổi"}}}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("RecentTurns = %#v, want %#v", got, want)
	}
	wantState := llm.NarratorState{CurrentTurn: 1, CurrentScene: "loc_outer_gate", Player: llm.NarratorPlayerState{Identity: "Người mới cầu nhập môn tại Thanh Vân Tông", Name: "Nam", Realm: "Luyện Khí", Stage: 1, HP: 30, MaxHP: 30, SpiritualEnergy: 19, MaxEnergy: 20, Attack: 6, Defense: 4, Speed: 5, Comprehension: 5, Luck: 5, Relationships: map[string]int{"npc_luc_thanh_nghi": 0}, Techniques: []string{"Đòn cơ bản"}}, Inventory: map[string]int{}, WorldFlags: map[string]bool{}, Cooldowns: map[string]int{}}
	if got := client.requests[1].AuthoritativeState; !reflect.DeepEqual(got, wantState) {
		t.Fatalf("AuthoritativeState = %#v, want %#v", got, wantState)
	}
}

func TestHandleTurnReturnsResolvedTurnWhenMemoryExtractionFails(t *testing.T) {
	ctx := context.Background()
	store, err := memory.NewSQLiteStore(ctx, filepath.Join(t.TempDir(), "game.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	save := game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})
	session := NewSession(save, extractorFailingClient{}, store, []string{"trust", "secret"})

	result, err := session.HandleTurn(ctx, PlayerInput{Text: "ta quan sat cong mon"})
	if err != nil {
		t.Fatalf("HandleTurn returned error: %v", err)
	}
	if session.Save().CurrentTurn != 1 {
		t.Fatalf("turn = %d, want 1", session.Save().CurrentTurn)
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("warnings = %#v, want extraction warning", result.Warnings)
	}
}

func TestHandleTurnRejectsInvalidLLMEffectsWithoutFailingTurn(t *testing.T) {
	ctx := context.Background()
	store, err := memory.NewSQLiteStore(ctx, filepath.Join(t.TempDir(), "game.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	save := game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})
	session := NewSession(save, invalidEffectsClient{}, store, []string{"trust", "secret"})

	result, err := session.HandleTurn(ctx, PlayerInput{Text: "bắt đầu"})
	if err != nil {
		t.Fatalf("HandleTurn returned error: %v", err)
	}
	if result.Narration != "Bạn bước tới cổng môn." {
		t.Fatalf("narration = %q", result.Narration)
	}
	if session.Save().CurrentTurn != 1 {
		t.Fatalf("turn = %d, want 1", session.Save().CurrentTurn)
	}
	if len(result.StateChanges) != 0 {
		t.Fatalf("state changes = %#v, want invalid effects rejected", result.StateChanges)
	}
	if len(result.Warnings) != 2 {
		t.Fatalf("warnings = %#v, want two rejected effect warnings", result.Warnings)
	}
}

func TestSaveReturnsIndependentCopy(t *testing.T) {
	save := game.NewStarterSave(game.NewGameRequest{Name: "Nam", Traits: []string{"careful"}, CampaignID: "thanh-van-sect"})
	session := NewSession(save, llm.FakeClient{}, nil, nil)

	returnedSave := session.Save()
	returnedSave.Inventory["moonlit_grass"] = 1
	returnedSave.Player.Traits[0] = "reckless"

	currentSave := session.Save()
	if len(currentSave.Inventory) != 0 {
		t.Fatalf("inventory mutation leaked into session: %#v", currentSave.Inventory)
	}
	if got := currentSave.Player.Traits[0]; got != "careful" {
		t.Fatalf("trait = %q, want %q", got, "careful")
	}
}

func TestHandleTurnForwardsAllRetrievalPlanFilters(t *testing.T) {
	store := &recordingStore{}
	plan := llm.RetrievalPlan{
		Entities:    []string{"npc_luc_thanh_nghi"},
		Tags:        []string{"secret"},
		MemoryTypes: []string{"npc_event"},
		Locations:   []string{"loc_outer_gate"},
		QuestIDs:    []string{"quest_entry"},
		Keywords:    []string{"linh can"},
		MaxResults:  3,
	}
	session := NewSession(game.NewStarterSave(game.NewGameRequest{Name: "Nam"}), plannedRetrievalClient{plan: plan}, store, nil)

	_, err := session.HandleTurn(context.Background(), PlayerInput{Text: "ta hoi tham tin tuc"})
	if err != nil {
		t.Fatalf("HandleTurn returned error: %v", err)
	}
	want := memory.Query{
		SaveID:     session.Save().SaveID,
		Entities:   plan.Entities,
		Tags:       plan.Tags,
		Types:      plan.MemoryTypes,
		Locations:  plan.Locations,
		QuestIDs:   plan.QuestIDs,
		Keywords:   plan.Keywords,
		MaxResults: plan.MaxResults,
	}
	if !reflect.DeepEqual(store.query, want) {
		t.Fatalf("search query = %#v, want %#v", store.query, want)
	}
}

func TestHandleTurnRejectsEmptyInputWithoutAdvancingState(t *testing.T) {
	save := game.NewStarterSave(game.NewGameRequest{Name: "Nam"})
	session := NewSession(save, llm.FakeClient{}, &recordingStore{}, nil)

	_, err := session.HandleTurn(context.Background(), PlayerInput{Text: " \t "})
	if err == nil {
		t.Fatal("HandleTurn accepted empty input")
	}
	after := session.Save()
	if after.CurrentTurn != save.CurrentTurn {
		t.Fatalf("turn = %d, want %d", after.CurrentTurn, save.CurrentTurn)
	}
	if after.Player.SpiritualEnergy != save.Player.SpiritualEnergy {
		t.Fatalf("energy = %d, want %d", after.Player.SpiritualEnergy, save.Player.SpiritualEnergy)
	}
}
