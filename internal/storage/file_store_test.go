package storage

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestAppendEventProducesOneParseableLinePerCall(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	store := NewFileStore(dataDir)
	save := game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})

	for i := 1; i <= 3; i++ {
		event := Event{Turn: i, Type: EventTypeTurnResolved, PlayerAction: "hanh dong", CreatedAt: time.Now().UTC()}
		if err := store.AppendEvent(ctx, save.SaveID, event); err != nil {
			t.Fatalf("AppendEvent %d: %v", i, err)
		}
	}

	data, err := os.ReadFile(filepath.Join(dataDir, "saves", save.SaveID, "events.jsonl"))
	if err != nil {
		t.Fatalf("read events.jsonl: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 3 {
		t.Fatalf("lines = %d, want 3", len(lines))
	}
	for i, line := range lines {
		var decoded Event
		if err := json.Unmarshal([]byte(line), &decoded); err != nil {
			t.Fatalf("line %d not parseable: %v", i, err)
		}
		if decoded.Turn != i+1 {
			t.Fatalf("line %d turn = %d, want %d", i, decoded.Turn, i+1)
		}
	}
}

func TestAcquireLockBlocksSecondAcquisitionRegardlessOfLiveness(t *testing.T) {
	ctx := context.Background()
	store := NewFileStore(t.TempDir())
	saveID := "save_lock_test"

	lock, err := store.AcquireLock(ctx, saveID)
	if err != nil {
		t.Fatalf("first AcquireLock: %v", err)
	}
	defer lock.Release()

	// This phase does not check whether the process that created the lock
	// is still running, so an existing .lock file blocks acquisition
	// unconditionally; that's what this asserts, not "a live process blocks."
	if _, err := store.AcquireLock(ctx, saveID); err == nil {
		t.Fatal("expected second AcquireLock to fail while an existing lock file is present")
	}
}

func TestLockReleaseAllowsReacquisition(t *testing.T) {
	ctx := context.Background()
	store := NewFileStore(t.TempDir())
	saveID := "save_lock_test"

	lock, err := store.AcquireLock(ctx, saveID)
	if err != nil {
		t.Fatalf("first AcquireLock: %v", err)
	}
	if err := lock.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}

	second, err := store.AcquireLock(ctx, saveID)
	if err != nil {
		t.Fatalf("AcquireLock after release: %v", err)
	}
	_ = second.Release()
}

func TestAcquireLockCreatesMissingSaveDirectory(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	store := NewFileStore(dataDir)
	saveID := "save_new_game_test"

	lock, err := store.AcquireLock(ctx, saveID)
	if err != nil {
		t.Fatalf("AcquireLock on empty data dir: %v", err)
	}
	defer lock.Release()

	if _, err := os.Stat(filepath.Join(dataDir, "saves", saveID)); err != nil {
		t.Fatalf("save directory not created: %v", err)
	}
}

func TestListSavesFiltersByCampaignAndOrdersByUpdatedAtDescending(t *testing.T) {
	ctx := context.Background()
	store := NewFileStore(t.TempDir())

	other := game.NewStarterSave(game.NewGameRequest{Name: "KhacCampaign", CampaignID: "other-campaign"})
	if err := store.SaveSnapshot(ctx, other); err != nil {
		t.Fatalf("seed other-campaign save: %v", err)
	}
	older := game.NewStarterSave(game.NewGameRequest{Name: "Cu", CampaignID: "thanh-van-sect"})
	if err := store.SaveSnapshot(ctx, older); err != nil {
		t.Fatalf("seed older save: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	newer := game.NewStarterSave(game.NewGameRequest{Name: "Moi", CampaignID: "thanh-van-sect"})
	if err := store.SaveSnapshot(ctx, newer); err != nil {
		t.Fatalf("seed newer save: %v", err)
	}

	summaries, err := store.ListSaves(ctx, "thanh-van-sect")
	if err != nil {
		t.Fatalf("ListSaves: %v", err)
	}
	if len(summaries) != 2 {
		t.Fatalf("summaries = %#v, want 2 (other-campaign save excluded)", summaries)
	}
	if summaries[0].SaveID != newer.SaveID || summaries[1].SaveID != older.SaveID {
		t.Fatalf("order = [%q, %q], want [%q, %q]", summaries[0].SaveID, summaries[1].SaveID, newer.SaveID, older.SaveID)
	}
}

func TestListSavesTieBreaksEqualUpdatedAtBySaveIDDescending(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	store := NewFileStore(dataDir)

	first := game.NewStarterSave(game.NewGameRequest{Name: "A", CampaignID: "thanh-van-sect"})
	first.SaveID = "save_0000000000000001_1"
	second := game.NewStarterSave(game.NewGameRequest{Name: "B", CampaignID: "thanh-van-sect"})
	second.SaveID = "save_0000000000000002_1"
	if err := store.SaveSnapshot(ctx, first); err != nil {
		t.Fatalf("seed first: %v", err)
	}
	if err := store.SaveSnapshot(ctx, second); err != nil {
		t.Fatalf("seed second: %v", err)
	}
	sameTime := time.Now()
	for _, id := range []string{first.SaveID, second.SaveID} {
		path := filepath.Join(dataDir, "saves", id, "state.json")
		if err := os.Chtimes(path, sameTime, sameTime); err != nil {
			t.Fatalf("Chtimes %s: %v", id, err)
		}
	}

	summaries, err := store.ListSaves(ctx, "thanh-van-sect")
	if err != nil {
		t.Fatalf("ListSaves: %v", err)
	}
	if len(summaries) != 2 || summaries[0].SaveID != second.SaveID {
		t.Fatalf("order = %#v, want %q first (SaveID descending tie-break)", summaries, second.SaveID)
	}
}

func TestListSavesSkipsEntryWithMismatchedEmbeddedSaveID(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	store := NewFileStore(dataDir)

	valid := game.NewStarterSave(game.NewGameRequest{Name: "Hop Le", CampaignID: "thanh-van-sect"})
	if err := store.SaveSnapshot(ctx, valid); err != nil {
		t.Fatalf("seed valid save: %v", err)
	}

	mismatched := game.NewStarterSave(game.NewGameRequest{Name: "Sai Lech", CampaignID: "thanh-van-sect"})
	if err := store.SaveSnapshot(ctx, mismatched); err != nil {
		t.Fatalf("seed mismatched save: %v", err)
	}
	corrupted := mismatched
	corrupted.SaveID = "save_other_id"
	data, err := json.MarshalIndent(corrupted, "", "  ")
	if err != nil {
		t.Fatalf("marshal corrupted: %v", err)
	}
	mismatchedPath := filepath.Join(dataDir, "saves", mismatched.SaveID, "state.json")
	if err := os.WriteFile(mismatchedPath, data, 0o644); err != nil {
		t.Fatalf("write corrupted state.json: %v", err)
	}

	summaries, err := store.ListSaves(ctx, "thanh-van-sect")
	if err != nil {
		t.Fatalf("ListSaves: %v", err)
	}
	if len(summaries) != 1 || summaries[0].SaveID != valid.SaveID {
		t.Fatalf("summaries = %#v, want only %q", summaries, valid.SaveID)
	}
}
