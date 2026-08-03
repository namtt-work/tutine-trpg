package orchestrator

import (
	"context"
	"errors"
	"path/filepath"
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
