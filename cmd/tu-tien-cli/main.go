package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/namtt/tutine-trpg/internal/game"
	"github.com/namtt/tutine-trpg/internal/llm"
	"github.com/namtt/tutine-trpg/internal/memory"
	"github.com/namtt/tutine-trpg/internal/orchestrator"
)

func main() {
	offline := flag.Bool("offline", true, "run with fake LLM client")
	name := flag.String("name", "Vo Danh", "player name")
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

	fmt.Println("Tutine TRPG")
	fmt.Println("Nhap /exit de thoat, /status de xem nhan vat.")
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("> ")
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
		if text == "/status" {
			save := session.Save()
			fmt.Printf("%s - %s tang %d | HP %d/%d | Linh luc %d/%d\n", save.Player.Name, save.Player.Realm, save.Player.Stage, save.Player.HP, save.Player.MaxHP, save.Player.SpiritualEnergy, save.Player.MaxEnergy)
			continue
		}
		result, err := session.HandleTurn(context.Background(), orchestrator.PlayerInput{Text: text})
		if err != nil {
			fmt.Println("Loi:", err)
			continue
		}
		fmt.Println(result.Narration)
		for _, change := range result.StateChanges {
			fmt.Printf("- %s: %+d\n", change.Type, change.Amount)
		}
		for i, option := range result.SuggestedActions {
			fmt.Printf("%d. %s\n", i+1, option)
		}
		for _, warning := range result.Warnings {
			fmt.Println("Canh bao:", warning)
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "Loi doc dau vao:", err)
	}
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
