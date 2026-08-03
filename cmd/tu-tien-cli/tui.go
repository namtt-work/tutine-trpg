package main

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/namtt/tutine-trpg/internal/game"
	"github.com/namtt/tutine-trpg/internal/orchestrator"
)

type tuiModel struct {
	ctx           context.Context
	session       orchestrator.GameSession
	providerLabel string
	log           []string
	suggestions   []string
	input         string
	loading       bool
	width         int
	height        int
}

type turnFinishedMsg struct {
	input  string
	result *game.TurnResult
	err    error
}

var (
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86"))
	panelStyle  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	errorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	hintStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
)

func newTUIModel(session orchestrator.GameSession, providerLabel string) tuiModel {
	return tuiModel{
		ctx:           context.Background(),
		session:       session,
		providerLabel: providerLabel,
		log:           []string{"Chào mừng đến Tutine TRPG. Nhập hành động hoặc /help."},
		width:         100,
		height:        30,
	}
}

func runTUI(ctx context.Context, session orchestrator.GameSession, providerLabel string) error {
	model := newTUIModel(session, providerLabel)
	model.ctx = ctx
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
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			return m, tea.Quit
		case tea.KeyEnter:
			text := m.input
			m.input = ""
			return m.handleText(m.ctx, text)
		case tea.KeyBackspace:
			if len(m.input) > 0 {
				m.input = strings.TrimSuffix(m.input, string([]rune(m.input)[len([]rune(m.input))-1]))
			}
		case tea.KeyRunes:
			if !m.loading {
				m.input += string(msg.Runes)
			}
		}
		return m, nil
	case turnFinishedMsg:
		return m.applyTurnMsg(msg)
	default:
		return m, nil
	}
}

func (m tuiModel) handleText(ctx context.Context, text string) (tuiModel, tea.Cmd) {
	text = strings.TrimSpace(text)
	if text == "" {
		return m, nil
	}
	if m.loading {
		m.log = append(m.log, "Đang chờ LLM trả lời, vui lòng đợi.")
		return m, nil
	}
	if option, ok := resolveSuggestedAction(text, m.suggestions); ok {
		if command, isCommand := commandForSuggestedAction(option); isCommand {
			text = command
		} else {
			text = option
		}
	}
	if strings.HasPrefix(text, "/") {
		return m.handleCommand(text)
	}
	m.loading = true
	m.log = append(m.log, "> "+text)
	return m, func() tea.Msg {
		result, err := m.session.HandleTurn(ctx, orchestrator.PlayerInput{Text: text})
		return turnFinishedMsg{input: text, result: result, err: err}
	}
}

func (m tuiModel) handleCommand(command string) (tuiModel, tea.Cmd) {
	switch command {
	case "/help":
		m.log = append(m.log, "Lệnh: /help, /status, /inventory, /exit. Nhập số để chọn gợi ý mới nhất.")
	case "/status":
		m.log = append(m.log, formatStatus(m.session.Save()))
	case "/inventory":
		m.log = append(m.log, formatInventory(m.session.Save()))
	case "/exit":
		return m, tea.Quit
	default:
		m.log = append(m.log, fmt.Sprintf("Không hiểu lệnh %s. Nhập /help để xem các lệnh hiện có.", command))
	}
	return m, nil
}

func (m tuiModel) applyTurnMsg(msg turnFinishedMsg) (tuiModel, tea.Cmd) {
	m.loading = false
	if msg.err != nil {
		m.log = append(m.log, errorStyle.Render("Lỗi: "+msg.err.Error()))
		return m, nil
	}
	if msg.result == nil {
		m.log = append(m.log, errorStyle.Render("Lỗi: lượt chơi không có kết quả"))
		return m, nil
	}
	if strings.TrimSpace(msg.result.Narration) != "" {
		m.log = append(m.log, msg.result.Narration)
	}
	for _, change := range msg.result.StateChanges {
		m.log = append(m.log, fmt.Sprintf("- %s: %+d", change.Type, change.Amount))
	}
	for _, warning := range msg.result.Warnings {
		m.log = append(m.log, "Cảnh báo: "+warning)
	}
	m.suggestions = append([]string{}, msg.result.SuggestedActions...)
	return m, nil
}

func (m tuiModel) View() string {
	save := m.session.Save()
	header := headerStyle.Render(fmt.Sprintf("Tutine TRPG | %s | %s", save.CurrentScene, m.providerLabel))
	status := panelStyle.Render(formatStatus(save))
	suggestions := renderSuggestions(m.suggestions)
	log := strings.Join(trimLog(m.log, m.logLimit()), "\n")
	if m.loading {
		log += "\n" + hintStyle.Render("Đang gọi LLM...")
	}
	input := "> " + m.input
	footer := hintStyle.Render("/help /status /inventory /exit | Enter gửi hành động | Esc thoát")

	if m.width > 90 {
		mainWidth := m.width - 34
		if mainWidth < 40 {
			mainWidth = 40
		}
		left := lipgloss.NewStyle().Width(mainWidth).Render(log + "\n\n" + suggestions)
		right := lipgloss.NewStyle().Width(30).Render(status)
		return strings.Join([]string{header, lipgloss.JoinHorizontal(lipgloss.Top, left, right), input, footer}, "\n")
	}
	return strings.Join([]string{header, status, log, suggestions, input, footer}, "\n")
}

func (m tuiModel) logLimit() int {
	limit := m.height - 10
	if limit < 8 {
		return 8
	}
	return limit
}

func trimLog(entries []string, limit int) []string {
	if len(entries) <= limit {
		return entries
	}
	return entries[len(entries)-limit:]
}

func renderSuggestions(options []string) string {
	if len(options) == 0 {
		return hintStyle.Render("Chưa có gợi ý. Nhập hành động tự do để bắt đầu.")
	}
	lines := make([]string, 0, len(options)+1)
	lines = append(lines, "Gợi ý:")
	for i, option := range options {
		lines = append(lines, fmt.Sprintf("%d. %s", i+1, option))
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
