package main

import (
	"bytes"
	"context"
	"errors"
	"log"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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
	if !strings.Contains(model.View(), "Nam - Luyện Khí tầng 1") {
		t.Fatalf("view missing status:\n%s", model.View())
	}
}

func TestTUITurnErrorIsRendered(t *testing.T) {
	session := &failingSession{save: game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"}), err: errors.New("provider unavailable")}
	model := newTUIModel(session, "test-model")
	var logBuf bytes.Buffer
	model.logger = log.New(&logBuf, "", 0)

	model, cmd := model.handleText(context.Background(), "ta quan sát")
	if cmd == nil {
		t.Fatal("cmd is nil, want async turn command")
	}
	model, _ = model.applyTurnMsg(cmd().(turnFinishedMsg))

	if strings.Contains(model.View(), "provider unavailable") {
		t.Fatalf("view leaks raw internal error:\n%s", model.View())
	}
	if !strings.Contains(model.View(), "Người kể chuyện gặp trục trặc") {
		t.Fatalf("view missing friendly error message:\n%s", model.View())
	}
	if !strings.Contains(logBuf.String(), "provider unavailable") {
		t.Fatalf("logger missing raw error detail: %q", logBuf.String())
	}
}

func TestTUIInitialScreenShowsExampleAndUpToThreeSuggestions(t *testing.T) {
	session := &recordingSession{save: game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})}
	model := newTUIModel(session, "test-model")

	if len(model.suggestions) == 0 || len(model.suggestions) > 3 {
		t.Fatalf("initial suggestions = %#v, want 1-3 scene-appropriate suggestions", model.suggestions)
	}
	view := model.View()
	if !strings.Contains(view, "Ví dụ: ta quan sát") {
		t.Fatalf("view missing example action hint:\n%s", view)
	}
	if !strings.Contains(view, "Bắt đầu:") {
		t.Fatalf("view missing first-run suggestion label:\n%s", view)
	}
}

func TestTUIInvalidNumericChoiceIsRejectedLocally(t *testing.T) {
	session := &recordingSession{save: game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})}
	model := newTUIModel(session, "test-model")
	model.suggestions = []string{"Quan sát xung quanh", "Kiểm tra trạng thái"}

	model, cmd := model.handleText(context.Background(), "9")
	if cmd != nil {
		t.Fatal("cmd is not nil, out-of-range numeric choice must not call HandleTurn")
	}
	if len(session.inputs) != 0 {
		t.Fatalf("inputs = %#v, want none", session.inputs)
	}
	if !strings.Contains(model.View(), "Chọn từ 1 đến 2") {
		t.Fatalf("view missing actionable range message:\n%s", model.View())
	}
}

func TestTUINumberSelectsCommandLikeSuggestionLocally(t *testing.T) {
	session := &recordingSession{save: game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})}
	model := newTUIModel(session, "test-model")
	model.suggestions = []string{"Quan sát xung quanh", "Kiểm tra trạng thái"}

	model, cmd := model.handleText(context.Background(), "2")
	if cmd != nil {
		t.Fatal("cmd is not nil, command-like suggestion should not call HandleTurn")
	}
	if len(session.inputs) != 0 {
		t.Fatalf("inputs = %#v, want none", session.inputs)
	}
	if model.tempView != tempViewStatus {
		t.Fatalf("tempView = %v, want status view opened locally", model.tempView)
	}
}

func TestTUITempViewOpenCloseKeepsExistingDraft(t *testing.T) {
	session := &recordingSession{save: game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})}
	model := newTUIModel(session, "test-model")
	model.input = "ta dang go do choi tiep"

	model, _ = model.handleCommand("/status")
	if model.tempView != tempViewStatus {
		t.Fatalf("tempView = %v, want status", model.tempView)
	}
	if model.input != "ta dang go do choi tiep" {
		t.Fatalf("input = %q, want draft preserved while view is open", model.input)
	}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(tuiModel)
	if model.tempView != tempViewNone {
		t.Fatal("Esc should close the temporary view")
	}
	if model.input != "ta dang go do choi tiep" {
		t.Fatalf("input = %q, want draft preserved after closing view", model.input)
	}
}

func TestTUIPendingTurnBlocksDuplicateSubmission(t *testing.T) {
	session := &recordingSession{
		save:    game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"}),
		results: []*game.TurnResult{{Narration: "ok"}},
	}
	model := newTUIModel(session, "test-model")
	for _, r := range "ta cho" {
		updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		model = updated.(tuiModel)
	}

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(tuiModel)
	if cmd == nil {
		t.Fatal("cmd is nil, want async command for first submission")
	}
	if model.pending == nil {
		t.Fatal("expected pending turn to be set")
	}
	viewBeforeDuplicate := model.View()

	updated2, cmd2 := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model2 := updated2.(tuiModel)
	if cmd2 != nil {
		t.Fatal("duplicate Enter should not trigger a second async command")
	}
	if len(session.inputs) != 0 {
		t.Fatalf("HandleTurn ran before the pending command executed, inputs = %#v", session.inputs)
	}
	if model2.View() != viewBeforeDuplicate {
		t.Fatalf("duplicate submission attempt changed the rendered view:\nbefore:\n%s\nafter:\n%s", viewBeforeDuplicate, model2.View())
	}
}

func TestTUIFailedTurnRestoresInputAndAllowsRetry(t *testing.T) {
	session := &failingSession{save: game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"}), err: errors.New("provider unavailable")}
	model := newTUIModel(session, "test-model")

	model, cmd := model.handleText(context.Background(), "ta hoi de tu gac cong")
	if cmd == nil {
		t.Fatal("cmd is nil, want async turn command")
	}
	model, _ = model.applyTurnMsg(cmd().(turnFinishedMsg))

	if model.input != "ta hoi de tu gac cong" {
		t.Fatalf("input = %q, want original action restored for retry", model.input)
	}
	if !model.recoverable {
		t.Fatal("recoverable should be true after a failed turn")
	}
	if model.pending != nil {
		t.Fatal("pending should be cleared after failure")
	}

	model, cmd = model.handleText(context.Background(), model.input)
	if cmd == nil {
		t.Fatal("retry should submit a new turn")
	}
	if model.pending == nil || model.pending.action != "ta hoi de tu gac cong" {
		t.Fatalf("pending = %#v, want retry to resubmit the same action", model.pending)
	}
}

func TestTUIDoesNotRenderRawIDs(t *testing.T) {
	rawIDs := []string{"loc_outer_gate", "qi_refining"}

	defaultModel := newTUIModel(&recordingSession{save: game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})}, "test-model")
	statusModel := defaultModel
	statusModel.tempView = tempViewStatus

	for _, view := range []string{defaultModel.View(), statusModel.View()} {
		for _, raw := range rawIDs {
			if strings.Contains(view, raw) {
				t.Fatalf("view leaks raw internal id %q:\n%s", raw, view)
			}
		}
	}
}

func TestTUINarrowLayoutKeepsActionAreaAndShowsHiddenHistoryIndicator(t *testing.T) {
	session := &recordingSession{save: game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})}
	model := newTUIModel(session, "test-model")
	model.width = 60
	model.height = 15
	for i := range 20 {
		model.turns = append(model.turns, turnBlock{
			turnNumber:   i + 1,
			playerAction: "hành động",
			narration:    "Một dòng trần thuật khá dài để lấp đầy lịch sử cuộc chơi.",
		})
	}

	view := model.View()
	if !strings.Contains(view, "Enter gửi") {
		t.Fatalf("action area missing from narrow view:\n%s", view)
	}
	if !strings.Contains(view, hiddenHistoryIndicator) {
		t.Fatalf("view missing hidden-history indicator:\n%s", view)
	}
}

func TestTUISaveCommandShowsTurnWithoutRawIDOrPath(t *testing.T) {
	save := game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})
	save.CurrentTurn = 7
	session := &recordingSession{save: save}
	model := newTUIModel(session, "test-model")

	model, cmd := model.handleText(context.Background(), "/save")
	if cmd != nil {
		t.Fatal("cmd is not nil, /save should not call HandleTurn")
	}
	if len(session.inputs) != 0 {
		t.Fatalf("inputs = %#v, want none", session.inputs)
	}
	view := model.View()
	if !strings.Contains(view, "lượt 7") {
		t.Fatalf("view missing turn confirmation:\n%s", view)
	}
	if strings.Contains(view, save.SaveID) {
		t.Fatalf("view leaks raw save id %q:\n%s", save.SaveID, view)
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
