package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/namtt/tutine-trpg/internal/config"
	"github.com/namtt/tutine-trpg/internal/game"
	"github.com/namtt/tutine-trpg/internal/llm"
	"github.com/namtt/tutine-trpg/internal/memory"
	"github.com/namtt/tutine-trpg/internal/orchestrator"
)

func main() {
	name := flag.String("name", "Vô Danh", "player name")
	configPath := flag.String("config", "configs/llm.yaml", "runtime config path")
	flag.Parse()

	session, logger, cleanup, err := buildSession(context.Background(), *configPath, *name)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer cleanup()

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := runTUI(context.Background(), session, cfg.LLM.Model, logger); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func renderStatus(output interface{ Write([]byte) (int, error) }, save game.SaveGame) {
	fmt.Fprintf(output, "%s - %s tầng %d | HP %d/%d | Linh lực %d/%d\n", save.Player.Name, realmName(save.Player.Realm), save.Player.Stage, save.Player.HP, save.Player.MaxHP, save.Player.SpiritualEnergy, save.Player.MaxEnergy)
}

func renderInventory(output interface{ Write([]byte) (int, error) }, save game.SaveGame) {
	if len(save.Inventory) == 0 {
		fmt.Fprintln(output, "Túi đồ đang trống.")
		return
	}
	fmt.Fprintln(output, "Túi đồ:")
	for itemID, amount := range save.Inventory {
		fmt.Fprintf(output, "- %s x%d\n", itemID, amount)
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

func buildSession(ctx context.Context, configPath string, name string) (*orchestrator.Session, *log.Logger, func(), error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("load config %s: %w", configPath, err)
	}
	client, err := llm.NewOpenAICompatibleClient(llm.OpenAICompatibleConfig{BaseURL: cfg.LLM.BaseURL, APIKeyEnv: cfg.LLM.APIKeyEnv, Model: cfg.LLM.Model, TimeoutSeconds: cfg.LLM.TimeoutSeconds, MaxRetries: cfg.LLM.MaxRetries})
	if err != nil {
		return nil, nil, nil, err
	}
	dataDir := cfg.Storage.DataDir
	if strings.TrimSpace(dataDir) == "" {
		dataDir = "./data/dev"
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, nil, nil, err
	}
	logFile, err := os.OpenFile(filepath.Join(dataDir, "debug.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, nil, nil, err
	}
	logger := log.New(logFile, "", log.LstdFlags)

	save := game.NewStarterSave(game.NewGameRequest{Name: name, CampaignID: "thanh-van-sect", Traits: []string{"careful"}})
	saveDir := filepath.Join(dataDir, "saves", save.SaveID)
	if err := os.MkdirAll(saveDir, 0o755); err != nil {
		_ = logFile.Close()
		return nil, nil, nil, err
	}
	store, err := memory.NewSQLiteStore(ctx, filepath.Join(saveDir, "game.db"))
	if err != nil {
		_ = logFile.Close()
		return nil, nil, nil, err
	}
	session := orchestrator.NewSession(save, client, store, []string{"trust", "secret", "sect_politics"})
	cleanup := func() {
		_ = store.Close()
		_ = logFile.Close()
	}
	return session, logger, cleanup, nil
}
