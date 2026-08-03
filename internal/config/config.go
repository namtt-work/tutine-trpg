package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	LLM     LLMConfig     `yaml:"llm"`
	Storage StorageConfig `yaml:"storage"`
	Debug   DebugConfig   `yaml:"debug"`
}

type LLMConfig struct {
	BaseURL        string `yaml:"base_url"`
	APIKeyEnv      string `yaml:"api_key_env"`
	Model          string `yaml:"model"`
	TimeoutSeconds int    `yaml:"timeout_seconds"`
	MaxRetries     int    `yaml:"max_retries"`
}

type StorageConfig struct {
	DataDir string `yaml:"data_dir"`
}

type DebugConfig struct {
	LogLLMRequests bool `yaml:"log_llm_requests"`
	LogRetrieval   bool `yaml:"log_retrieval"`
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}
