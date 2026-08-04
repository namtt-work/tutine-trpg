package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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
	model, _ = model.applyTurnMsg(runTurnCommand(t, cmd))

	if got, want := session.inputs, []string{"ta quan sát cổng môn"}; !equalStrings(got, want) {
		t.Fatalf("inputs = %#v, want %#v", got, want)
	}
	view := model.View().Content
	if !strings.Contains(view, "Bạn đứng trước cổng môn.") || !strings.Contains(view, "1. Quan sát xung quanh") {
		t.Fatalf("view missing narration or suggestion:\n%s", view)
	}
}

func TestTruncateCellsKeepsVietnameseTextOnOneLine(t *testing.T) {
	got := truncateCells("Hỏi đệ tử gác cổng", 12)
	if got != "Hỏi đệ tử g…" {
		t.Fatalf("truncateCells = %q, want %q", got, "Hỏi đệ tử g…")
	}
	if strings.Contains(got, "\n") {
		t.Fatalf("truncateCells must not add a line break: %q", got)
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
	model, _ = model.applyTurnMsg(runTurnCommand(t, cmd))

	if got, want := session.inputs, []string{"Quan sát xung quanh"}; !equalStrings(got, want) {
		t.Fatalf("inputs = %#v, want %#v", got, want)
	}
	if view := model.View().Content; !strings.Contains(view, "Bạn quan sát xung quanh.") {
		t.Fatalf("view missing selected turn result:\n%s", view)
	}
}

func TestTUIResolvedSuggestionsCapToThreeNonEmptyItems(t *testing.T) {
	model := newTUIModel(&recordingSession{save: game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})}, "test-model")
	model, _ = model.applyTurnMsg(turnFinishedMsg{input: "ta quan sát", result: &game.TurnResult{
		Narration:        "Mây trôi qua cổng môn.",
		SuggestedActions: []string{"", "Quan sát cổng môn", "Hỏi đệ tử", "Kiểm tra trạng thái", "Rời đi"},
	}})

	want := []string{"Quan sát cổng môn", "Hỏi đệ tử", "Kiểm tra trạng thái"}
	if !equalStrings(model.suggestions, want) {
		t.Fatalf("suggestions = %#v, want %#v", model.suggestions, want)
	}
	model = model.handleTab()
	if got := model.editor.Value(); got != want[0] {
		t.Fatalf("Tab draft = %q, want first capped suggestion %q", got, want[0])
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
	model, _ = model.applyTurnMsg(runTurnCommand(t, cmd))

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

func TestTUITextareaKeepsMultilineVietnameseDraft(t *testing.T) {
	model := newTUIModel(&recordingSession{save: game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})}, "test-model")

	updated, _ := model.Update(keyPress('đ', "đ"))
	model = updated.(tuiModel)
	updated, cmd := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter, Mod: tea.ModShift}))
	model = updated.(tuiModel)
	if cmd != nil {
		t.Fatal("Shift+Enter should insert a newline instead of submitting")
	}
	updated, _ = model.Update(keyPress('ạ', "ạ"))
	model = updated.(tuiModel)

	if got, want := model.editor.Value(), "đ\nạ"; got != want {
		t.Fatalf("textarea draft = %q, want %q", got, want)
	}
	if model.pending != nil {
		t.Fatal("Shift+Enter should not submit a turn")
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

func TestTUIPendingTurnStartsSpinnerAndDisablesEditor(t *testing.T) {
	session := &recordingSession{save: game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})}
	model := newTUIModel(session, "test-model")
	model.editor.SetValue("ta quan sát")
	model.input = model.editor.Value()

	model, cmd := model.handleText(context.Background(), model.editor.Value())
	if model.pending == nil || model.editor.Focused() {
		t.Fatal("submitting should mark the turn pending and disable the editor")
	}
	if cmd == nil {
		t.Fatal("submitting should schedule both turn completion and spinner work")
	}
	if model.spinner.View() == "" {
		t.Fatal("pending model should own a spinner")
	}
}

func TestTUIFailedTurnRestoresInputAndAllowsRetry(t *testing.T) {
	session := &failingSession{save: game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"}), err: errors.New("provider unavailable")}
	model := newTUIModel(session, "test-model")

	model, cmd := model.handleText(context.Background(), "ta hoi de tu gac cong")
	if cmd == nil {
		t.Fatal("cmd is nil, want async turn command")
	}
	model, _ = model.applyTurnMsg(runTurnCommand(t, cmd))

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

func TestTUINarrowLayoutKeepsActionAreaAndShowsViewportHistory(t *testing.T) {
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

	model.syncLayout()
	model.refreshTranscript(false)
	view := model.View().Content
	if model.editor.Height() < 1 || model.editor.Height() > 3 {
		t.Fatalf("editor height = %d, want bounded visible action editor", model.editor.Height())
	}
	if strings.Contains(view, "Lịch sử cũ hơn đang ẩn.") {
		t.Fatalf("view must not destructively clip history:\n%s", view)
	}
	if got, want := model.historyText(session.save), model.viewport.GetContent(); got != want {
		t.Fatalf("viewport content was truncated\ngot: %q\nwant: %q", got, want)
	}
}

func TestTUINarrowNormalHeightFitsFooterAndEditor(t *testing.T) {
	model := newTUIModel(&recordingSession{save: game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})}, "test-model")
	model.width, model.height = 60, 14
	model.syncLayout()
	view := model.View().Content
	if model.editor.Height() != 3 {
		t.Fatalf("editor height = %d, want bounded normal-layout height", model.editor.Height())
	}
	if rows := strings.Count(view, "\n") + 1; rows > model.height {
		t.Fatalf("rendered rows = %d, exceed terminal height %d:\n%s", rows, model.height, view)
	}
}

func TestTUIWideViewportUsesTranscriptColumnWidth(t *testing.T) {
	model := newTUIModel(&recordingSession{save: game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})}, "test-model")
	model.width, model.height = 100, 20
	model.turns = append(model.turns, turnBlock{turnNumber: 1, narration: strings.Repeat("một dòng dài ", 20)})
	model.syncLayout()
	model.refreshTranscript(true)
	if model.viewport.Width() >= model.width {
		t.Fatalf("wide viewport width = %d, want a narrower transcript column than %d", model.viewport.Width(), model.width)
	}
}

func TestTUIWideVisualShellLabelsFourRegions(t *testing.T) {
	model := newTUIModel(&recordingSession{save: game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})}, "test-model")
	model.width, model.height = 100, 30
	view := model.View().Content
	for _, label := range []string{"NHẬT KÝ HÀNH TRÌNH", "NHÂN VẬT", "BẠN MUỐN LÀM GÌ?"} {
		if !strings.Contains(view, label) {
			t.Fatalf("wide visual shell missing %q:\n%s", label, view)
		}
	}
}

func TestTUIVisualShellFitsWideHeightBudget(t *testing.T) {
	model := newTUIModel(&recordingSession{save: game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})}, "test-model")
	model.width, model.height = 100, 30
	if rows := strings.Count(model.View().Content, "\n") + 1; rows > model.height {
		t.Fatalf("rendered rows = %d, exceed height %d:\n%s", rows, model.height, model.View().Content)
	}
}

func TestTUINarrowVisualShellKeepsThreeSeparateSuggestions(t *testing.T) {
	model := newTUIModel(&recordingSession{save: game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})}, "test-model")
	model.width, model.height = 60, 18
	view := model.View().Content
	for _, line := range []string{"NHẬT KÝ HÀNH TRÌNH", "HÀNH ĐỘNG", "1. Quan sát cổng môn", "2. Hỏi đệ tử gác cổng", "3. Kiểm tra trạng thái"} {
		if !strings.Contains(view, line) {
			t.Fatalf("narrow visual shell missing %q:\n%s", line, view)
		}
	}
}

func TestTUIVisualShellFitsNarrowHeightBudget(t *testing.T) {
	model := newTUIModel(&recordingSession{save: game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})}, "test-model")
	model.width, model.height = 60, 18
	if rows := strings.Count(model.View().Content, "\n") + 1; rows > model.height {
		t.Fatalf("rendered rows = %d, exceed height %d:\n%s", rows, model.height, model.View().Content)
	}
}

func TestTUICompactShellShowsOnlySelectedSuggestion(t *testing.T) {
	model := newTUIModel(&recordingSession{save: game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})}, "test-model")
	model.width, model.height = 60, 10
	view := model.View().Content
	if !strings.Contains(view, "1. Quan sát cổng môn · Tab 1/3") {
		t.Fatalf("compact view missing selected-suggestion affordance:\n%s", view)
	}
	if strings.Contains(view, "2. Hỏi đệ tử gác cổng") {
		t.Fatalf("compact view rendered an unselected suggestion:\n%s", view)
	}
	if rows := strings.Count(view, "\n") + 1; rows > model.height {
		t.Fatalf("rendered rows = %d, exceed height %d:\n%s", rows, model.height, view)
	}
}

func TestTUICompactShellWinsOverWideWidthAtShortHeights(t *testing.T) {
	for _, height := range []int{10, 11, 12, 13} {
		t.Run(fmt.Sprintf("100x%d", height), func(t *testing.T) {
			model := newTUIModel(&recordingSession{save: game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})}, "test-model")
			model.width, model.height = 100, height
			view := model.View().Content
			if strings.Contains(view, "NHÂN VẬT") || strings.Contains(view, "2. Hỏi đệ tử gác cổng") {
				t.Fatalf("short terminal must use compact shell:\n%s", view)
			}
			if rows := strings.Count(view, "\n") + 1; rows > height {
				t.Fatalf("rendered rows = %d, exceed height %d:\n%s", rows, height, view)
			}
		})
	}
}

func TestTUIResizeFallbackOmitsInteractiveControls(t *testing.T) {
	model := newTUIModel(&recordingSession{save: game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})}, "test-model")
	model.width, model.height = 60, 9
	view := model.View().Content
	if !strings.Contains(view, "tối thiểu 10 hàng") {
		t.Fatalf("fallback missing resize instruction:\n%s", view)
	}
	for _, forbidden := range []string{"Bạn muốn làm gì?", "Enter gửi", "Bắt đầu:"} {
		if strings.Contains(view, forbidden) {
			t.Fatalf("fallback must omit %q:\n%s", forbidden, view)
		}
	}
}

func TestTUIPaletteFitsFullNarrowHeightBudget(t *testing.T) {
	model := newTUIModel(&recordingSession{save: game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})}, "test-model")
	model.width, model.height, model.paletteOpen = 60, 18, true
	if rows := strings.Count(model.View().Content, "\n") + 1; rows > model.height {
		t.Fatalf("palette rows = %d, exceed height %d:\n%s", rows, model.height, model.View().Content)
	}
}

func TestTUIRecoveryFitsCompactHeightBudget(t *testing.T) {
	model := newTUIModel(&recordingSession{save: game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})}, "test-model")
	model.width, model.height = 60, 10
	model.recoverable = true
	model.notice = turnFailureMessage
	model.editor.SetValue("ta hỏi đệ tử")
	if rows := strings.Count(model.View().Content, "\n") + 1; rows > model.height {
		t.Fatalf("recovery rows = %d, exceed height %d:\n%s", rows, model.height, model.View().Content)
	}
}

func TestTUICompactRecoveryHidesSuggestionsAndRetainsEditor(t *testing.T) {
	model := newTUIModel(&recordingSession{save: game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})}, "test-model")
	model.width, model.height = 60, 10
	model.recoverable = true
	model.notice = turnFailureMessage
	model.editor.SetValue("ta hỏi đệ tử")
	view := model.View().Content
	if strings.Contains(view, "1. Quan sát cổng môn") || !strings.Contains(view, "ta hỏi đệ tử") {
		t.Fatalf("compact recovery should replace suggestions with retained editor:\n%s", view)
	}
}

func TestTUIVisualLayoutsShowUnseenTurnWithinHeightBudget(t *testing.T) {
	for _, size := range []struct{ width, height int }{{60, 18}, {60, 17}, {60, 14}, {60, 10}} {
		t.Run(fmt.Sprintf("%dx%d", size.width, size.height), func(t *testing.T) {
			model := newTUIModel(&recordingSession{save: game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})}, "test-model")
			model.width, model.height = size.width, size.height
			model.syncLayout()
			model.refreshTranscript(true)
			model.viewport.GotoTop()
			model.turns = append(model.turns, turnBlock{turnNumber: 1, narration: "lượt mới"})
			model.refreshTranscript(false)
			view := model.View().Content
			wantIndicator := "↓ Có lượt mới · End để xem"
			if size.height == 10 {
				wantIndicator = "End mới"
			}
			if !strings.Contains(view, wantIndicator) {
				t.Fatalf("unseen indicator %q missing:\n%s", wantIndicator, view)
			}
			if rows := strings.Count(view, "\n") + 1; rows > model.height {
				t.Fatalf("rendered rows = %d, exceed height %d:\n%s", rows, model.height, view)
			}
		})
	}
}

func TestTUIVisualLayoutsKeepNoticeAndLongDraftWithinBudget(t *testing.T) {
	for _, size := range []struct{ width, height int }{{60, 18}, {60, 17}, {60, 14}} {
		t.Run(fmt.Sprintf("%dx%d", size.width, size.height), func(t *testing.T) {
			model := newTUIModel(&recordingSession{save: game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})}, "test-model")
			model.width, model.height = size.width, size.height
			model.notice = "Thông báo rất dài cần luôn ở trong vùng composer cố định."
			model.editor.SetValue(strings.Repeat("hành động dài ", 20))
			view := model.View().Content
			if !strings.Contains(view, "Thông báo rất dài") {
				t.Fatalf("notice missing:\n%s", view)
			}
			if rows := strings.Count(view, "\n") + 1; rows > model.height {
				t.Fatalf("rendered rows = %d, exceed height %d:\n%s", rows, model.height, view)
			}
		})
	}
}

func TestTUIVisualStateMatrixFitsTargetLayouts(t *testing.T) {
	targets := []struct{ width, height int }{{60, 18}, {60, 17}, {60, 14}, {60, 10}}
	states := []struct {
		name      string
		configure func(*tuiModel)
		contains  []string
	}{
		{"palette", func(m *tuiModel) { m.paletteOpen = true; m.palette.SetFilterText("trạng thái") }, []string{"LỆNH", "trạng thái", "/status", "Enter chọn"}},
		{"pending", func(m *tuiModel) { m.pending = &pendingTurn{turnNumber: 1, action: "hành động"} }, []string{"ĐANG XỬ LÝ LƯỢT", "PgUp/PgDn"}},
		{"recovery", func(m *tuiModel) {
			m.recoverable = true
			m.notice = strings.Repeat("Thông báo lỗi ", 12)
			m.editor.SetValue("bản nháp")
		}, []string{"THỬ LẠI HÀNH ĐỘNG", "bản nháp"}},
		{"locked", func(m *tuiModel) { m.locked = true; m.notice = ambiguousCompletionMessage }, []string{"/exit"}},
		{"temporary", func(m *tuiModel) { m.tempView = tempViewHelp }, []string{"Esc quay lại"}},
	}
	for _, target := range targets {
		for _, state := range states {
			t.Run(fmt.Sprintf("%s-%dx%d", state.name, target.width, target.height), func(t *testing.T) {
				model := newTUIModel(&recordingSession{save: game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})}, "test-model")
				model.width, model.height = target.width, target.height
				state.configure(&model)
				view := model.View().Content
				for _, required := range state.contains {
					if !strings.Contains(view, required) {
						t.Fatalf("state control %q missing:\n%s", required, view)
					}
				}
				if rows := strings.Count(view, "\n") + 1; rows > target.height {
					t.Fatalf("rendered rows = %d, exceed height %d:\n%s", rows, target.height, view)
				}
				for _, line := range strings.Split(view, "\n") {
					if cells := lipgloss.Width(line); cells > target.width {
						t.Fatalf("line width = %d, exceed width %d: %q", cells, target.width, line)
					}
				}
			})
		}
	}
}

func TestTUIAmbiguousCompletionLocksSubmission(t *testing.T) {
	session := &recordingSession{save: game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})}
	model := newTUIModel(session, "test-model")
	model, _ = model.applyTurnMsg(turnFinishedMsg{input: "ta quan sát"})
	if !model.locked || model.editor.Focused() || model.input != "" {
		t.Fatalf("ambiguous result should lock and clear input: %#v", model)
	}
	updated, cmd := model.Update(keyPress('a', "a"))
	model = updated.(tuiModel)
	if cmd != nil || model.editor.Value() != "" {
		t.Fatal("locked input must ignore printable text")
	}
	model, cmd = model.handleText(context.Background(), "ta thử lại")
	if cmd != nil || len(session.inputs) != 0 {
		t.Fatal("locked mode must never submit another turn")
	}
}

func TestTUIPaletteFiltersAndEscRestoresDraft(t *testing.T) {
	model := newTUIModel(&recordingSession{save: game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})}, "test-model")
	updated, _ := model.Update(keyPress('/', "/"))
	model = updated.(tuiModel)
	for _, r := range "túi" {
		updated, _ = model.Update(keyPress(r, string(r)))
		model = updated.(tuiModel)
	}
	if model.palette.FilterValue() != "túi" {
		t.Fatalf("palette filter = %q, want interactive filter text", model.palette.FilterValue())
	}
	model.editor.SetValue("bản nháp")
	model.input = "bản nháp"
	updated, cmd := model.Update(keyPress(tea.KeyEsc, ""))
	model = updated.(tuiModel)
	if cmd != nil || model.paletteOpen || !model.editor.Focused() || model.editor.Value() != "bản nháp" {
		t.Fatal("Esc should close palette and restore the exact editable draft")
	}
}

func TestTUIPaletteClosesForNonCommandText(t *testing.T) {
	model := newTUIModel(&recordingSession{save: game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})}, "test-model")
	updated, _ := model.Update(keyPress('/', "/"))
	model = updated.(tuiModel)
	updated, cmd := model.Update(keyPress('z', "z"))
	model = updated.(tuiModel)
	if cmd != nil || model.paletteOpen || !model.editor.Focused() || model.editor.Value() != "z" {
		t.Fatal("non-command text should close the palette and return to the editor")
	}
}

func TestTUIHelpExplainsMultilineHistoryPaletteAndEscPrecedence(t *testing.T) {
	model := newTUIModel(&recordingSession{save: game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})}, "test-model")
	model, _ = model.handleCommand("/help")
	view := model.View().Content
	for _, instruction := range []string{"Shift+Enter", "PgUp/PgDn", "bảng lệnh", "rồi mới thoát"} {
		if !strings.Contains(view, instruction) {
			t.Fatalf("help missing %q:\n%s", instruction, view)
		}
	}
}

func TestTUIViewportPreservesManualScrollAndMarksUnseen(t *testing.T) {
	model := newTUIModel(&recordingSession{save: game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})}, "test-model")
	model.width, model.height = 50, 14
	for i := range 20 {
		model.turns = append(model.turns, turnBlock{turnNumber: i + 1, narration: "dòng lịch sử"})
	}
	model.syncLayout()
	model.refreshTranscript(true)
	model.viewport.GotoTop()
	model.turns = append(model.turns, turnBlock{turnNumber: 21, narration: "lượt mới"})
	model.refreshTranscript(false)
	if model.viewport.AtBottom() || !model.unseen {
		t.Fatal("new content must not pull a manually scrolled transcript to bottom")
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

func TestTUITemporaryHelpFitsSupportedShortTerminal(t *testing.T) {
	model := newTUIModel(&recordingSession{save: game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})}, "test-model")
	model.width, model.height = 60, 14
	model, _ = model.handleCommand("/help")
	view := model.View().Content
	if rows := strings.Count(view, "\n") + 1; rows > model.height {
		t.Fatalf("temporary help rows = %d, exceed terminal height %d:\n%s", rows, model.height, view)
	}
}

func TestTUIPaletteFallbackKeepsCompleteTypedDraft(t *testing.T) {
	model := newTUIModel(&recordingSession{save: game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})}, "test-model")
	updated, _ := model.Update(keyPress('/', "/"))
	model = updated.(tuiModel)
	for _, r := range "helo" {
		updated, _ = model.Update(keyPress(r, string(r)))
		model = updated.(tuiModel)
	}
	if model.paletteOpen || !model.editor.Focused() || model.editor.Value() != "helo" {
		t.Fatalf("palette fallback lost draft: open=%t focused=%t draft=%q", model.paletteOpen, model.editor.Focused(), model.editor.Value())
	}
}

func TestTUIAmbiguousCompletionIgnoresTranscriptNavigation(t *testing.T) {
	model := newTUIModel(&recordingSession{save: game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})}, "test-model")
	model.width, model.height = 50, 14
	for i := range 20 {
		model.turns = append(model.turns, turnBlock{turnNumber: i + 1, narration: "dòng lịch sử"})
	}
	model.syncLayout()
	model.refreshTranscript(true)
	model, _ = model.applyTurnMsg(turnFinishedMsg{input: "ta quan sát"})
	model.viewport.GotoTop()
	offset := model.viewport.YOffset()
	updated, cmd := model.Update(keyPress(tea.KeyEnd, ""))
	model = updated.(tuiModel)
	if cmd != nil || model.viewport.YOffset() != offset {
		t.Fatal("locked mode must ignore transcript navigation")
	}
}

func TestTUIAmbiguousCompletionPaletteOffersOnlyExit(t *testing.T) {
	model := newTUIModel(&recordingSession{save: game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})}, "test-model")
	model, _ = model.applyTurnMsg(turnFinishedMsg{input: "ta quan sát"})
	updated, _ := model.Update(keyPress('/', "/"))
	model = updated.(tuiModel)
	items := model.palette.Items()
	if len(items) != 1 {
		t.Fatalf("locked palette has %d items, want only /exit", len(items))
	}
	item, ok := items[0].(commandItem)
	if !ok || item.command != "/exit" {
		t.Fatalf("locked palette exposes %#v, want only /exit", items[0])
	}
	updated, cmd := model.Update(keyPress(tea.KeyEnter, ""))
	model = updated.(tuiModel)
	if model.tempView != tempViewNone || !model.locked {
		t.Fatal("locked palette must not open a temporary view or unlock the model")
	}
	assertQuitCommand(t, cmd)
}

func TestTUIAmbiguousCompletionAllowsExitCommand(t *testing.T) {
	model := newTUIModel(&recordingSession{save: game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})}, "test-model")
	model, _ = model.applyTurnMsg(turnFinishedMsg{input: "ta quan sát"})
	updated, _ := model.Update(keyPress('/', "/"))
	model = updated.(tuiModel)
	for _, r := range "exit" {
		updated, _ = model.Update(keyPress(r, string(r)))
		model = updated.(tuiModel)
	}
	updated, cmd := model.Update(keyPress(tea.KeyEnter, ""))
	model = updated.(tuiModel)
	if !model.locked {
		t.Fatal("exit control must not unlock ambiguous completion")
	}
	assertQuitCommand(t, cmd)
}

func TestTUICtrlCWhilePendingCancelsTurnAndQuitsAfterCompletion(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	model := newTUIModel(&recordingSession{save: game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})}, "test-model")
	model.ctx = ctx
	model.cancel = cancel
	model.pending = &pendingTurn{turnNumber: 1, action: "ta quan sát"}

	updated, cmd := model.Update(tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl}))
	model = updated.(tuiModel)
	if cmd != nil {
		t.Fatal("cmd is not nil, quit must wait for the pending turn to finish")
	}
	if !model.quitting {
		t.Fatal("quitting should be set so applyTurnMsg quits once the turn completes")
	}
	if ctx.Err() == nil {
		t.Fatal("pending turn context was not canceled")
	}

	model, cmd = model.applyTurnMsg(turnFinishedMsg{input: "ta quan sát", err: ctx.Err()})
	assertQuitCommand(t, cmd)
}

func TestTUIEscWhilePendingCancelsTurnAndQuitsAfterCompletion(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	model := newTUIModel(&recordingSession{save: game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})}, "test-model")
	model.ctx = ctx
	model.cancel = cancel
	model.pending = &pendingTurn{turnNumber: 1, action: "ta quan sát"}

	updated, cmd := model.Update(keyPress(tea.KeyEsc, ""))
	model = updated.(tuiModel)
	if cmd != nil {
		t.Fatal("cmd is not nil, quit must wait for the pending turn to finish")
	}
	if !model.quitting {
		t.Fatal("quitting should be set so applyTurnMsg quits once the turn completes")
	}
	if ctx.Err() == nil {
		t.Fatal("pending turn context was not canceled")
	}
}

func TestTUIExitCommandWhilePendingCancelsTurnAndWaits(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	model := newTUIModel(&recordingSession{save: game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})}, "test-model")
	model.ctx = ctx
	model.cancel = cancel
	model.pending = &pendingTurn{turnNumber: 1, action: "ta quan sát"}

	model, cmd := model.handleCommand("/exit")
	if cmd != nil {
		t.Fatal("cmd is not nil, /exit while pending must wait for the turn to finish")
	}
	if !model.quitting || ctx.Err() == nil {
		t.Fatalf("quitting = %v, ctx.Err() = %v, want quitting=true and canceled ctx", model.quitting, ctx.Err())
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

func runTurnCommand(t *testing.T, cmd tea.Cmd) turnFinishedMsg {
	t.Helper()
	msg := cmd()
	if result, ok := msg.(turnFinishedMsg); ok {
		return result
	}
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("command message = %T, want turnFinishedMsg or tea.BatchMsg", msg)
	}
	for _, next := range batch {
		if result, ok := next().(turnFinishedMsg); ok {
			return result
		}
	}
	t.Fatal("batch did not include a turn completion")
	return turnFinishedMsg{}
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

func TestTUICondensedShellKeepsTwoViewportRowsWithoutSectionLabel(t *testing.T) {
	for _, height := range []int{14, 15, 16, 17} {
		t.Run(fmt.Sprintf("60x%d", height), func(t *testing.T) {
			model := newTUIModel(&recordingSession{save: game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})}, "test-model")
			model.width, model.height = 60, height
			model.syncLayout()
			model, _ = model.applyTurnMsg(turnFinishedMsg{input: "ta quan sát", result: &game.TurnResult{
				Narration:        "Bạn đứng trước cổng môn.",
				SuggestedActions: []string{"Quan sát cổng môn", "Hỏi đệ tử", "Kiểm tra trạng thái"},
			}})
			view := model.View().Content
			if strings.Contains(view, "NHẬT KÝ HÀNH TRÌNH") {
				t.Fatalf("condensed shell must not render the transcript section label:\n%s", view)
			}
			if rows := strings.Count(view, "\n") + 1; rows > model.height {
				t.Fatalf("rendered rows = %d, exceed height %d:\n%s", rows, model.height, view)
			}
			lines := strings.Split(view, "\n")
			vpRows := 0
			for _, line := range lines[2:] { // skip header + summary, stop at first suggestion row
				if strings.HasPrefix(line, "1. ") {
					break
				}
				vpRows++
			}
			if vpRows < 2 {
				t.Fatalf("condensed viewport rows = %d, want >= 2:\n%s", vpRows, view)
			}
		})
	}
}
