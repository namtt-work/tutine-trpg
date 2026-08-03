package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/namtt/tutine-trpg/internal/game"
	"github.com/namtt/tutine-trpg/internal/orchestrator"
)

func TestBuildOfflineSession(t *testing.T) {
	session, cleanup, err := buildOfflineSession(t.TempDir(), "Nam")
	if err != nil {
		t.Fatalf("buildOfflineSession returned error: %v", err)
	}
	defer cleanup()
	if session.Save().Player.Name != "Nam" {
		t.Fatalf("player name = %q, want Nam", session.Save().Player.Name)
	}
}

func TestBuildOfflineSessionUsesDistinctSaveStorage(t *testing.T) {
	dataDir := t.TempDir()
	first, firstCleanup, err := buildOfflineSession(dataDir, "Nam")
	if err != nil {
		t.Fatalf("build first offline session: %v", err)
	}
	firstResult, err := first.HandleTurn(context.Background(), orchestrator.PlayerInput{Text: "ta quan sat cong mon"})
	firstCleanup()
	if err != nil {
		t.Fatalf("handle first turn: %v", err)
	}
	if len(firstResult.Warnings) != 0 {
		t.Fatalf("first turn warnings = %#v", firstResult.Warnings)
	}

	second, secondCleanup, err := buildOfflineSession(dataDir, "Nam")
	if err != nil {
		t.Fatalf("build second offline session: %v", err)
	}
	defer secondCleanup()
	secondResult, err := second.HandleTurn(context.Background(), orchestrator.PlayerInput{Text: "ta quan sat cong mon"})
	if err != nil {
		t.Fatalf("handle second turn: %v", err)
	}
	if first.Save().SaveID == second.Save().SaveID {
		t.Fatalf("save IDs match: %q", first.Save().SaveID)
	}
	if len(secondResult.Warnings) != 0 {
		t.Fatalf("second turn warnings = %#v", secondResult.Warnings)
	}
}

func TestRunInteractiveHandlesHelpWithoutAdvancingTurn(t *testing.T) {
	session, cleanup, err := buildOfflineSession(t.TempDir(), "Nam")
	if err != nil {
		t.Fatalf("buildOfflineSession returned error: %v", err)
	}
	defer cleanup()

	var out bytes.Buffer
	err = runInteractive(context.Background(), session, strings.NewReader("/help\n/exit\n"), &out)
	if err != nil {
		t.Fatalf("runInteractive returned error: %v", err)
	}
	if session.Save().CurrentTurn != 0 {
		t.Fatalf("turn advanced to %d for /help", session.Save().CurrentTurn)
	}
	if !strings.Contains(out.String(), "/status") || !strings.Contains(out.String(), "/help") {
		t.Fatalf("help output missing commands:\n%s", out.String())
	}
}

func TestRunInteractiveRejectsUnknownSlashCommand(t *testing.T) {
	session, cleanup, err := buildOfflineSession(t.TempDir(), "Nam")
	if err != nil {
		t.Fatalf("buildOfflineSession returned error: %v", err)
	}
	defer cleanup()

	var out bytes.Buffer
	err = runInteractive(context.Background(), session, strings.NewReader("/wat\n/exit\n"), &out)
	if err != nil {
		t.Fatalf("runInteractive returned error: %v", err)
	}
	if session.Save().CurrentTurn != 0 {
		t.Fatalf("turn advanced to %d for unknown command", session.Save().CurrentTurn)
	}
	if !strings.Contains(out.String(), "Không hiểu lệnh /wat") {
		t.Fatalf("unknown command output mismatch:\n%s", out.String())
	}
}

func TestRunInteractiveWritesStateChangesToOutput(t *testing.T) {
	session := &recordingSession{
		results: []*game.TurnResult{{
			Narration:    "Bạn nhận linh thạch.",
			StateChanges: []game.StateChangeView{{Type: game.EffectGrantItem, TargetID: "low_spirit_stone", Amount: 1}},
		}},
		save: game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"}),
	}

	var out bytes.Buffer
	err := runInteractive(context.Background(), session, strings.NewReader("nhặt linh thạch\n/exit\n"), &out)
	if err != nil {
		t.Fatalf("runInteractive returned error: %v", err)
	}
	if !strings.Contains(out.String(), "- grant_item: +1") {
		t.Fatalf("state change was not written to provided output:\n%s", out.String())
	}
}

func TestRunInteractiveMapsNumberToSuggestedAction(t *testing.T) {
	session := &recordingSession{
		results: []*game.TurnResult{
			{
				Narration:        "Bạn đứng trước cổng môn.",
				SuggestedActions: []string{"Quan sát xung quanh", "Hỏi đệ tử gác cổng", "Kiểm tra trạng thái"},
			},
			{
				Narration: "Bạn kiểm tra trạng thái.",
			},
		},
		save: game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"}),
	}

	var out bytes.Buffer
	err := runInteractive(context.Background(), session, strings.NewReader("ta quan sát cổng môn\n3\n/exit\n"), &out)
	if err != nil {
		t.Fatalf("runInteractive returned error: %v", err)
	}
	if got, want := session.inputs, []string{"ta quan sát cổng môn", "Kiểm tra trạng thái"}; !equalStrings(got, want) {
		t.Fatalf("inputs = %#v, want %#v", got, want)
	}
}

type recordingSession struct {
	save    game.SaveGame
	inputs  []string
	results []*game.TurnResult
}

func (s *recordingSession) HandleTurn(ctx context.Context, input orchestrator.PlayerInput) (*game.TurnResult, error) {
	s.inputs = append(s.inputs, input.Text)
	s.save.CurrentTurn++
	if len(s.results) == 0 {
		return &game.TurnResult{}, nil
	}
	result := s.results[0]
	s.results = s.results[1:]
	return result, nil
}

func (s *recordingSession) Save() game.SaveGame {
	return s.save
}

func equalStrings(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
