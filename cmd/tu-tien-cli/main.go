package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/namtt/tutine-trpg/internal/game"
	"github.com/namtt/tutine-trpg/internal/llm"
	"github.com/namtt/tutine-trpg/internal/memory"
	"github.com/namtt/tutine-trpg/internal/orchestrator"
)

func main() {
	offline := flag.Bool("offline", true, "run with fake LLM client")
	name := flag.String("name", "Vô Danh", "player name")
	dataDir := flag.String("data-dir", "./data/dev", "data directory")
	flag.Parse()
	if !*offline {
		fmt.Fprintln(os.Stderr, "online provider is not wired in this foundation build")
		os.Exit(2)
	}

	session, cleanup, err := buildOfflineSession(*dataDir, *name)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer cleanup()

	if err := runInteractive(context.Background(), session, os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runInteractive(ctx context.Context, session orchestrator.GameSession, input io.Reader, output io.Writer) error {
	fmt.Fprintln(output, "Tutine TRPG")
	fmt.Fprintln(output, "Nhập /help để xem lệnh, /exit để thoát.")

	var lastOptions []string
	scanner := bufio.NewScanner(input)
	for {
		fmt.Fprint(output, "> ")
		if !scanner.Scan() {
			break
		}
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			continue
		}
		if text == "/exit" {
			break
		}
		if strings.HasPrefix(text, "/") {
			handleCommand(output, session, text)
			continue
		}
		if option, ok := resolveSuggestedAction(text, lastOptions); ok {
			if command, isCommand := commandForSuggestedAction(option); isCommand {
				handleCommand(output, session, command)
				continue
			}
			text = option
		}

		result, err := session.HandleTurn(ctx, orchestrator.PlayerInput{Text: text})
		if err != nil {
			fmt.Fprintln(output, "Lỗi:", err)
			continue
		}
		renderTurnResult(output, result)
		lastOptions = append([]string{}, result.SuggestedActions...)
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("lỗi đọc đầu vào: %w", err)
	}
	return nil
}

func handleCommand(output io.Writer, session orchestrator.GameSession, command string) {
	switch command {
	case "/help":
		renderHelp(output)
	case "/status":
		renderStatus(output, session.Save())
	case "/inventory":
		renderInventory(output, session.Save())
	default:
		fmt.Fprintf(output, "Không hiểu lệnh %s. Nhập /help để xem các lệnh hiện có.\n", command)
	}
}

func renderHelp(output io.Writer) {
	fmt.Fprintln(output, "Lệnh hiện có:")
	fmt.Fprintln(output, "  /help      xem danh sách lệnh")
	fmt.Fprintln(output, "  /status    xem trạng thái nhân vật")
	fmt.Fprintln(output, "  /inventory xem túi đồ")
	fmt.Fprintln(output, "  /exit      thoát game")
	fmt.Fprintln(output, "Bạn cũng có thể nhập số 1, 2, 3... để chọn gợi ý vừa hiện.")
}

func renderStatus(output io.Writer, save game.SaveGame) {
	fmt.Fprintf(output, "%s - %s tầng %d | HP %d/%d | Linh lực %d/%d\n", save.Player.Name, save.Player.Realm, save.Player.Stage, save.Player.HP, save.Player.MaxHP, save.Player.SpiritualEnergy, save.Player.MaxEnergy)
}

func renderInventory(output io.Writer, save game.SaveGame) {
	if len(save.Inventory) == 0 {
		fmt.Fprintln(output, "Túi đồ đang trống.")
		return
	}
	fmt.Fprintln(output, "Túi đồ:")
	for itemID, amount := range save.Inventory {
		fmt.Fprintf(output, "- %s x%d\n", itemID, amount)
	}
}

func renderTurnResult(output io.Writer, result *game.TurnResult) {
	fmt.Fprintln(output, result.Narration)
	for _, change := range result.StateChanges {
		fmt.Fprintf(output, "- %s: %+d\n", change.Type, change.Amount)
	}
	for i, option := range result.SuggestedActions {
		fmt.Fprintf(output, "%d. %s\n", i+1, option)
	}
	for _, warning := range result.Warnings {
		fmt.Fprintln(output, "Cảnh báo:", warning)
	}
}

func commandForSuggestedAction(action string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "kiểm tra trạng thái", "xem trạng thái", "status":
		return "/status", true
	case "xem túi đồ", "kiểm tra túi đồ", "inventory":
		return "/inventory", true
	default:
		return "", false
	}
}

func resolveSuggestedAction(text string, options []string) (string, bool) {
	choice, err := strconv.Atoi(text)
	if err != nil || choice < 1 || choice > len(options) {
		return "", false
	}
	return options[choice-1], true
}

func buildOfflineSession(dataDir string, name string) (*orchestrator.Session, func(), error) {
	save := game.NewStarterSave(game.NewGameRequest{Name: name, CampaignID: "thanh-van-sect", Traits: []string{"careful"}})
	saveDir := filepath.Join(dataDir, "saves", save.SaveID)
	if err := os.MkdirAll(saveDir, 0o755); err != nil {
		return nil, nil, err
	}
	store, err := memory.NewSQLiteStore(context.Background(), filepath.Join(saveDir, "game.db"))
	if err != nil {
		return nil, nil, err
	}
	session := orchestrator.NewSession(save, llm.FakeClient{}, store, []string{"trust", "secret", "sect_politics"})
	return session, func() { _ = store.Close() }, nil
}
