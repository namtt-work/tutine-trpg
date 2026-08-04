package storage

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/namtt/tutine-trpg/internal/game"
)

func TestSaveSnapshotThenLoadSnapshotRoundTrips(t *testing.T) {
	ctx := context.Background()
	store := NewFileStore(t.TempDir())
	save := game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})

	if err := store.SaveSnapshot(ctx, save); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}
	loaded, err := store.LoadSnapshot(ctx, save.SaveID)
	if err != nil {
		t.Fatalf("LoadSnapshot: %v", err)
	}
	if loaded.SaveID != save.SaveID || loaded.Player.Name != save.Player.Name || loaded.CurrentScene != save.CurrentScene {
		t.Fatalf("loaded = %#v, want equivalent to %#v", loaded, save)
	}
}

func TestLoadSnapshotUnknownSaveReturnsNotFoundError(t *testing.T) {
	store := NewFileStore(t.TempDir())
	if _, err := store.LoadSnapshot(context.Background(), "save_does_not_exist"); err == nil {
		t.Fatal("expected not-found error")
	}
}

func TestLoadSnapshotRejectsMismatchedEmbeddedSaveID(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	store := NewFileStore(dataDir)
	save := game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})
	if err := store.SaveSnapshot(ctx, save); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}

	corrupted := save
	corrupted.SaveID = "save_other"
	data, err := json.MarshalIndent(corrupted, "", "  ")
	if err != nil {
		t.Fatalf("marshal corrupted snapshot: %v", err)
	}
	statePath := filepath.Join(dataDir, "saves", save.SaveID, "state.json")
	if err := os.WriteFile(statePath, data, 0o644); err != nil {
		t.Fatalf("write corrupted state.json: %v", err)
	}

	if _, err := store.LoadSnapshot(ctx, save.SaveID); err == nil {
		t.Fatal("expected corruption error for mismatched embedded save id")
	}
}

func TestSaveSnapshotFailureLeavesPreviousSnapshotReadable(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	store := NewFileStore(dataDir)
	save := game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})
	if err := store.SaveSnapshot(ctx, save); err != nil {
		t.Fatalf("initial SaveSnapshot: %v", err)
	}

	dir := filepath.Join(dataDir, "saves", save.SaveID)
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("chmod read-only: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	save.Player.Name = "Changed"
	if err := store.SaveSnapshot(ctx, save); err == nil {
		t.Fatal("expected SaveSnapshot to fail on a read-only directory")
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatalf("restore permissions: %v", err)
	}

	loaded, err := store.LoadSnapshot(ctx, save.SaveID)
	if err != nil {
		t.Fatalf("LoadSnapshot after failed write: %v", err)
	}
	if loaded.Player.Name != "Nam" {
		t.Fatalf("player name = %q, want previous valid snapshot preserved", loaded.Player.Name)
	}
}

func TestValidateSaveIDRejectsTraversalShapes(t *testing.T) {
	for _, id := range []string{"", ".", "..", "../etc", "a/b", "/etc/passwd"} {
		if err := validateSaveID(id); err == nil {
			t.Fatalf("validateSaveID(%q) = nil, want error", id)
		}
	}
}
