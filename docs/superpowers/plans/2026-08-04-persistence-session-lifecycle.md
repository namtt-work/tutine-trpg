# Persistence And Session Lifecycle Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make a save durable across process runs by adding a filesystem `internal/storage` package (snapshot, event log, discovery, advisory lock), wiring it into the turn loop, and changing CLI startup to resume the latest save by default instead of always starting a fresh one.

**Architecture:** A new `internal/storage` package owns all save-directory I/O behind a `Store` interface (`FileStore` is the only implementation). `orchestrator.Session` gains a `storage.Store` dependency and persists a snapshot + event after every resolved turn, in the same "warn, don't fail the turn" style it already uses for memory extraction. `cmd/tu-tien-cli` resolves which save to use (explicit `--save`, `--new`, or auto-resume) before building the session, acquiring the save's lock as part of that resolution.

**Tech Stack:** Go 1.24, standard library only for the new package (`os`, `path/filepath`, `encoding/json`, `sort`, `context`, `time`). No new dependencies.

**Source spec:** `docs/superpowers/specs/2026-08-04-persistence-session-lifecycle-design.md` — read it if a step here seems to skip context; this plan implements it section by section but does not restate the rationale for every decision.

## Global Constraints

- Go module `github.com/namtt/tutine-trpg`, Go 1.24. No new dependencies required by this plan.
- Run `gofmt -l` and fix any listed file before each task's commit; the codebase has no lint config beyond `gofmt`.
- No test may call a real LLM provider or touch the filesystem outside `t.TempDir()`. Use `llm.FakeClient` / the fakes already in `internal/orchestrator`'s test file for LLM and memory; use a real `storage.FileStore` rooted at `t.TempDir()` where the spec calls for it (matches the existing convention in `cmd/tu-tien-cli/main_test.go`, which already uses real config files and a real `memory.SQLiteStore`).
- `internal/storage` must not import `internal/llm`, `internal/memory`, or any `cmd/...` package — it may only depend on `internal/game` (for `SaveGame`, `StateChangeView`) and the standard library.
- `internal/game` remains the sole authority for state mutation. Nothing in this plan lets the LLM write state directly; persistence only ever writes what `internal/game` already produced.
- Run `go test ./...` before the final commit of each task.
- Commits: one per task (or per step group where noted), scoped to that task's files, no AI co-author footer (per this repo's `AGENTS.md`).

---

## File Structure

```txt
internal/storage/
  store.go            NEW  Store, Lock interfaces; Event, SaveSummary types; EventTypeTurnResolved.
  file_store.go        NEW  FileStore: validateSaveID, SaveSnapshot, LoadSnapshot, AppendEvent,
                             AcquireLock/fileLock, ListSaves.
  file_store_test.go   NEW  Tests for all of the above.

internal/orchestrator/
  session.go           MODIFY  NewSession takes a storage.Store; HandleTurn persists after
                                memory extraction.
  session_test.go      MODIFY  fakeStore test double; every existing NewSession(...) call site
                                gets a store argument; two new tests for persistence behavior.

cmd/tu-tien-cli/
  main.go               MODIFY  StartupOptions type; buildSession takes StartupOptions instead of
                                 a bare name; resolveStartupSave implements the resolution order;
                                 main() gains --save/--new flags.
  main_test.go          MODIFY  Existing buildSession call sites updated for the new signature;
                                 TestBuildSessionUsesDistinctSaveStorage rewritten; new tests for
                                 auto-resume, explicit save, lock conflicts, and initial persistence.
  tui.go                MODIFY  tempViewSave; /save in the command palette, handleCommand, and
                                 renderTempViewBody; formatSaveConfirmation; updated helpText.
  tui_test.go           MODIFY  New test asserting /save shows the turn number without the raw
                                 save ID.
```

---

### Task 1: `internal/storage` — snapshot round-trip

**Files:**
- Create: `internal/storage/store.go`
- Create: `internal/storage/file_store.go`
- Create: `internal/storage/file_store_test.go`

**Interfaces:**
- Consumes: `game.SaveGame`, `game.NewStarterSave`, `game.NewGameRequest` (existing, `internal/game`).
- Produces (used by Task 2 and later tasks): `storage.Store` interface, `storage.Lock` interface, `storage.Event`, `storage.SaveSummary`, `storage.EventTypeTurnResolved`, `storage.NewFileStore(dataDir string) *FileStore`, `(*FileStore) SaveSnapshot(ctx, save game.SaveGame) error`, `(*FileStore) LoadSnapshot(ctx, saveID string) (game.SaveGame, error)`, the unexported `validateSaveID(id string) error`.

- [ ] **Step 1: Write the failing tests**

Create `internal/storage/file_store_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/storage/...`
Expected: FAIL — the package does not exist yet (`no Go files in .../internal/storage` or `undefined: NewFileStore`).

- [ ] **Step 3: Implement the minimal code**

Create `internal/storage/store.go`:

```go
package storage

import (
	"context"
	"time"

	"github.com/namtt/tutine-trpg/internal/game"
)

// Store is the persistence boundary for a save's durable state: the
// snapshot used to resume, the append-only turn history, save discovery for
// auto-resume, and the per-save advisory lock. internal/orchestrator and
// cmd/tu-tien-cli depend on this interface, never on FileStore directly.
type Store interface {
	SaveSnapshot(ctx context.Context, save game.SaveGame) error
	LoadSnapshot(ctx context.Context, saveID string) (game.SaveGame, error)
	AppendEvent(ctx context.Context, saveID string, event Event) error
	ListSaves(ctx context.Context, campaignID string) ([]SaveSummary, error)
	AcquireLock(ctx context.Context, saveID string) (Lock, error)
}

// Lock is held for the lifetime of a session on one save; Release removes it.
type Lock interface {
	Release() error
}

// EventTypeTurnResolved is the Event.Type written for the roleplay/combat
// turns orchestrator.Session.HandleTurn resolves in this phase.
const EventTypeTurnResolved = "turn_resolved"

// Event is one append-only entry in a save's events.jsonl: audit and
// debugging history. state.json, not this, is what resume reads.
type Event struct {
	Turn            int                    `json:"turn"`
	Type            string                 `json:"type"`
	PlayerAction    string                 `json:"player_action"`
	ResolvedEffects []game.StateChangeView `json:"resolved_effects"`
	Narration       string                 `json:"narration"`
	Warnings        []string               `json:"warnings,omitempty"`
	CreatedAt       time.Time              `json:"created_at"`
}

// SaveSummary describes one discoverable save for ListSaves without the
// caller loading the full game.SaveGame.
type SaveSummary struct {
	SaveID       string
	CampaignID   string
	PlayerName   string
	CurrentTurn  int
	CurrentScene string
	UpdatedAt    time.Time
}
```

Create `internal/storage/file_store.go`:

```go
package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/namtt/tutine-trpg/internal/game"
)

// FileStore is the filesystem Store implementation: one directory per save
// under <dataDir>/saves/<save_id>/.
type FileStore struct {
	dataDir string
}

func NewFileStore(dataDir string) *FileStore {
	return &FileStore{dataDir: dataDir}
}

func (fs *FileStore) saveDir(saveID string) string {
	return filepath.Join(fs.dataDir, "saves", saveID)
}

// validateSaveID rejects anything that isn't a single path segment. This
// closes the traversal case (../.., absolute paths) for every FileStore
// method that turns a caller-supplied ID into a path.
func validateSaveID(id string) error {
	if id == "" || id != filepath.Base(id) || id == "." || id == ".." {
		return fmt.Errorf("invalid save id %q", id)
	}
	return nil
}

func (fs *FileStore) SaveSnapshot(ctx context.Context, save game.SaveGame) error {
	if err := validateSaveID(save.SaveID); err != nil {
		return err
	}
	dir := fs.saveDir(save.SaveID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create save directory: %w", err)
	}
	data, err := json.MarshalIndent(save, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal save snapshot: %w", err)
	}

	tmp, err := os.CreateTemp(dir, "state-*.json.tmp")
	if err != nil {
		return fmt.Errorf("create temp snapshot file: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write temp snapshot file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("sync temp snapshot file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close temp snapshot file: %w", err)
	}
	if err := os.Rename(tmpPath, filepath.Join(dir, "state.json")); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename snapshot into place: %w", err)
	}
	return nil
}

// LoadSnapshot cross-checks the deserialized SaveGame.SaveID against the
// requested ID: a directory name is trusted by construction (only
// AcquireLock/SaveSnapshot create one, from an already-validated ID), but
// state.json is a plain file a player could hand-edit or a bug could write
// incorrectly, and callers derive other paths (game.db) from this same ID.
func (fs *FileStore) LoadSnapshot(ctx context.Context, saveID string) (game.SaveGame, error) {
	if err := validateSaveID(saveID); err != nil {
		return game.SaveGame{}, err
	}
	path := filepath.Join(fs.saveDir(saveID), "state.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return game.SaveGame{}, fmt.Errorf("save %q not found", saveID)
		}
		return game.SaveGame{}, fmt.Errorf("read snapshot for save %q: %w", saveID, err)
	}
	var save game.SaveGame
	if err := json.Unmarshal(data, &save); err != nil {
		return game.SaveGame{}, fmt.Errorf("decode snapshot for save %q: %w", saveID, err)
	}
	if save.SaveID != saveID {
		return game.SaveGame{}, fmt.Errorf("save %q is corrupted: embedded save id %q does not match its directory", saveID, save.SaveID)
	}
	return save, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/storage/... -v`
Expected: PASS for all five tests.

- [ ] **Step 5: Format and commit**

```bash
gofmt -l internal/storage
git add internal/storage/store.go internal/storage/file_store.go internal/storage/file_store_test.go
git commit -m "feat: add storage.Store snapshot round-trip with save id validation"
```

---

### Task 2: `internal/storage` — event log, lock, discovery

**Files:**
- Modify: `internal/storage/file_store.go`
- Modify: `internal/storage/file_store_test.go`

**Interfaces:**
- Consumes: everything from Task 1 (`FileStore`, `validateSaveID`, `Event`, `SaveSummary`, `Lock`).
- Produces (used by Task 4): `(*FileStore) AppendEvent(ctx, saveID string, event Event) error`, `(*FileStore) AcquireLock(ctx, saveID string) (Lock, error)`, `(*FileStore) ListSaves(ctx, campaignID string) ([]SaveSummary, error)`. After this task `*FileStore` fully implements `Store`.

- [ ] **Step 1: Write the failing tests**

Append to `internal/storage/file_store_test.go` (add `"sort"` is not needed here; add `"strings"` and `"time"` to the import block):

```go
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
```

Add these test functions to the same file:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/storage/...`
Expected: FAIL to compile — `AppendEvent`, `AcquireLock`, `ListSaves` are undefined on `*FileStore`.

- [ ] **Step 3: Implement**

Append to `internal/storage/file_store.go` (add `"sort"` to the import block, alongside the existing imports from Task 1):

```go
func (fs *FileStore) AppendEvent(ctx context.Context, saveID string, event Event) error {
	if err := validateSaveID(saveID); err != nil {
		return err
	}
	dir := fs.saveDir(saveID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create save directory: %w", err)
	}
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	f, err := os.OpenFile(filepath.Join(dir, "events.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open event log: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write event: %w", err)
	}
	return f.Sync()
}

type fileLock struct {
	path string
}

func (l *fileLock) Release() error {
	return os.Remove(l.path)
}

// AcquireLock ensures the save directory exists (a brand new save has none
// yet, since game.NewStarterSave performs no I/O) before creating .lock with
// O_EXCL, so a second process racing on the same save fails immediately
// instead of silently overwriting state.
func (fs *FileStore) AcquireLock(ctx context.Context, saveID string) (Lock, error) {
	if err := validateSaveID(saveID); err != nil {
		return nil, err
	}
	dir := fs.saveDir(saveID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create save directory: %w", err)
	}
	lockPath := filepath.Join(dir, ".lock")
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return nil, fmt.Errorf("save %q is already open in another process (remove %s if that process is no longer running)", saveID, lockPath)
		}
		return nil, fmt.Errorf("acquire lock for save %q: %w", saveID, err)
	}
	_, writeErr := fmt.Fprintf(f, "%d", os.Getpid())
	closeErr := f.Close()
	if writeErr != nil || closeErr != nil {
		os.Remove(lockPath)
		if writeErr != nil {
			return nil, fmt.Errorf("write lock pid: %w", writeErr)
		}
		return nil, fmt.Errorf("close lock file: %w", closeErr)
	}
	return &fileLock{path: lockPath}, nil
}

// ListSaves reuses LoadSnapshot's SaveID cross-check while scanning: an
// entry whose state.json embeds a different SaveID than its directory name
// is silently skipped rather than returned, so a corrupted or malformed
// save never becomes an auto-resume candidate.
func (fs *FileStore) ListSaves(ctx context.Context, campaignID string) ([]SaveSummary, error) {
	savesRoot := filepath.Join(fs.dataDir, "saves")
	entries, err := os.ReadDir(savesRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read saves directory: %w", err)
	}

	var summaries []SaveSummary
	for _, entry := range entries {
		if !entry.IsDir() || validateSaveID(entry.Name()) != nil {
			continue
		}
		statePath := filepath.Join(savesRoot, entry.Name(), "state.json")
		info, err := os.Stat(statePath)
		if err != nil {
			continue
		}
		save, err := fs.LoadSnapshot(ctx, entry.Name())
		if err != nil {
			continue
		}
		if save.CampaignID != campaignID {
			continue
		}
		summaries = append(summaries, SaveSummary{
			SaveID:       save.SaveID,
			CampaignID:   save.CampaignID,
			PlayerName:   save.Player.Name,
			CurrentTurn:  save.CurrentTurn,
			CurrentScene: save.CurrentScene,
			UpdatedAt:    info.ModTime(),
		})
	}

	sort.Slice(summaries, func(i, j int) bool {
		if !summaries[i].UpdatedAt.Equal(summaries[j].UpdatedAt) {
			return summaries[i].UpdatedAt.After(summaries[j].UpdatedAt)
		}
		return summaries[i].SaveID > summaries[j].SaveID
	})
	return summaries, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/storage/... -v`
Expected: PASS for every test in the package (11 tests total between Task 1 and Task 2).

- [ ] **Step 5: Format and commit**

```bash
gofmt -l internal/storage
git add internal/storage/file_store.go internal/storage/file_store_test.go
git commit -m "feat: add event log, advisory lock, and save discovery to storage.FileStore"
```

---

### Task 3: Wire `storage.Store` into `orchestrator.Session`

**Files:**
- Modify: `internal/orchestrator/session.go`
- Modify: `internal/orchestrator/session_test.go`

**Interfaces:**
- Consumes: `storage.Store`, `storage.Event`, `storage.EventTypeTurnResolved` (Tasks 1–2).
- Produces (used by Task 4): `orchestrator.NewSession(save game.SaveGame, client llm.Client, memories memory.Store, store storage.Store, allowedTags []string) *Session` — signature changed from the current 4-argument form by inserting `store storage.Store` before `allowedTags`.

- [ ] **Step 1: Update the test file — fake store, all call sites, two new tests**

In `internal/orchestrator/session_test.go`, add `"time"` and `"github.com/namtt/tutine-trpg/internal/storage"` to the imports:

```go
import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/namtt/tutine-trpg/internal/game"
	"github.com/namtt/tutine-trpg/internal/llm"
	"github.com/namtt/tutine-trpg/internal/memory"
	"github.com/namtt/tutine-trpg/internal/storage"
)
```

Add the fake store test double, next to `recordingStore`:

```go
type fakeStore struct {
	snapshotErr error
	eventErr    error
	snapshots   []game.SaveGame
	events      []storage.Event
	callOrder   []string
}

func (s *fakeStore) SaveSnapshot(_ context.Context, save game.SaveGame) error {
	s.callOrder = append(s.callOrder, "snapshot")
	s.snapshots = append(s.snapshots, save)
	return s.snapshotErr
}

func (s *fakeStore) LoadSnapshot(context.Context, string) (game.SaveGame, error) {
	return game.SaveGame{}, errors.New("fakeStore: LoadSnapshot not used by HandleTurn tests")
}

func (s *fakeStore) AppendEvent(_ context.Context, _ string, event storage.Event) error {
	s.callOrder = append(s.callOrder, "event")
	s.events = append(s.events, event)
	return s.eventErr
}

func (s *fakeStore) ListSaves(context.Context, string) ([]storage.SaveSummary, error) {
	return nil, nil
}

func (s *fakeStore) AcquireLock(context.Context, string) (storage.Lock, error) {
	return nil, errors.New("fakeStore: AcquireLock not used by HandleTurn tests")
}
```

Update every existing `NewSession(...)` call site to pass a `&fakeStore{}` before the trailing `allowedTags` argument:

| Test | Old call | New call |
|---|---|---|
| `TestHandleTurnReturnsNarrationAndAdvancesTurn` | `NewSession(save, llm.FakeClient{}, store, []string{"trust", "secret"})` | `NewSession(save, llm.FakeClient{}, store, &fakeStore{}, []string{"trust", "secret"})` |
| `TestHandleTurnProvidesRecentNarrationToNextTurn` | `NewSession(game.NewStarterSave(game.NewGameRequest{Name: "Nam"}), client, &recordingStore{}, nil)` | `NewSession(game.NewStarterSave(game.NewGameRequest{Name: "Nam"}), client, &recordingStore{}, &fakeStore{}, nil)` |
| `TestHandleTurnReturnsResolvedTurnWhenMemoryExtractionFails` | `NewSession(save, extractorFailingClient{}, store, []string{"trust", "secret"})` | `NewSession(save, extractorFailingClient{}, store, &fakeStore{}, []string{"trust", "secret"})` |
| `TestHandleTurnRejectsInvalidLLMEffectsWithoutFailingTurn` | `NewSession(save, invalidEffectsClient{}, store, []string{"trust", "secret"})` | `NewSession(save, invalidEffectsClient{}, store, &fakeStore{}, []string{"trust", "secret"})` |
| `TestHandleTurnUsesToolCapableClientRollCheck` | `NewSession(save, client, &recordingStore{}, nil)` | `NewSession(save, client, &recordingStore{}, &fakeStore{}, nil)` |
| `TestExecuteToolRejectsUnknownStat` | `NewSession(save, llm.FakeClient{}, nil, nil)` | `NewSession(save, llm.FakeClient{}, nil, &fakeStore{}, nil)` |
| `TestExecuteToolRejectsUnknownToolName` | `NewSession(save, llm.FakeClient{}, nil, nil)` | `NewSession(save, llm.FakeClient{}, nil, &fakeStore{}, nil)` |
| `TestSaveReturnsIndependentCopy` | `NewSession(save, llm.FakeClient{}, nil, nil)` | `NewSession(save, llm.FakeClient{}, nil, &fakeStore{}, nil)` |
| `TestHandleTurnForwardsAllRetrievalPlanFilters` | `NewSession(game.NewStarterSave(game.NewGameRequest{Name: "Nam"}), plannedRetrievalClient{plan: plan}, store, nil)` | `NewSession(game.NewStarterSave(game.NewGameRequest{Name: "Nam"}), plannedRetrievalClient{plan: plan}, store, &fakeStore{}, nil)` |
| `TestHandleTurnRejectsEmptyInputWithoutAdvancingState` | `NewSession(save, llm.FakeClient{}, &recordingStore{}, nil)` | `NewSession(save, llm.FakeClient{}, &recordingStore{}, &fakeStore{}, nil)` |

Add two new tests, appended at the end of the file:

```go
func TestHandleTurnPersistsSnapshotAndEventInOrder(t *testing.T) {
	ctx := context.Background()
	memStore, err := memory.NewSQLiteStore(ctx, filepath.Join(t.TempDir(), "game.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer memStore.Close()

	save := game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})
	store := &fakeStore{}
	session := NewSession(save, llm.FakeClient{}, memStore, store, []string{"trust", "secret"})

	result, err := session.HandleTurn(ctx, PlayerInput{Text: "ta quan sat cong mon"})
	if err != nil {
		t.Fatalf("HandleTurn returned error: %v", err)
	}
	if result.Narration == "" {
		t.Fatal("expected narration")
	}
	if len(store.snapshots) != 1 || store.snapshots[0].CurrentTurn != 1 {
		t.Fatalf("snapshots = %#v, want one snapshot at turn 1", store.snapshots)
	}
	if len(store.events) != 1 || store.events[0].Turn != 1 || store.events[0].Type != storage.EventTypeTurnResolved {
		t.Fatalf("events = %#v, want one turn_resolved event at turn 1", store.events)
	}
	if !reflect.DeepEqual(store.callOrder, []string{"snapshot", "event"}) {
		t.Fatalf("call order = %#v, want snapshot before event", store.callOrder)
	}
}

func TestHandleTurnWarnsButDoesNotFailWhenPersistenceFails(t *testing.T) {
	ctx := context.Background()
	save := game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})
	store := &fakeStore{snapshotErr: errors.New("disk full"), eventErr: errors.New("disk full")}
	session := NewSession(save, llm.FakeClient{}, &recordingStore{}, store, nil)

	result, err := session.HandleTurn(ctx, PlayerInput{Text: "ta quan sat cong mon"})
	if err != nil {
		t.Fatalf("HandleTurn returned error: %v", err)
	}
	if result.Narration == "" {
		t.Fatal("expected narration despite persistence failure")
	}
	foundSnapshotWarning, foundEventWarning := false, false
	for _, w := range result.Warnings {
		if strings.Contains(w, "save persistence failed") {
			foundSnapshotWarning = true
		}
		if strings.Contains(w, "event log write failed") {
			foundEventWarning = true
		}
	}
	if !foundSnapshotWarning || !foundEventWarning {
		t.Fatalf("warnings = %#v, want both save and event failure warnings", result.Warnings)
	}
	if len(store.events) != 1 {
		t.Fatalf("events = %d, want AppendEvent still attempted once even though SaveSnapshot failed", len(store.events))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/orchestrator/...`
Expected: FAIL to compile — `too many arguments in call to NewSession` (or `not enough arguments`) at every call site, since `session.go` doesn't accept a `storage.Store` yet.

- [ ] **Step 3: Implement — update `session.go`**

Add `"time"` and `"github.com/namtt/tutine-trpg/internal/storage"` to the import block:

```go
import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/namtt/tutine-trpg/internal/game"
	"github.com/namtt/tutine-trpg/internal/llm"
	"github.com/namtt/tutine-trpg/internal/memory"
	"github.com/namtt/tutine-trpg/internal/storage"
)
```

Change the `Session` struct and `NewSession`:

```go
type Session struct {
	save        game.SaveGame
	client      llm.Client
	memories    memory.Store
	storage     storage.Store
	allowedTags []string
	recentTurns []llm.RecentTurn
	rollFunc    func() int
}

func NewSession(save game.SaveGame, client llm.Client, memories memory.Store, store storage.Store, allowedTags []string) *Session {
	return &Session{save: save, client: client, memories: memories, storage: store, allowedTags: append([]string{}, allowedTags...), rollFunc: defaultRoll}
}
```

In `HandleTurn`, insert the persistence block after the existing memory-extraction `for`/`if` block and before the final `return`, matching the MVP design's turn-flow order (memory extraction, then storage):

```go
func (s *Session) HandleTurn(ctx context.Context, input PlayerInput) (*game.TurnResult, error) {
	input.Text = strings.TrimSpace(input.Text)
	if input.Text == "" {
		return nil, errors.New("player input is required")
	}

	plan, err := s.client.PlanRetrieval(ctx, llm.PlannerRequest{PlayerAction: input.Text, SceneID: s.save.CurrentScene, AllowedTags: s.allowedTags, NearbyIDs: []string{"player"}})
	if err != nil {
		return nil, err
	}

	hits, err := s.memories.Search(ctx, memory.Query{SaveID: s.save.SaveID, Entities: plan.Entities, Tags: plan.Tags, Types: plan.MemoryTypes, Locations: plan.Locations, QuestIDs: plan.QuestIDs, Keywords: plan.Keywords, MaxResults: plan.MaxResults})
	if err != nil {
		return nil, err
	}
	contextLines := make([]string, 0, len(hits))
	for _, hit := range hits {
		contextLines = append(contextLines, hit.Memory.Summary)
	}

	narration, err := s.narrate(ctx, llm.NarratorRequest{PlayerAction: input.Text, SceneID: s.save.CurrentScene, AuthoritativeState: narratorState(s.save), RecentTurns: copyRecentTurns(s.recentTurns), RetrievedContext: contextLines, AllowedEffects: []string{game.EffectEnergyDelta, game.EffectRelationshipDelta, game.EffectGrantItem}})
	if err != nil {
		return nil, err
	}

	changes, warnings := s.applyProposedEffects(narration.ProposedEffects)
	s.save.CurrentTurn++
	s.rememberTurn(llm.RecentTurn{PlayerAction: input.Text, Narration: narration.Narration, ResolvedChanges: changes})

	drafts, err := s.client.ExtractMemories(ctx, llm.ExtractorRequest{PlayerAction: input.Text, Narration: narration.Narration, ResolvedChanges: changes, AllowedTags: s.allowedTags})
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("memory extraction failed: %v", err))
	} else {
		for i, draft := range drafts {
			memoryID := fmt.Sprintf("%s_turn_%d_%d", s.save.SaveID, s.save.CurrentTurn, i)
			if err := s.memories.Add(ctx, memory.Memory{ID: memoryID, SaveID: s.save.SaveID, CampaignID: s.save.CampaignID, Turn: s.save.CurrentTurn, Type: draft.Type, Importance: draft.Importance, Text: draft.Text, Summary: draft.Text, Entities: draft.Entities, Tags: filterTags(draft.Tags, s.allowedTags), FactsJSON: draft.FactsJSON}); err != nil {
				warnings = append(warnings, fmt.Sprintf("memory persistence failed: %v", err))
			}
		}
	}

	if err := s.storage.SaveSnapshot(ctx, s.save); err != nil {
		warnings = append(warnings, fmt.Sprintf("save persistence failed: %v", err))
	}
	if err := s.storage.AppendEvent(ctx, s.save.SaveID, storage.Event{
		Turn:            s.save.CurrentTurn,
		Type:            storage.EventTypeTurnResolved,
		PlayerAction:    input.Text,
		ResolvedEffects: changes,
		Narration:       narration.Narration,
		Warnings:        warnings,
		CreatedAt:       time.Now().UTC(),
	}); err != nil {
		warnings = append(warnings, fmt.Sprintf("event log write failed: %v", err))
	}

	return &game.TurnResult{Narration: narration.Narration, StateChanges: changes, SuggestedActions: narration.SuggestedNextOptions, Warnings: warnings}, nil
}
```

No other function in `session.go` changes.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/orchestrator/... -v`
Expected: PASS for all existing tests plus the two new ones.

- [ ] **Step 5: Format and commit**

```bash
gofmt -l internal/orchestrator
git add internal/orchestrator/session.go internal/orchestrator/session_test.go
git commit -m "feat: persist snapshot and event after every resolved turn"
```

---

### Task 4: CLI startup — resume by default, `--save`/`--new`, initial persistence

**Files:**
- Modify: `cmd/tu-tien-cli/main.go`
- Modify: `cmd/tu-tien-cli/main_test.go`

**Interfaces:**
- Consumes: `storage.NewFileStore(dataDir string) *FileStore`, `storage.Store`, `storage.Lock` (Tasks 1–2); `orchestrator.NewSession(save, client, memories, store, allowedTags)` (Task 3, new signature).
- Produces: `StartupOptions{PlayerName, SaveID, ForceNew string/bool}`, `buildSession(ctx context.Context, configPath string, opts StartupOptions) (*orchestrator.Session, *log.Logger, func(), error)` (signature changed: third parameter is now `StartupOptions`, not a bare `string`), the unexported `resolveStartupSave`.

- [ ] **Step 1: Write the failing tests**

Replace the top of `cmd/tu-tien-cli/main_test.go` (imports and the three existing `buildSession`-calling tests) with:

```go
package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/namtt/tutine-trpg/internal/game"
	"github.com/namtt/tutine-trpg/internal/orchestrator"
	"github.com/namtt/tutine-trpg/internal/storage"
)

func TestBuildSessionRejectsMissingAPIKey(t *testing.T) {
	t.Setenv("TEST_GROQ_API_KEY", "")
	cfgPath := writeTestConfig(t, t.TempDir(), "TEST_GROQ_API_KEY")

	_, _, cleanup, err := buildSession(context.Background(), cfgPath, StartupOptions{PlayerName: "Nam"})
	if cleanup != nil {
		cleanup()
	}
	if err == nil || !strings.Contains(err.Error(), "TEST_GROQ_API_KEY") {
		t.Fatalf("err = %v, want missing API key env error", err)
	}
}

func TestBuildSessionUsesConfigAndPlayerName(t *testing.T) {
	t.Setenv("TEST_GROQ_API_KEY", "secret-test-key")
	cfgPath := writeTestConfig(t, t.TempDir(), "TEST_GROQ_API_KEY")

	session, logger, cleanup, err := buildSession(context.Background(), cfgPath, StartupOptions{PlayerName: "Nam"})
	if err != nil {
		t.Fatalf("buildSession returned error: %v", err)
	}
	defer cleanup()
	if logger == nil {
		t.Fatal("logger is nil, want non-nil debug logger")
	}
	if session.Save().Player.Name != "Nam" {
		t.Fatalf("player name = %q, want Nam", session.Save().Player.Name)
	}
}

func TestBuildSessionRejectsSaveIDAndForceNewTogether(t *testing.T) {
	t.Setenv("TEST_GROQ_API_KEY", "secret-test-key")
	cfgPath := writeTestConfig(t, t.TempDir(), "TEST_GROQ_API_KEY")

	_, _, cleanup, err := buildSession(context.Background(), cfgPath, StartupOptions{SaveID: "save_x", ForceNew: true})
	if cleanup != nil {
		cleanup()
	}
	if err == nil || !strings.Contains(err.Error(), "cannot be used together") {
		t.Fatalf("err = %v, want cannot-be-used-together error", err)
	}
}

func TestBuildSessionWritesInitialSnapshotForNewGame(t *testing.T) {
	t.Setenv("TEST_GROQ_API_KEY", "secret-test-key")
	dataDir := t.TempDir()
	cfgPath := writeTestConfig(t, dataDir, "TEST_GROQ_API_KEY")

	session, _, cleanup, err := buildSession(context.Background(), cfgPath, StartupOptions{PlayerName: "Nam"})
	if err != nil {
		t.Fatalf("buildSession returned error: %v", err)
	}
	defer cleanup()

	statePath := filepath.Join(dataDir, "saves", session.Save().SaveID, "state.json")
	if _, err := os.Stat(statePath); err != nil {
		t.Fatalf("state.json missing right after buildSession: %v", err)
	}
}

func TestBuildSessionResumesSameSaveByDefault(t *testing.T) {
	t.Setenv("TEST_GROQ_API_KEY", "secret-test-key")
	dataDir := t.TempDir()
	cfgPath := writeTestConfig(t, dataDir, "TEST_GROQ_API_KEY")

	first, _, firstCleanup, err := buildSession(context.Background(), cfgPath, StartupOptions{PlayerName: "Nam"})
	if err != nil {
		t.Fatalf("build first session: %v", err)
	}
	firstID := first.Save().SaveID
	// Release the first save's lock before the second call: otherwise the
	// second call would correctly fail lock acquisition instead of
	// resuming, which is a different case than this test.
	firstCleanup()

	second, _, secondCleanup, err := buildSession(context.Background(), cfgPath, StartupOptions{PlayerName: "Nam"})
	if err != nil {
		t.Fatalf("build second session: %v", err)
	}
	defer secondCleanup()
	if second.Save().SaveID != firstID {
		t.Fatalf("save id = %q, want resume of %q", second.Save().SaveID, firstID)
	}
}

func TestBuildSessionForceNewCreatesDistinctSaveStorage(t *testing.T) {
	t.Setenv("TEST_GROQ_API_KEY", "secret-test-key")
	dataDir := t.TempDir()
	cfgPath := writeTestConfig(t, dataDir, "TEST_GROQ_API_KEY")

	first, _, firstCleanup, err := buildSession(context.Background(), cfgPath, StartupOptions{PlayerName: "Nam"})
	if err != nil {
		t.Fatalf("build first session: %v", err)
	}
	firstCleanup()

	second, _, secondCleanup, err := buildSession(context.Background(), cfgPath, StartupOptions{PlayerName: "Nam", ForceNew: true})
	if err != nil {
		t.Fatalf("build second session: %v", err)
	}
	defer secondCleanup()
	if first.Save().SaveID == second.Save().SaveID {
		t.Fatalf("save IDs match: %q, want ForceNew to create a distinct save even though one already exists", first.Save().SaveID)
	}
}

func TestBuildSessionAutoResumesMostRecentlyUpdatedSave(t *testing.T) {
	t.Setenv("TEST_GROQ_API_KEY", "secret-test-key")
	dataDir := t.TempDir()
	cfgPath := writeTestConfig(t, dataDir, "TEST_GROQ_API_KEY")
	ctx := context.Background()
	fileStore := storage.NewFileStore(dataDir)

	older := game.NewStarterSave(game.NewGameRequest{Name: "Cu", CampaignID: "thanh-van-sect"})
	if err := fileStore.SaveSnapshot(ctx, older); err != nil {
		t.Fatalf("seed older save: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	newer := game.NewStarterSave(game.NewGameRequest{Name: "Moi", CampaignID: "thanh-van-sect"})
	if err := fileStore.SaveSnapshot(ctx, newer); err != nil {
		t.Fatalf("seed newer save: %v", err)
	}

	session, _, cleanup, err := buildSession(ctx, cfgPath, StartupOptions{PlayerName: "Nam"})
	if err != nil {
		t.Fatalf("buildSession returned error: %v", err)
	}
	defer cleanup()
	if session.Save().SaveID != newer.SaveID {
		t.Fatalf("resumed save = %q, want most recently updated %q", session.Save().SaveID, newer.SaveID)
	}
}

func TestBuildSessionExplicitSaveIDOverridesAutoResume(t *testing.T) {
	t.Setenv("TEST_GROQ_API_KEY", "secret-test-key")
	dataDir := t.TempDir()
	cfgPath := writeTestConfig(t, dataDir, "TEST_GROQ_API_KEY")
	ctx := context.Background()
	fileStore := storage.NewFileStore(dataDir)

	older := game.NewStarterSave(game.NewGameRequest{Name: "Cu", CampaignID: "thanh-van-sect"})
	if err := fileStore.SaveSnapshot(ctx, older); err != nil {
		t.Fatalf("seed older save: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	newer := game.NewStarterSave(game.NewGameRequest{Name: "Moi", CampaignID: "thanh-van-sect"})
	if err := fileStore.SaveSnapshot(ctx, newer); err != nil {
		t.Fatalf("seed newer save: %v", err)
	}

	session, _, cleanup, err := buildSession(ctx, cfgPath, StartupOptions{SaveID: older.SaveID})
	if err != nil {
		t.Fatalf("buildSession returned error: %v", err)
	}
	defer cleanup()
	if session.Save().SaveID != older.SaveID {
		t.Fatalf("resumed save = %q, want explicitly requested %q", session.Save().SaveID, older.SaveID)
	}
}

func TestBuildSessionRejectsUnknownSaveID(t *testing.T) {
	t.Setenv("TEST_GROQ_API_KEY", "secret-test-key")
	cfgPath := writeTestConfig(t, t.TempDir(), "TEST_GROQ_API_KEY")

	_, _, cleanup, err := buildSession(context.Background(), cfgPath, StartupOptions{SaveID: "save_does_not_exist"})
	if cleanup != nil {
		cleanup()
	}
	if err == nil {
		t.Fatal("expected error for unknown save id")
	}
}

func TestBuildSessionRejectsPathTraversalSaveID(t *testing.T) {
	t.Setenv("TEST_GROQ_API_KEY", "secret-test-key")
	cfgPath := writeTestConfig(t, t.TempDir(), "TEST_GROQ_API_KEY")

	_, _, cleanup, err := buildSession(context.Background(), cfgPath, StartupOptions{SaveID: "../../etc"})
	if cleanup != nil {
		cleanup()
	}
	if err == nil {
		t.Fatal("expected error for path-traversal-shaped save id")
	}
}

func TestBuildSessionRejectsAlreadyLockedSave(t *testing.T) {
	t.Setenv("TEST_GROQ_API_KEY", "secret-test-key")
	dataDir := t.TempDir()
	cfgPath := writeTestConfig(t, dataDir, "TEST_GROQ_API_KEY")

	first, _, firstCleanup, err := buildSession(context.Background(), cfgPath, StartupOptions{PlayerName: "Nam"})
	if err != nil {
		t.Fatalf("build first session: %v", err)
	}
	defer firstCleanup()
	saveID := first.Save().SaveID

	_, _, cleanup, err := buildSession(context.Background(), cfgPath, StartupOptions{SaveID: saveID})
	if cleanup != nil {
		cleanup()
	}
	if err == nil {
		t.Fatal("expected error acquiring lock on a save already open in the first session")
	}
}

func TestBuildSessionReleasesLockWhenMemoryStoreFailsToOpen(t *testing.T) {
	t.Setenv("TEST_GROQ_API_KEY", "secret-test-key")
	dataDir := t.TempDir()
	cfgPath := writeTestConfig(t, dataDir, "TEST_GROQ_API_KEY")

	first, _, firstCleanup, err := buildSession(context.Background(), cfgPath, StartupOptions{PlayerName: "Nam"})
	if err != nil {
		t.Fatalf("build first session: %v", err)
	}
	saveID := first.Save().SaveID
	firstCleanup()

	dbPath := filepath.Join(dataDir, "saves", saveID, "game.db")
	if err := os.Remove(dbPath); err != nil {
		t.Fatalf("remove game.db: %v", err)
	}
	if err := os.Mkdir(dbPath, 0o755); err != nil {
		t.Fatalf("replace game.db with a directory to force sqlite open failure: %v", err)
	}

	_, _, cleanup, err := buildSession(context.Background(), cfgPath, StartupOptions{SaveID: saveID})
	if cleanup != nil {
		cleanup()
	}
	if err == nil {
		t.Fatal("expected buildSession to fail when the memory store cannot open")
	}

	fileStore := storage.NewFileStore(dataDir)
	lock, err := fileStore.AcquireLock(context.Background(), saveID)
	if err != nil {
		t.Fatalf("AcquireLock after failed buildSession: %v, want the lock released on failure", err)
	}
	_ = lock.Release()
}
```

`writeTestConfig` at the bottom of `main_test.go`, and `recordingSession`/`equalStrings` used by `tui_test.go`, are unchanged — leave them as-is.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cmd/tu-tien-cli/...`
Expected: FAIL to compile — `undefined: StartupOptions` and `cannot use StartupOptions{...} as string value` at every `buildSession` call site.

- [ ] **Step 3: Implement — rewrite `main.go`**

Replace the full contents of `cmd/tu-tien-cli/main.go`:

```go
package main

import (
	"context"
	"errors"
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
	name := flag.String("name", "Vô Danh", "player name for a new game")
	configPath := flag.String("config", "configs/llm.yaml", "runtime config path")
	saveID := flag.String("save", "", "resume a specific save, skipping auto-resume")
	forceNew := flag.Bool("new", false, "force a new game even if a save exists")
	flag.Parse()

	opts := StartupOptions{PlayerName: *name, SaveID: *saveID, ForceNew: *forceNew}
	session, logger, cleanup, err := buildSession(context.Background(), *configPath, opts)
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
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./cmd/tu-tien-cli/... -v`
Expected: PASS for all tests in the package (existing TUI tests are unaffected since they construct `tuiModel` directly, not through `buildSession`).

- [ ] **Step 5: Format and commit**

```bash
gofmt -l cmd/tu-tien-cli
git add cmd/tu-tien-cli/main.go cmd/tu-tien-cli/main_test.go
git commit -m "feat: resume the latest save by default, add --save/--new startup flags"
```

---

### Task 5: `/save` command in the TUI

**Files:**
- Modify: `cmd/tu-tien-cli/tui.go`
- Modify: `cmd/tu-tien-cli/tui_test.go`

**Interfaces:**
- Consumes: `session.Save()` (existing, unchanged).
- Produces: `formatSaveConfirmation(save game.SaveGame) string`, `tempViewSave` (new `tempViewKind` value).

- [ ] **Step 1: Write the failing test**

Append to `cmd/tu-tien-cli/tui_test.go`:

```go
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
	view := model.View()
	if !strings.Contains(view, "lượt 7") {
		t.Fatalf("view missing turn confirmation:\n%s", view)
	}
	if strings.Contains(view, save.SaveID) {
		t.Fatalf("view leaks raw save id %q:\n%s", save.SaveID, view)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cmd/tu-tien-cli/...`
Expected: FAIL — `/save` is not recognized by `handleCommand`, so `model.notice` becomes an "unknown command" message instead of opening the save view; the view will not contain "lượt 7".

- [ ] **Step 3: Implement — update `tui.go`**

Add `tempViewSave` to the `tempViewKind` block:

```go
const (
	tempViewNone tempViewKind = iota
	tempViewStatus
	tempViewInventory
	tempViewHelp
	tempViewSave
)
```

Add a case to `handleCommand`:

```go
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
```

Add a case to `renderTempViewBody`:

```go
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
```

Add the formatting function next to `formatStatus`/`formatInventory`:

```go
// formatSaveConfirmation deliberately omits the raw save id and filesystem
// path: internal identifiers must not appear in player-facing UI. The id and
// path remain available in debug.log for anyone who needs to locate the
// file on disk.
func formatSaveConfirmation(save game.SaveGame) string {
	return fmt.Sprintf("Tiến trình đã được lưu tự động ở lượt %d.", save.CurrentTurn)
}
```

Update `renderCommandPalette`:

```go
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
```

Update `helpText`:

```go
func helpText(providerLabel string) string {
	lines := []string{
		"Hướng dẫn chơi:",
		"- Nhập hành động tự do, ví dụ: ta quan sát cổng môn.",
		"- Nhập số để chọn một gợi ý đang hiển thị.",
		"- Nhấn Tab để đưa gợi ý hiện tại vào ô nhập, có thể sửa trước khi gửi.",
		"- Lệnh: /status, /inventory, /save, /help, /exit.",
		"- Tiến trình được lưu tự động sau mỗi lượt và tự tiếp tục ở lần chơi sau.",
		"- Esc đóng màn hình đang xem hoặc thoát game.",
	}
	if strings.TrimSpace(providerLabel) != "" {
		lines = append(lines, "Mô hình đang dùng: "+providerLabel)
	}
	return strings.Join(lines, "\n")
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./cmd/tu-tien-cli/... -v`
Expected: PASS for all tests, including the new one.

Then run the full suite once to confirm nothing else regressed:

Run: `go test ./...`
Expected: PASS across every package (`internal/storage`, `internal/orchestrator`, `internal/game`, `internal/campaign`, `internal/config`, `internal/llm`, `internal/memory`, `cmd/tu-tien-cli`).

- [ ] **Step 5: Format and commit**

```bash
gofmt -l cmd/tu-tien-cli
git add cmd/tu-tien-cli/tui.go cmd/tu-tien-cli/tui_test.go
git commit -m "feat: add /save command showing turn without raw save id or path"
```

---

## Self-Review Notes

**Spec coverage:**
- `internal/storage` (Scope, Architecture, File Layout, Save ID Validation, Concurrent Access, Save Discovery Ordering) → Tasks 1–2.
- Orchestrator persistence ordering and warning behavior (Orchestrator Integration, Consistency And Crash Behavior) → Task 3.
- CLI resolution order, `--save`/`--new`, initial snapshot, canonical ID for `game.db`, lock release on failure (CLI Startup And Resume Flow, Concurrent Access) → Task 4.
- `/save` command without raw identifiers (`/save` Command) → Task 5.
- Non-Goals (mid-session hot-swap, save-picker UI, cross-file transactional consistency, distributed locking, combat/quest/reward work) are intentionally not implemented by any task above.

**Placeholder scan:** every step has literal code or a literal shell command; no "TBD", "add error handling", or "similar to Task N" shortcuts.

**Type consistency:** `storage.Store`, `storage.Lock`, `storage.Event`, `storage.SaveSummary`, `storage.EventTypeTurnResolved` are defined once in Task 1 and used with the same names/shapes in Tasks 2–4. `orchestrator.NewSession`'s signature is defined once in Task 3 and every call site in Tasks 3–4 uses that exact five-argument form. `StartupOptions` and `buildSession`'s new signature are defined once in Task 4 and used consistently across its own tests.
