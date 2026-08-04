package main

import (
	"bytes"
	"context"
	"errors"
	"log"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
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
	view := model.View().Content
	if !strings.Contains(view, "Bạn đứng trước cổng môn.") || !strings.Contains(view, "1. Quan sát xung quanh") {
		t.Fatalf("view missing narration or suggestion:\n%s", view)
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
	if view := model.View().Content; !strings.Contains(view, "Bạn quan sát xung quanh.") {
		t.Fatalf("view missing selected turn result:\n%s", view)
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
	if view := model.View().Content; !strings.Contains(view, "Nam - Luyện Khí tầng 1") {
		t.Fatalf("view missing status:\n%s", view)
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

	view := model.View().Content
	if strings.Contains(view, "provider unavailable") {
		t.Fatalf("view leaks raw internal error:\n%s", view)
	}
	if !strings.Contains(view, "Người kể chuyện gặp trục trặc") {
		t.Fatalf("view missing friendly error message:\n%s", view)
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
	view := model.View().Content
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
	if view := model.View().Content; !strings.Contains(view, "Chọn từ 1 đến 2") {
		t.Fatalf("view missing actionable range message:\n%s", view)
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

	updated, _ := model.Update(keyPress(tea.KeyEsc, ""))
	model = updated.(tuiModel)
	if model.tempView != tempViewNone {
		t.Fatal("Esc should close the temporary view")
	}
	if model.input != "ta dang go do choi tiep" {
		t.Fatalf("input = %q, want draft preserved after closing view", model.input)
	}
}

func TestTUIKeyPressEscCancelsRecoverableDraft(t *testing.T) {
	model := newTUIModel(&recordingSession{save: game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})}, "test-model")
	model.recoverable = true
	model.input = "ta hỏi đệ tử"
	model.notice = turnFailureMessage

	updated, cmd := model.Update(keyPress(tea.KeyEsc, ""))
	model = updated.(tuiModel)
	if cmd != nil {
		t.Fatal("Esc should cancel recovery before quitting")
	}
	if model.recoverable || model.input != "" || model.notice != "" {
		t.Fatalf("recovery state = %#v, want cleared", model)
	}
}

func TestTUIKeyPressEscClosesTemporaryViewBeforeRecoverableDraft(t *testing.T) {
	model := newTUIModel(&recordingSession{save: game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})}, "test-model")
	model.tempView = tempViewStatus
	model.recoverable = true
	model.input = "ta hỏi đệ tử"
	model.notice = turnFailureMessage

	updated, cmd := model.Update(keyPress(tea.KeyEsc, ""))
	model = updated.(tuiModel)
	if cmd != nil {
		t.Fatal("Esc should close the temporary view before cancelling recovery")
	}
	if model.tempView != tempViewNone {
		t.Fatal("Esc should close the temporary view")
	}
	if !model.recoverable || model.input != "ta hỏi đệ tử" || model.notice != turnFailureMessage {
		t.Fatalf("recovery state = %#v, want preserved after closing temporary view", model)
	}
}

func TestTUIKeyPressEscQuitsWhenNoLayerIsOpen(t *testing.T) {
	model := newTUIModel(&recordingSession{save: game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})}, "test-model")

	_, cmd := model.Update(keyPress(tea.KeyEsc, ""))
	assertQuitCommand(t, cmd)
}

func TestTUIKeyPressCtrlCQuits(t *testing.T) {
	model := newTUIModel(&recordingSession{save: game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})}, "test-model")

	_, cmd := model.Update(tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl}))
	assertQuitCommand(t, cmd)
}

func TestTUIKeyPressTabReplacesDraftWithSuggestion(t *testing.T) {
	model := newTUIModel(&recordingSession{save: game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})}, "test-model")
	model.suggestions = []string{"Quan sát cổng môn", "Hỏi đệ tử"}
	model.input = "bản nháp"

	updated, cmd := model.Update(keyPress(tea.KeyTab, ""))
	model = updated.(tuiModel)
	if cmd != nil {
		t.Fatal("Tab should update the selected suggestion locally")
	}
	if model.selected != 0 || model.input != "Quan sát cổng môn" {
		t.Fatalf("selected/input = %d/%q, want 0/first suggestion", model.selected, model.input)
	}
}

func TestTUIKeyPressBackspaceRemovesOneVietnameseRune(t *testing.T) {
	model := newTUIModel(&recordingSession{save: game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})}, "test-model")
	model.input = "đạo"
	model.notice = "chọn lại"

	updated, cmd := model.Update(keyPress(tea.KeyBackspace, ""))
	model = updated.(tuiModel)
	if cmd != nil {
		t.Fatal("Backspace should update the draft locally")
	}
	if model.input != "đạ" || model.notice != "" {
		t.Fatalf("input/notice = %q/%q, want Vietnamese rune removed and notice cleared", model.input, model.notice)
	}
}

func TestTUIKeyPressAppendsVietnameseText(t *testing.T) {
	model := newTUIModel(&recordingSession{save: game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})}, "test-model")

	updated, cmd := model.Update(keyPress('đ', "đ"))
	model = updated.(tuiModel)
	if cmd != nil {
		t.Fatal("printable text should update the draft locally")
	}
	if model.input != "đ" {
		t.Fatalf("input = %q, want Vietnamese text appended once", model.input)
	}
}

func TestTUIPendingTurnBlocksDuplicateSubmission(t *testing.T) {
	session := &recordingSession{
		save:    game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"}),
		results: []*game.TurnResult{{Narration: "ok"}},
	}
	model := newTUIModel(session, "test-model")
	for _, r := range "ta cho" {
		updated, _ := model.Update(keyPress(r, string(r)))
		model = updated.(tuiModel)
	}

	updated, cmd := model.Update(keyPress(tea.KeyEnter, ""))
	model = updated.(tuiModel)
	if cmd == nil {
		t.Fatal("cmd is nil, want async command for first submission")
	}
	if model.pending == nil {
		t.Fatal("expected pending turn to be set")
	}
	viewBeforeDuplicate := model.View().Content

	updated2, cmd2 := model.Update(keyPress(tea.KeyEnter, ""))
	model2 := updated2.(tuiModel)
	if cmd2 != nil {
		t.Fatal("duplicate Enter should not trigger a second async command")
	}
	if len(session.inputs) != 0 {
		t.Fatalf("HandleTurn ran before the pending command executed, inputs = %#v", session.inputs)
	}
	if model2.View().Content != viewBeforeDuplicate {
		t.Fatalf("duplicate submission attempt changed the rendered view:\nbefore:\n%s\nafter:\n%s", viewBeforeDuplicate, model2.View().Content)
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

func TestTUIViewUsesAlternateScreen(t *testing.T) {
	model := newTUIModel(&recordingSession{save: game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})}, "test-model")

	view := model.View()
	if view.Content == "" {
		t.Fatal("view content must not be empty")
	}
	if !view.AltScreen {
		t.Fatal("view must request alternate screen")
	}
}

func TestTUIDoesNotRenderRawIDs(t *testing.T) {
	rawIDs := []string{"loc_outer_gate", "qi_refining"}

	defaultModel := newTUIModel(&recordingSession{save: game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})}, "test-model")
	statusModel := defaultModel
	statusModel.tempView = tempViewStatus

	for _, view := range []string{defaultModel.View().Content, statusModel.View().Content} {
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

	view := model.View().Content
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
	view := model.View().Content
	if !strings.Contains(view, "lượt 7") {
		t.Fatalf("view missing turn confirmation:\n%s", view)
	}
	if strings.Contains(view, save.SaveID) {
		t.Fatalf("view leaks raw save id %q:\n%s", save.SaveID, view)
	}
}

func keyPress(code rune, text string) tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: code, Text: text})
}

func assertQuitCommand(t *testing.T, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		t.Fatal("cmd is nil, want quit command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("command message = %T, want tea.QuitMsg", cmd())
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
