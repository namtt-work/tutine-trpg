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
