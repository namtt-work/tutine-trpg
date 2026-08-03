package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/namtt/tutine-trpg/internal/game"
	"github.com/namtt/tutine-trpg/internal/orchestrator"
)

func TestBuildSessionRejectsMissingAPIKey(t *testing.T) {
	t.Setenv("TEST_GROQ_API_KEY", "")
	cfgPath := writeTestConfig(t, t.TempDir(), "TEST_GROQ_API_KEY")

	_, cleanup, err := buildSession(context.Background(), cfgPath, "Nam")
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

	session, cleanup, err := buildSession(context.Background(), cfgPath, "Nam")
	if err != nil {
		t.Fatalf("buildSession returned error: %v", err)
	}
	defer cleanup()
	if session.Save().Player.Name != "Nam" {
		t.Fatalf("player name = %q, want Nam", session.Save().Player.Name)
	}
}

func TestBuildSessionUsesDistinctSaveStorage(t *testing.T) {
	t.Setenv("TEST_GROQ_API_KEY", "secret-test-key")
	dataDir := t.TempDir()
	cfgPath := writeTestConfig(t, dataDir, "TEST_GROQ_API_KEY")
	first, firstCleanup, err := buildSession(context.Background(), cfgPath, "Nam")
	if err != nil {
		t.Fatalf("build first session: %v", err)
	}
	firstCleanup()

	second, secondCleanup, err := buildSession(context.Background(), cfgPath, "Nam")
	if err != nil {
		t.Fatalf("build second session: %v", err)
	}
	defer secondCleanup()
	if first.Save().SaveID == second.Save().SaveID {
		t.Fatalf("save IDs match: %q", first.Save().SaveID)
	}
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
