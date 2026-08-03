package orchestrator

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/namtt/tutine-trpg/internal/game"
	"github.com/namtt/tutine-trpg/internal/llm"
	"github.com/namtt/tutine-trpg/internal/memory"
)

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
