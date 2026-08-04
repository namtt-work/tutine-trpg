package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/namtt/tutine-trpg/internal/game"
	"github.com/namtt/tutine-trpg/internal/orchestrator"
)

// turnFailureMessage is shown to the player instead of the raw error text.
// The underlying error (provider outage, malformed LLM output, etc.) is an
// implementation detail the player can't act on; it's logged separately via
// tuiModel.logger for debugging instead. It also doubles as the player-facing
// confirmation that the failed turn was not applied to game state.
const turnFailureMessage = "Người kể chuyện gặp trục trặc và chưa thể phản hồi hợp lệ. Hành động của bạn chưa được ghi nhận, hãy thử lại."

const hiddenHistoryIndicator = "Lịch sử cũ hơn đang ẩn."

const wideBreakpoint = 90

type tempViewKind int

const (
	tempViewNone tempViewKind = iota
	tempViewStatus
	tempViewInventory
	tempViewHelp
)

// turnBlock is one resolved turn in the history region: player action,
// narration, player-facing state-change labels, and warnings.
type turnBlock struct {
	turnNumber   int
	playerAction string
	narration    string
	changes      []game.StateChangeView
	warnings     []string
}

// pendingTurn tracks a turn submitted to the LLM but not yet resolved, so
// the history region can show the player's action immediately and the
// action area can block duplicate submission.
type pendingTurn struct {
	turnNumber int
	action     string
}

type tuiModel struct {
	ctx           context.Context
	session       orchestrator.GameSession
	providerLabel string
	logger        *log.Logger

	turns       []turnBlock
	suggestions []string
	selected    int // index into suggestions highlighted by Tab, -1 = none

	input       string
	notice      string // transient message shown above the input line
	pending     *pendingTurn
	recoverable bool // true after a failed turn, until retried or cancelled
	tempView    tempViewKind

	width  int
	height int
}

type turnFinishedMsg struct {
	input  string
	result *game.TurnResult
	err    error
}

var (
	headerStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86"))
	panelStyle    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	errorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	hintStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
)

func newTUIModel(session orchestrator.GameSession, providerLabel string) tuiModel {
	save := session.Save()
	return tuiModel{
		ctx:           context.Background(),
		session:       session,
		providerLabel: providerLabel,
		suggestions:   initialSuggestionsFor(save.CurrentScene),
		selected:      -1,
		width:         100,
		height:        30,
	}
}

func runTUI(ctx context.Context, session orchestrator.GameSession, providerLabel string, logger *log.Logger) error {
	model := newTUIModel(session, providerLabel)
	model.ctx = ctx
	model.logger = logger
	_, err := tea.NewProgram(model, tea.WithAltScreen()).Run()
	return err
}

func (m tuiModel) Init() tea.Cmd {
	return nil
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	case turnFinishedMsg:
		return m.applyTurnMsg(msg)
	default:
		return m, nil
	}
}

func (m tuiModel) handleKey(msg tea.KeyMsg) (tuiModel, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyEsc:
		return m.handleEsc()
	case tea.KeyEnter:
		if m.pending != nil {
			return m, nil
		}
		text := m.input
		m.input = ""
		return m.handleText(m.ctx, text)
	case tea.KeyTab:
		return m.handleTab(), nil
	case tea.KeyBackspace:
		if len(m.input) > 0 {
			runes := []rune(m.input)
			m.input = string(runes[:len(runes)-1])
		}
		m.notice = ""
		return m, nil
	case tea.KeyRunes:
		if m.pending == nil {
			m.input += string(msg.Runes)
			m.notice = ""
		}
		return m, nil
	}
	return m, nil
}

// handleEsc is contextual: it closes whatever is currently "on top" (a
// temporary view, then a retained error draft) before falling back to
// quitting the application, per the spec's layered recovery flow.
func (m tuiModel) handleEsc() (tuiModel, tea.Cmd) {
	switch {
	case m.tempView != tempViewNone:
		m.tempView = tempViewNone
		return m, nil
	case m.recoverable:
		m.recoverable = false
		m.input = ""
		m.notice = ""
		return m, nil
	default:
		return m, tea.Quit
	}
}

func (m tuiModel) handleTab() tuiModel {
	if len(m.suggestions) == 0 {
		return m
	}
	m.selected = (m.selected + 1) % len(m.suggestions)
	m.input = m.suggestions[m.selected]
	m.notice = ""
	return m
}

func (m tuiModel) handleText(ctx context.Context, text string) (tuiModel, tea.Cmd) {
	text = strings.TrimSpace(text)
	if text == "" {
		return m, nil
	}
	if choice, ok := parseNumericChoice(text); ok {
		if choice < 1 || choice > len(m.suggestions) {
			m.notice = fmt.Sprintf("Chọn từ 1 đến %d, hoặc nhập hành động bằng chữ.", len(m.suggestions))
			return m, nil
		}
		text = m.suggestions[choice-1]
	}
	m.selected = -1
	m.notice = ""
	if command, isCommand := commandForSuggestedAction(text); isCommand {
		return m.handleCommand(command)
	}
	if strings.HasPrefix(text, "/") {
		return m.handleCommand(text)
	}

	m.pending = &pendingTurn{turnNumber: m.session.Save().CurrentTurn + 1, action: text}
	return m, func() tea.Msg {
		result, err := m.session.HandleTurn(ctx, orchestrator.PlayerInput{Text: text})
		return turnFinishedMsg{input: text, result: result, err: err}
	}
}

func parseNumericChoice(text string) (int, bool) {
	n, err := strconv.Atoi(text)
	if err != nil {
		return 0, false
	}
	return n, true
}

// handleCommand never touches m.input: the draft the player had typed
// (usually the command text itself) is left exactly as-is so closing a
// temporary view returns focus to an unchanged input, per spec.
func (m tuiModel) handleCommand(command string) (tuiModel, tea.Cmd) {
	switch strings.ToLower(command) {
	case "/status":
		m.tempView = tempViewStatus
	case "/inventory":
		m.tempView = tempViewInventory
	case "/help":
		m.tempView = tempViewHelp
	case "/exit":
		return m, tea.Quit
	default:
		m.notice = fmt.Sprintf("Không hiểu lệnh %s. Nhập /help để xem các lệnh hiện có.", command)
	}
	return m, nil
}

func (m tuiModel) applyTurnMsg(msg turnFinishedMsg) (tuiModel, tea.Cmd) {
	m.pending = nil
	if msg.err != nil {
		if m.logger != nil {
			m.logger.Printf("turn failed (input=%q): %v", msg.input, msg.err)
		}
		m.recoverable = true
		m.input = msg.input
		m.notice = turnFailureMessage
		return m, nil
	}
	m.recoverable = false
	m.notice = ""
	if msg.result == nil {
		m.notice = "lượt chơi không có kết quả"
		return m, nil
	}

	m.turns = append(m.turns, turnBlock{
		turnNumber:   m.session.Save().CurrentTurn,
		playerAction: msg.input,
		narration:    strings.TrimSpace(msg.result.Narration),
		changes:      msg.result.StateChanges,
		warnings:     msg.result.Warnings,
	})
	m.suggestions = append([]string{}, msg.result.SuggestedActions...)
	m.selected = -1
	return m, nil
}

func (m tuiModel) View() string {
	save := m.session.Save()
	header := renderHeader(save)
	action := m.renderActionArea()

	if m.tempView != tempViewNone {
		return strings.Join([]string{header, m.renderTempViewBody(save), action}, "\n\n")
	}

	history := clipHistory(m.historyText(save), m.historyBudget())
	summary := renderSummary(save)

	if m.width > wideBreakpoint {
		// summary is already a fully rendered bordered block; measure its
		// real cell width instead of wrapping it in a second Width() style,
		// which would re-wrap the box-drawing characters themselves and
		// corrupt the border.
		rightWidth := lipgloss.Width(summary)
		mainWidth := max(m.width-rightWidth-2, 40)
		left := lipgloss.NewStyle().Width(mainWidth).Render(history)
		body := lipgloss.JoinHorizontal(lipgloss.Top, left, summary)
		return strings.Join([]string{header, body, action}, "\n\n")
	}
	return strings.Join([]string{header, summary, history, action}, "\n\n")
}

func renderHeader(save game.SaveGame) string {
	return headerStyle.Render(fmt.Sprintf("Tutine TRPG | %s | Lượt %02d", sceneName(save.CurrentScene), save.CurrentTurn+1))
}

func renderSummary(save game.SaveGame) string {
	return panelStyle.Render(fmt.Sprintf("%s tầng %d | HP %d/%d | Linh lực %d/%d | Túi đồ %d món",
		realmName(save.Player.Realm), save.Player.Stage, save.Player.HP, save.Player.MaxHP,
		save.Player.SpiritualEnergy, save.Player.MaxEnergy, inventoryCount(save)))
}

func (m tuiModel) historyText(save game.SaveGame) string {
	var blocks []string
	if len(m.turns) == 0 {
		blocks = append(blocks, sceneIntro(save.CurrentScene))
	}
	for _, turn := range m.turns {
		blocks = append(blocks, renderTurnBlock(turn))
	}
	if m.pending != nil {
		blocks = append(blocks, renderPendingBlock(*m.pending))
	}
	return strings.Join(blocks, "\n\n")
}

func renderTurnBlock(b turnBlock) string {
	lines := []string{
		fmt.Sprintf("Lượt %02d", b.turnNumber),
		"Bạn: " + b.playerAction,
		"",
	}
	if b.narration != "" {
		lines = append(lines, b.narration)
	}
	if changes := renderChangeSummary(b.changes); changes != "" {
		lines = append(lines, "", "Thay đổi: "+changes)
	}
	for _, warning := range b.warnings {
		lines = append(lines, "Cảnh báo: "+warning)
	}
	return strings.Join(lines, "\n")
}

func renderChangeSummary(changes []game.StateChangeView) string {
	if len(changes) == 0 {
		return ""
	}
	parts := make([]string, 0, len(changes))
	for _, change := range changes {
		parts = append(parts, fmt.Sprintf("%s %+d", effectLabel(change.Type), change.Amount))
	}
	return strings.Join(parts, ", ")
}

func renderPendingBlock(p pendingTurn) string {
	return fmt.Sprintf("Lượt %02d\nBạn: %s", p.turnNumber, p.action)
}

// historyBudget reserves lines for the header, summary (on narrow layouts),
// and action area so the history region clips instead of pushing the
// always-visible regions off-screen.
func (m tuiModel) historyBudget() int {
	reserved := 10
	if m.width <= wideBreakpoint {
		reserved += 4
	}
	return max(m.height-reserved, 5)
}

func clipHistory(text string, budget int) string {
	lines := strings.Split(text, "\n")
	if len(lines) <= budget {
		return text
	}
	keep := max(budget-1, 1)
	clipped := append([]string{hiddenHistoryIndicator}, lines[len(lines)-keep:]...)
	return strings.Join(clipped, "\n")
}

func (m tuiModel) renderActionArea() string {
	if m.pending != nil {
		return hintStyle.Render("Đang xử lý lượt chơi...")
	}
	if m.tempView != tempViewNone {
		return strings.Join([]string{"> " + m.input, hintStyle.Render("Esc quay lại")}, "\n")
	}

	var parts []string
	if strings.HasPrefix(m.input, "/") {
		parts = append(parts, renderCommandPalette())
	} else {
		parts = append(parts, m.renderSuggestions())
		if len(m.turns) == 0 {
			parts = append(parts, "Bạn muốn làm gì? Ví dụ: ta quan sát cổng môn")
		}
	}
	if m.notice != "" {
		parts = append(parts, m.renderNotice())
	}
	parts = append(parts, "> "+m.input)
	parts = append(parts, hintStyle.Render(m.footer()))
	return strings.Join(parts, "\n")
}

func (m tuiModel) footer() string {
	if m.recoverable {
		return "Enter thử lại | Sửa nội dung trước khi gửi | Esc huỷ"
	}
	return "Enter gửi | Tab chọn gợi ý | / lệnh | Esc thoát"
}

func (m tuiModel) renderNotice() string {
	if m.recoverable {
		return errorStyle.Render("Lỗi: " + m.notice)
	}
	return hintStyle.Render(m.notice)
}

func (m tuiModel) renderSuggestions() string {
	if len(m.suggestions) == 0 {
		return hintStyle.Render("Chưa có gợi ý. Nhập hành động tự do để bắt đầu.")
	}
	label := "Tiếp theo:"
	if len(m.turns) == 0 {
		label = "Bắt đầu:"
	}
	lines := make([]string, 0, len(m.suggestions)+1)
	lines = append(lines, label)
	for i, option := range m.suggestions {
		line := fmt.Sprintf("%d. %s", i+1, option)
		if i == m.selected {
			line = selectedStyle.Render(line)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func renderCommandPalette() string {
	return strings.Join([]string{
		"Lệnh:",
		"  /status     Xem trạng thái nhân vật",
		"  /inventory  Xem túi đồ",
		"  /help       Xem hướng dẫn chơi",
		"  /exit       Thoát game",
	}, "\n")
}

func (m tuiModel) renderTempViewBody(save game.SaveGame) string {
	switch m.tempView {
	case tempViewStatus:
		return panelStyle.Render(formatStatus(save))
	case tempViewInventory:
		return panelStyle.Render(formatInventory(save))
	case tempViewHelp:
		return panelStyle.Render(helpText(m.providerLabel))
	default:
		return ""
	}
}

func helpText(providerLabel string) string {
	lines := []string{
		"Hướng dẫn chơi:",
		"- Nhập hành động tự do, ví dụ: ta quan sát cổng môn.",
		"- Nhập số để chọn một gợi ý đang hiển thị.",
		"- Nhấn Tab để đưa gợi ý hiện tại vào ô nhập, có thể sửa trước khi gửi.",
		"- Lệnh: /status, /inventory, /help, /exit.",
		"- Esc đóng màn hình đang xem hoặc thoát game.",
	}
	if strings.TrimSpace(providerLabel) != "" {
		lines = append(lines, "Mô hình đang dùng: "+providerLabel)
	}
	return strings.Join(lines, "\n")
}

func formatStatus(save game.SaveGame) string {
	var out bytes.Buffer
	renderStatus(&out, save)
	return strings.TrimSpace(out.String())
}

func formatInventory(save game.SaveGame) string {
	var out bytes.Buffer
	renderInventory(&out, save)
	return strings.TrimSpace(out.String())
}
