package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/namtt/tutine-trpg/internal/config"
	"github.com/namtt/tutine-trpg/internal/game"
	"github.com/namtt/tutine-trpg/internal/llm"
	"github.com/namtt/tutine-trpg/internal/memory"
	"github.com/namtt/tutine-trpg/internal/orchestrator"
	"github.com/namtt/tutine-trpg/internal/storage"
)

const defaultCampaignID = "thanh-van-sect"

// StartupOptions selects which save buildSession uses. SaveID and ForceNew
// are mutually exclusive; see resolveStartupSave for the resolution order.
type StartupOptions struct {
	PlayerName string
	SaveID     string
	ForceNew   bool
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	name := flag.String("name", "Vô Danh", "player name for a new game")
	configPath := flag.String("config", "configs/llm.yaml", "runtime config path")
	saveID := flag.String("save", "", "resume a specific save, skipping auto-resume")
	forceNew := flag.Bool("new", false, "force a new game even if a save exists")
	flag.Parse()

	opts := StartupOptions{PlayerName: *name, SaveID: *saveID, ForceNew: *forceNew}
	session, logger, cleanup, err := buildSession(ctx, *configPath, opts)
	if err != nil {
		return err
	}
	defer cleanup()

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	return runTUI(ctx, session, cfg.LLM.Model, logger)
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

func buildSession(ctx context.Context, configPath string, opts StartupOptions) (*orchestrator.Session, *log.Logger, func(), error) {
	if opts.SaveID != "" && opts.ForceNew {
		return nil, nil, nil, errors.New("--save and --new cannot be used together")
	}

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

	fileStore := storage.NewFileStore(dataDir)
	save, lock, resumeKind, err := resolveStartupSave(ctx, fileStore, opts)
	if err != nil {
		_ = logFile.Close()
		return nil, nil, nil, err
	}

	// save.SaveID is safe to use for the game.db path here: resolveStartupSave
	// only returns via LoadSnapshot (which cross-checks the embedded SaveID
	// against the requested one) or a just-created SaveGame, so it is always
	// the canonical ID for this save, not a value blindly trusted from JSON.
	saveDir := filepath.Join(dataDir, "saves", save.SaveID)
	memStore, err := memory.NewSQLiteStore(ctx, filepath.Join(saveDir, "game.db"))
	if err != nil {
		_ = lock.Release()
		_ = logFile.Close()
		return nil, nil, nil, err
	}

	logger.Printf("%s save %s at turn %d", resumeKind, save.SaveID, save.CurrentTurn)

	session := orchestrator.NewSession(save, client, memStore, fileStore, []string{"trust", "secret", "sect_politics"})
	cleanup := func() {
		_ = lock.Release()
		_ = memStore.Close()
		_ = logFile.Close()
	}
	return session, logger, cleanup, nil
}

// resolveStartupSave implements the StartupOptions resolution order: explicit
// SaveID, else ForceNew, else auto-resume the most recently updated save for
// the campaign, else start a new one. It acquires the save's lock before
// returning and, for a brand new save, writes the initial snapshot so the
// save exists on disk even if the player quits before their first resolved
// turn. Any error after the lock is acquired releases it before returning.
func resolveStartupSave(ctx context.Context, store storage.Store, opts StartupOptions) (game.SaveGame, storage.Lock, string, error) {
	if opts.SaveID != "" {
		lock, err := store.AcquireLock(ctx, opts.SaveID)
		if err != nil {
			return game.SaveGame{}, nil, "", err
		}
		save, err := store.LoadSnapshot(ctx, opts.SaveID)
		if err != nil {
			_ = lock.Release()
			return game.SaveGame{}, nil, "", err
		}
		return save, lock, "resumed", nil
	}

	if !opts.ForceNew {
		saves, err := store.ListSaves(ctx, defaultCampaignID)
		if err != nil {
			return game.SaveGame{}, nil, "", err
		}
		if len(saves) > 0 {
			latestID := saves[0].SaveID
			lock, err := store.AcquireLock(ctx, latestID)
			if err != nil {
				return game.SaveGame{}, nil, "", err
			}
			save, err := store.LoadSnapshot(ctx, latestID)
			if err != nil {
				_ = lock.Release()
				return game.SaveGame{}, nil, "", err
			}
			return save, lock, "resumed", nil
		}
	}

	save := game.NewStarterSave(game.NewGameRequest{Name: opts.PlayerName, CampaignID: defaultCampaignID, Traits: []string{"careful"}})
	lock, err := store.AcquireLock(ctx, save.SaveID)
	if err != nil {
		return game.SaveGame{}, nil, "", err
	}
	if err := store.SaveSnapshot(ctx, save); err != nil {
		_ = lock.Release()
		return game.SaveGame{}, nil, "", fmt.Errorf("write initial save snapshot: %w", err)
	}
	return save, lock, "started new", nil
}
