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

func writeTestConfig(t *testing.T, dataDir string, apiKeyEnv string) string {
	t.Helper()
	path := filepath.Join(dataDir, "llm.yaml")
	content := "llm:\n  base_url: https://api.groq.com/openai/v1\n  api_key_env: " + apiKeyEnv + "\n  model: test-model\n  timeout_seconds: 5\n  max_retries: 0\nstorage:\n  data_dir: " + filepath.ToSlash(dataDir) + "\ndebug:\n  log_retrieval: true\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

type recordingSession struct {
	save    game.SaveGame
	inputs  []string
	results []*game.TurnResult
}

func (s *recordingSession) HandleTurn(ctx context.Context, input orchestrator.PlayerInput) (*game.TurnResult, error) {
	s.inputs = append(s.inputs, input.Text)
	s.save.CurrentTurn++
	if len(s.results) == 0 {
		return &game.TurnResult{}, nil
	}
	result := s.results[0]
	s.results = s.results[1:]
	return result, nil
}

func (s *recordingSession) Save() game.SaveGame {
	return s.save
}

func equalStrings(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func TestBuildSessionUsesPrivateDataPermissions(t *testing.T) {
	t.Setenv("TEST_GROQ_API_KEY", "secret-test-key")
	dataDir := t.TempDir() // writeTestConfig writes dataDir/llm.yaml directly; no pre-created subdir
	cfgPath := writeTestConfig(t, dataDir, "TEST_GROQ_API_KEY")
	_, _, cleanup, err := buildSession(context.Background(), cfgPath, StartupOptions{PlayerName: "Nam"})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	for path, what := range map[string]string{
		filepath.Join(dataDir, "debug.log"): "debug.log",
	} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", what, err)
		}
		if perm := info.Mode().Perm(); perm&0o077 != 0 {
			t.Fatalf("%s permissions = %#o, want no group/other access", what, perm)
		}
	}
	// t.TempDir() pre-creates dataDir itself (0775 under Go 1.26's umask
	// handling), so assert on the artifacts buildSession creates: the fresh
	// save directory (MkdirAll 0o700) and its state.json (0600).
	savesDir := filepath.Join(dataDir, "saves")
	entries, err := os.ReadDir(savesDir)
	if err != nil {
		t.Fatalf("read saves dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("saves dirs = %d, want exactly one created by buildSession", len(entries))
	}
	saveDir := filepath.Join(savesDir, entries[0].Name())
	for path, what := range map[string]string{
		saveDir:                              "save directory",
		filepath.Join(saveDir, "state.json"): "state.json",
	} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", what, err)
		}
		if perm := info.Mode().Perm(); perm&0o077 != 0 {
			t.Fatalf("%s permissions = %#o, want no group/other access", what, perm)
		}
	}
}
