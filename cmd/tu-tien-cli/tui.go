package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/namtt/tutine-trpg/internal/game"
	"github.com/namtt/tutine-trpg/internal/orchestrator"
)

// turnFailureMessage is shown to the player instead of the raw error text.
// The underlying error (provider outage, malformed LLM output, etc.) is an
// implementation detail the player can't act on; it's logged separately via
// tuiModel.logger for debugging instead. It also doubles as the player-facing
// confirmation that the failed turn was not applied to game state.
const turnFailureMessage = "Người kể chuyện gặp trục trặc và chưa thể phản hồi hợp lệ. Hành động của bạn chưa được ghi nhận, hãy thử lại."

const ambiguousCompletionMessage = "Không thể xác nhận lượt chơi này. Hãy khởi động lại hoặc mở lại phiên trước khi hành động tiếp."

const wideBreakpoint = 90

type tempViewKind int

const (
	tempViewNone tempViewKind = iota
	tempViewStatus
	tempViewInventory
	tempViewHelp
	tempViewSave
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

type commandItem struct{ command, description string }

func (i commandItem) FilterValue() string { return i.command + " " + i.description }
func (i commandItem) Title() string       { return i.command }
func (i commandItem) Description() string { return i.description }

type tuiKeyMap struct {
	submit, newline, suggest, palette, pageUp, pageDown, back key.Binding
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
	editor      textarea.Model
	spinner     spinner.Model
	viewport    viewport.Model
	palette     list.Model
	help        help.Model
	keys        tuiKeyMap
	notice      string // transient message shown above the input line
	pending     *pendingTurn
	recoverable bool // true after a failed turn, until retried or cancelled
	paletteOpen bool
	locked      bool
	unseen      bool
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
	editor := textarea.New()
	editor.Prompt = "> "
	editor.Placeholder = "Bạn muốn làm gì?"
	editor.DynamicHeight = true
	editor.MinHeight = 1
	editor.MaxHeight = 3
	editor.SetWidth(100)
	editor.Focus()
	loading := spinner.New()
	vp := viewport.New(viewport.WithWidth(100), viewport.WithHeight(12))
	vp.SoftWrap = true
	items := []list.Item{
		commandItem{"/status", "Xem trạng thái nhân vật"}, commandItem{"/inventory", "Xem túi đồ"},
		commandItem{"/save", "Xem tiến trình đã lưu"}, commandItem{"/help", "Xem hướng dẫn chơi"}, commandItem{"/exit", "Thoát game"},
	}
	palette := list.New(items, list.NewDefaultDelegate(), 60, 7)
	palette.Title = "Lệnh"
	palette.SetShowHelp(false)
	palette.SetShowStatusBar(false)
	helpModel := help.New()
	helpModel.SetWidth(100)
	keys := tuiKeyMap{
		submit: key.NewBinding(key.WithKeys("enter"), key.WithHelp("Enter", "gửi")), newline: key.NewBinding(key.WithKeys("shift+enter"), key.WithHelp("Shift+Enter", "xuống dòng")),
		suggest: key.NewBinding(key.WithKeys("tab"), key.WithHelp("Tab", "gợi ý")), palette: key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "lệnh")),
		pageUp: key.NewBinding(key.WithKeys("pgup"), key.WithHelp("PgUp/PgDn", "lịch sử")), pageDown: key.NewBinding(key.WithKeys("pgdown"), key.WithHelp("PgUp/PgDn", "lịch sử")),
		back: key.NewBinding(key.WithKeys("esc"), key.WithHelp("Esc", "thoát")),
	}
	return tuiModel{
		ctx:           context.Background(),
		session:       session,
		providerLabel: providerLabel,
		suggestions:   initialSuggestionsFor(save.CurrentScene),
		selected:      -1,
		editor:        editor,
		spinner:       loading,
		viewport:      vp,
		palette:       palette,
		help:          helpModel,
		keys:          keys,
		width:         100,
		height:        30,
	}
}

func runTUI(ctx context.Context, session orchestrator.GameSession, providerLabel string, logger *log.Logger) error {
	model := newTUIModel(session, providerLabel)
	model.ctx = ctx
	model.logger = logger
	_, err := tea.NewProgram(model).Run()
	return err
}

func (m tuiModel) Init() tea.Cmd {
	return nil
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.syncLayout()
		m.refreshTranscript(false)
		return m, nil
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	case turnFinishedMsg:
		return m.applyTurnMsg(msg)
	case spinner.TickMsg:
		if m.pending == nil {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	default:
		return m, nil
	}
}

func (m tuiModel) handleKey(msg tea.KeyPressMsg) (tuiModel, tea.Cmd) {
	keyPress := msg.Key()
	if keyPress.Code == 'c' && keyPress.Mod&tea.ModCtrl != 0 {
		return m, tea.Quit
	}
	if keyPress.Code == tea.KeyEsc {
		return m.handleEsc()
	}
	if m.tempView != tempViewNone {
		return m, nil
	}
	if m.paletteOpen {
		if keyPress.Code == tea.KeyEnter {
			if m.locked {
				filter := strings.TrimSpace(m.palette.FilterValue())
				if filter == "" || strings.EqualFold(filter, "exit") {
					m.paletteOpen = false
					return m.handleCommand("/exit")
				}
				if item, ok := m.palette.SelectedItem().(commandItem); ok && item.command == "/exit" {
					m.paletteOpen = false
					return m.handleCommand("/exit")
				}
				return m, nil
			}
			if item, ok := m.palette.SelectedItem().(commandItem); ok {
				m.paletteOpen = false
				m.editor.Focus()
				return m.handleCommand(item.command)
			}
			return m, nil
		}
		var cmd tea.Cmd
		m.palette, cmd = m.palette.Update(msg)
		if keyPress.Text != "" && !paletteFilterMatches(m.palette.FilterValue()) {
			draft := m.palette.FilterValue()
			m.paletteOpen = false
			m.palette.ResetFilter()
			m.editor.Focus()
			m.editor.SetValue(draft)
			m.input = m.editor.Value()
			return m, nil
		}
		return m, cmd
	}
	if m.locked && !(keyPress.Text == "/" && m.editor.Value() == "") {
		return m, nil
	}
	if keyPress.Code == tea.KeyEnd {
		m.viewport.GotoBottom()
		m.unseen = false
		return m, nil
	}
	if key.Matches(msg, m.keys.pageUp, m.keys.pageDown) {
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		m.unseen = !m.viewport.AtBottom()
		return m, cmd
	}
	if m.pending != nil {
		return m, nil
	}
	if keyPress.Code == tea.KeyEnter && keyPress.Mod&tea.ModShift != 0 {
		m.editor.InsertString("\n")
		m.input = m.editor.Value()
		return m, nil
	}
	if keyPress.Code == tea.KeyEnter {
		text := m.editor.Value()
		m.input = ""
		return m.handleText(m.ctx, text)
	}
	if key.Matches(msg, m.keys.suggest) {
		return m.handleTab(), nil
	}
	if keyPress.Text == "/" && m.editor.Value() == "" {
		m.paletteOpen = true
		m.editor.Blur()
		if m.locked {
			m.palette.SetItems([]list.Item{commandItem{"/exit", "Thoát game"}})
		}
		m.palette.ResetFilter()
		m.palette.SetFilterState(list.Filtering)
		return m, nil
	}
	var cmd tea.Cmd
	if m.input != m.editor.Value() {
		m.editor.SetValue(m.input)
	}
	m.editor, cmd = m.editor.Update(msg)
	m.input = m.editor.Value()
	m.notice = ""
	_ = cmd // textarea cursor-blink work is not needed by the root model.
	return m, nil
}

func paletteFilterMatches(filter string) bool {
	filter = strings.ToLower(strings.TrimSpace(filter))
	if filter == "" {
		return true
	}
	for _, candidate := range []string{"status xem trạng thái nhân vật", "inventory xem túi đồ", "save xem tiến trình đã lưu", "help xem hướng dẫn chơi", "exit thoát game"} {
		if strings.Contains(strings.ToLower(candidate), filter) {
			return true
		}
	}
	return false
}

// handleEsc is contextual: it closes whatever is currently "on top" (a
// temporary view, then a retained error draft) before falling back to
// quitting the application, per the spec's layered recovery flow.
func (m tuiModel) handleEsc() (tuiModel, tea.Cmd) {
	switch {
	case m.paletteOpen:
		m.paletteOpen = false
		m.editor.Focus()
		return m, nil
	case m.tempView != tempViewNone:
		m.tempView = tempViewNone
		m.editor.Focus()
		return m, nil
	case m.recoverable:
		m.recoverable = false
		m.input = ""
		m.editor.SetValue("")
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
	m.editor.SetValue(m.input)
	m.notice = ""
	return m
}

func (m tuiModel) handleText(ctx context.Context, text string) (tuiModel, tea.Cmd) {
	if m.locked || m.pending != nil {
		return m, nil
	}
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
	m.editor.SetValue("")
	m.input = ""
	m.editor.Blur()
	m.refreshTranscript(true)
	turnCmd := func() tea.Msg {
		result, err := m.session.HandleTurn(ctx, orchestrator.PlayerInput{Text: text})
		return turnFinishedMsg{input: text, result: result, err: err}
	}
	return m, tea.Batch(turnCmd, m.spinner.Tick)
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
	case "/save":
		m.tempView = tempViewSave
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
	wasFollowing := m.viewport.AtBottom()
	m.pending = nil
	if msg.err != nil {
		if m.logger != nil {
			m.logger.Printf("turn failed (input=%q): %v", msg.input, msg.err)
		}
		m.recoverable = true
		m.input = msg.input
		m.editor.SetValue(msg.input)
		m.editor.Focus()
		m.notice = turnFailureMessage
		m.refreshTranscript(wasFollowing)
		return m, nil
	}
	m.recoverable = false
	m.notice = ""
	if msg.result == nil {
		if m.logger != nil {
			m.logger.Printf("ambiguous turn completion (input=%q): nil result without error", msg.input)
		}
		m.locked = true
		m.editor.Blur()
		m.editor.SetValue("")
		m.input = ""
		m.notice = ambiguousCompletionMessage
		m.refreshTranscript(wasFollowing)
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
	m.editor.Focus()
	m.refreshTranscript(wasFollowing)
	return m, nil
}

func (m tuiModel) View() tea.View {
	save := m.session.Save()
	m.syncLayout()
	m.refreshTranscript(m.viewport.GetContent() == "")
	if m.tempView != tempViewNone {
		m.viewport.SetContent(m.renderTempViewBody(save))
	}
	header := renderHeader(save)
	action := m.renderActionArea()
	if m.height < 8 {
		return tea.View{Content: strings.Join([]string{"Cửa sổ quá thấp, hãy đổi kích thước (tối thiểu 8 hàng).", m.editor.View(), m.help.ShortHelpView([]key.Binding{m.keys.back})}, "\n"), AltScreen: true}
	}
	body := m.viewport.View()
	if m.width > wideBreakpoint && m.tempView == tempViewNone {
		summary := renderSummary(save)
		body = lipgloss.JoinHorizontal(lipgloss.Top, lipgloss.NewStyle().Width(max(m.width-lipgloss.Width(summary)-2, 20)).Render(body), summary)
		return tea.View{Content: strings.Join([]string{header, body, action}, "\n"), AltScreen: true}
	}
	if m.height < 14 {
		return tea.View{Content: strings.Join([]string{header, renderCompactSummary(save), body, action}, "\n"), AltScreen: true}
	}
	return tea.View{Content: strings.Join([]string{header, renderSummary(save), body, action}, "\n"), AltScreen: true}
}

func renderCompactSummary(save game.SaveGame) string {
	return fmt.Sprintf("%s %d · HP %d/%d · Linh lực %d/%d · Túi %d", realmName(save.Player.Realm), save.Player.Stage, save.Player.HP, save.Player.MaxHP, save.Player.SpiritualEnergy, save.Player.MaxEnergy, inventoryCount(save))
}

func renderHeader(save game.SaveGame) string {
	return headerStyle.Render(fmt.Sprintf("Tutine TRPG | %s | Lượt %02d", sceneName(save.CurrentScene), save.CurrentTurn+1))
}

func renderSummary(save game.SaveGame) string {
	return panelStyle.Render(fmt.Sprintf("%s tầng %d | HP %d/%d | Linh lực %d/%d | Túi đồ %d món",
		realmName(save.Player.Realm), save.Player.Stage, save.Player.HP, save.Player.MaxHP,
		save.Player.SpiritualEnergy, save.Player.MaxEnergy, inventoryCount(save)))
}

func (m *tuiModel) syncLayout() {
	width := max(m.width, 20)
	editorHeight := 3
	if m.height < 14 {
		editorHeight = 1
	}
	m.editor.SetWidth(width)
	m.editor.SetHeight(editorHeight)
	m.palette.SetSize(min(width, 60), min(7, max(m.height-4, 1)))
	m.help.SetWidth(width)
	viewportWidth := width
	reservedRows := 3 // header, action area, and their two separators in wide layout
	if width > wideBreakpoint && m.tempView == tempViewNone {
		viewportWidth = max(width-lipgloss.Width(renderSummary(m.session.Save()))-2, 20)
	} else {
		reservedRows += 1 // narrow layouts render a summary below the header
		if m.height >= 14 {
			reservedRows += lipgloss.Height(renderSummary(m.session.Save())) - 1
		}
		reservedRows++ // third separator between header, summary, viewport, and action
	}
	m.viewport.SetWidth(viewportWidth)
	m.viewport.SetHeight(max(m.height-lipgloss.Height(m.renderActionArea())-reservedRows, 1))
}

func (m tuiModel) actionRows() int {
	return lipgloss.Height(m.renderActionArea())
}

func (m *tuiModel) refreshTranscript(follow bool) {
	m.viewport.SetContent(m.historyText(m.session.Save()))
	if follow {
		m.viewport.GotoBottom()
		m.unseen = false
	} else if !m.viewport.AtBottom() {
		m.unseen = true
	}
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

func (m tuiModel) renderActionArea() string {
	if m.paletteOpen {
		return strings.Join([]string{m.palette.View(), hintStyle.Render("Enter chọn · ↑/↓ di chuyển · Esc quay lại")}, "\n")
	}
	if m.locked {
		return strings.Join([]string{errorStyle.Render(ambiguousCompletionMessage), hintStyle.Render("Nhập liệu đã khóa · Esc thoát")}, "\n")
	}
	if m.pending != nil {
		return strings.Join([]string{m.spinner.View() + " Đang xử lý lượt chơi...", hintStyle.Render("Đang xử lý lượt chơi… · PgUp/PgDn lịch sử")}, "\n")
	}
	if m.tempView != tempViewNone {
		return hintStyle.Render("Enter chọn · ↑/↓ di chuyển · Esc quay lại")
	}
	var parts []string
	if m.height >= 14 {
		parts = append(parts, m.renderSuggestions())
	} else {
		parts = append(parts, strings.ReplaceAll(m.renderSuggestions(), "\n", " · "))
	}
	if len(m.turns) == 0 {
		parts = append(parts, "Bạn muốn làm gì? Ví dụ: ta quan sát cổng môn")
	}
	if m.notice != "" {
		parts = append(parts, m.renderNotice())
	}
	if m.unseen {
		parts = append(parts, hintStyle.Render("↓ Có lượt mới (End để xem)"))
	}
	parts = append(parts, m.editor.View())
	parts = append(parts, hintStyle.Render(m.footer()))
	return strings.Join(parts, "\n")
}

func (m tuiModel) footer() string {
	if m.recoverable {
		return "Enter thử lại · sửa nội dung trước khi gửi · Esc huỷ"
	}
	return m.help.ShortHelpView([]key.Binding{m.keys.submit, m.keys.newline, m.keys.suggest, m.keys.palette, m.keys.pageUp, m.keys.back})
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
		"  /save       Xem tiến trình đã lưu",
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
	case tempViewSave:
		return panelStyle.Render(formatSaveConfirmation(save))
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
		"- Enter gửi; Shift+Enter xuống dòng trong hành động nhiều dòng.",
		"- PgUp/PgDn cuộn lịch sử; End trở về lượt mới nhất.",
		"- Gõ / để mở bảng lệnh, rồi lọc và chọn bằng Enter.",
		"- Lệnh: /status, /inventory, /save, /help, /exit.",
		"- Tiến trình được lưu tự động sau mỗi lượt và tự tiếp tục ở lần chơi sau.",
		"- Esc đóng bảng lệnh/màn hình tạm, huỷ bản nháp lỗi, rồi mới thoát game.",
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

// formatSaveConfirmation deliberately omits the raw save id and filesystem
// path: internal identifiers must not appear in player-facing UI. The id and
// path remain available in debug.log for anyone who needs to locate the
// file on disk.
func formatSaveConfirmation(save game.SaveGame) string {
	return fmt.Sprintf("Tiến trình đã được lưu tự động ở lượt %d.", save.CurrentTurn)
}
