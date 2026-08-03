package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/namtt/tutine-trpg/internal/game"
	"github.com/namtt/tutine-trpg/internal/orchestrator"
)

func TestTUIFreeTextSubmitsTurn(t *testing.T) {
	session := &recordingSession{
		save: game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"}),
		results: []*game.TurnResult{{
			Narration:        "Bạn đứng trước cổng môn.",
			SuggestedActions: []string{"Quan sát xung quanh"},
		}},
	}
	model := newTUIModel(session, "test-model")

	model, cmd := model.handleText(context.Background(), "ta quan sát cổng môn")
	if cmd == nil {
		t.Fatal("cmd is nil, want async turn command")
	}
	model, _ = model.applyTurnMsg(cmd().(turnFinishedMsg))

	if got, want := session.inputs, []string{"ta quan sát cổng môn"}; !equalStrings(got, want) {
		t.Fatalf("inputs = %#v, want %#v", got, want)
	}
	if !strings.Contains(model.View(), "Bạn đứng trước cổng môn.") || !strings.Contains(model.View(), "1. Quan sát xung quanh") {
		t.Fatalf("view missing narration or suggestion:\n%s", model.View())
	}
}

func TestTUINumberSelectsRoleplaySuggestion(t *testing.T) {
	session := &recordingSession{
		save:    game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"}),
		results: []*game.TurnResult{{Narration: "Bạn quan sát xung quanh."}},
	}
	model := newTUIModel(session, "test-model")
	model.suggestions = []string{"Quan sát xung quanh", "Kiểm tra trạng thái"}

	model, cmd := model.handleText(context.Background(), "1")
	if cmd == nil {
		t.Fatal("cmd is nil, want async turn command")
	}
	model, _ = model.applyTurnMsg(cmd().(turnFinishedMsg))

	if got, want := session.inputs, []string{"Quan sát xung quanh"}; !equalStrings(got, want) {
		t.Fatalf("inputs = %#v, want %#v", got, want)
	}
	if !strings.Contains(model.View(), "Bạn quan sát xung quanh.") {
		t.Fatalf("view missing selected turn result:\n%s", model.View())
	}
}

func TestTUIStatusCommandDoesNotSubmitTurn(t *testing.T) {
	session := &recordingSession{save: game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})}
	model := newTUIModel(session, "test-model")

	model, cmd := model.handleText(context.Background(), "/status")
	if cmd != nil {
		t.Fatal("cmd is not nil, /status should not call LLM")
	}
	if len(session.inputs) != 0 {
		t.Fatalf("inputs = %#v, want none", session.inputs)
	}
	if !strings.Contains(model.View(), "Nam - qi_refining tầng 1") {
		t.Fatalf("view missing status:\n%s", model.View())
	}
}

func TestTUITurnErrorIsRendered(t *testing.T) {
	session := &failingSession{save: game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"}), err: errors.New("provider unavailable")}
	model := newTUIModel(session, "test-model")

	model, cmd := model.handleText(context.Background(), "ta quan sát")
	if cmd == nil {
		t.Fatal("cmd is nil, want async turn command")
	}
	model, _ = model.applyTurnMsg(cmd().(turnFinishedMsg))

	if !strings.Contains(model.View(), "provider unavailable") {
		t.Fatalf("view missing error:\n%s", model.View())
	}
}

type failingSession struct {
	save game.SaveGame
	err  error
}

func (s *failingSession) HandleTurn(ctx context.Context, input orchestrator.PlayerInput) (*game.TurnResult, error) {
	return nil, s.err
}

func (s *failingSession) Save() game.SaveGame {
	return s.save
}
