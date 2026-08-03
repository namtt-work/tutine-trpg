package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	err := os.WriteFile(path, []byte("llm:\n  base_url: https://api.groq.com/openai/v1\n  api_key_env: GROQ_API_KEY\n  model: test-model\n  timeout_seconds: 45\nstorage:\n  data_dir: ./data\ndebug:\n  log_retrieval: true\n"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.LLM.Model != "test-model" || cfg.Storage.DataDir != "./data" || !cfg.Debug.LogRetrieval {
		t.Fatalf("unexpected config: %#v", cfg)
	}
}
