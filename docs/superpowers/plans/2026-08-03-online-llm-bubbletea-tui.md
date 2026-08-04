# Online LLM And Bubble Tea TUI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace offline CLI play with a Groq-backed OpenAI-compatible LLM runtime and a Bubble Tea terminal UI.

**Architecture:** Keep `internal/orchestrator.Session` as the gameplay boundary. Add a real HTTP LLM client under `internal/llm`, wire runtime config in `cmd/tu-tien-cli`, and keep Bubble Tea code in the CLI adapter only. Tests use fake clients or fake HTTP servers and never call real provider APIs.

**Tech Stack:** Go, `net/http`, OpenAI-compatible Chat Completions JSON, Bubble Tea, Lip Gloss, SQLite memory store, existing `testing` package.

## Global Constraints

- CLI play must not expose `--offline`.
- Runtime config defaults to `configs/llm.yaml` and can be overridden with `--config <path>`.
- Default provider is Groq with `api_key_env: GROQ_API_KEY`.
- Missing config, missing API key, or missing required LLM fields must fail startup clearly.
- `llm.FakeClient` remains available for tests only.
- Default tests must not perform real network calls.
- Bubble Tea code must not leak into `internal/game`, `internal/llm`, `internal/memory`, or `internal/orchestrator`.
- Run `gofmt` on changed Go files and `go test ./...` before completion.

---

## File Structure

- `internal/llm/openai_compatible.go`: OpenAI-compatible HTTP client, config validation, request/response DTOs, retry handling, JSON parsing.
- `internal/llm/openai_compatible_test.go`: fake HTTP server tests for request shape, API key validation, JSON parsing, and retry behavior.
- `cmd/tu-tien-cli/main.go`: CLI flags, config loading, real session construction, Bubble Tea program startup.
- `cmd/tu-tien-cli/tui.go`: Bubble Tea model, update loop, command routing, turn submission, rendering.
- `cmd/tu-tien-cli/tui_test.go`: TUI command/choice behavior tests with fake sessions.
- `cmd/tu-tien-cli/main_test.go`: update session construction tests to validate online config behavior without calling network.
- `configs/llm.yaml`: runtime default config for Groq.
- `README.md` and `docs/cli-guide.md`: update run instructions from offline CLI to online TUI.

---

### Task 1: OpenAI-Compatible LLM Client

**Files:**

- Create: `internal/llm/openai_compatible.go`
- Create: `internal/llm/openai_compatible_test.go`

**Interfaces:**

- Consumes: `llm.Client`, `PlannerRequest`, `RetrievalPlan`, `NarratorRequest`, `NarratorResponse`, `ExtractorRequest`, `MemoryDraft` from `internal/llm/contracts.go`.
- Produces: `type OpenAICompatibleConfig`, `func NewOpenAICompatibleClient(cfg OpenAICompatibleConfig) (*OpenAICompatibleClient, error)`, and `OpenAICompatibleClient` implementing `llm.Client`.

- [x] **Step 1: Write failing config validation tests**

Create `internal/llm/openai_compatible_test.go` with tests like:

```go
package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewOpenAICompatibleClientRejectsMissingAPIKey(t *testing.T) {
	t.Setenv("MISSING_KEY", "")

	_, err := NewOpenAICompatibleClient(OpenAICompatibleConfig{
		BaseURL:        "https://api.groq.com/openai/v1",
		APIKeyEnv:      "MISSING_KEY",
		Model:          "llama-3.1-70b-versatile",
		TimeoutSeconds: 45,
		MaxRetries:     2,
	})
	if err == nil || !strings.Contains(err.Error(), "MISSING_KEY") {
		t.Fatalf("err = %v, want missing key env error", err)
	}
}

func TestOpenAICompatibleClientSendsBearerToken(t *testing.T) {
	t.Setenv("TEST_LLM_KEY", "secret-test-key")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer secret-test-key" {
			t.Fatalf("Authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"intent\":\"recall_relevant_context\",\"entities\":[\"player\"],\"tags\":[],\"memory_types\":[],\"locations\":[],\"quest_ids\":[],\"keywords\":[\"gate\"],\"time_scope\":\"recent_or_important\",\"max_results\":4}"}}]}`))
	}))
	defer server.Close()

	client, err := NewOpenAICompatibleClient(OpenAICompatibleConfig{BaseURL: server.URL, APIKeyEnv: "TEST_LLM_KEY", Model: "test-model", TimeoutSeconds: 5})
	if err != nil {
		t.Fatalf("NewOpenAICompatibleClient returned error: %v", err)
	}

	plan, err := client.PlanRetrieval(context.Background(), PlannerRequest{PlayerAction: "ta quan sat", SceneID: "loc_outer_gate"})
	if err != nil {
		t.Fatalf("PlanRetrieval returned error: %v", err)
	}
	if plan.Intent != "recall_relevant_context" || len(plan.Keywords) != 1 || plan.Keywords[0] != "gate" {
		t.Fatalf("plan = %#v", plan)
	}
}
```

- [x] **Step 2: Run tests and verify failure**

Run: `go test ./internal/llm`

Expected: FAIL because `OpenAICompatibleConfig` and `NewOpenAICompatibleClient` do not exist.

- [x] **Step 3: Implement minimal client**

Create `internal/llm/openai_compatible.go` with:

```go
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type OpenAICompatibleConfig struct {
	BaseURL        string
	APIKeyEnv      string
	Model          string
	TimeoutSeconds int
	MaxRetries     int
}

type OpenAICompatibleClient struct {
	baseURL    string
	apiKey     string
	model      string
	maxRetries int
	httpClient *http.Client
}

func NewOpenAICompatibleClient(cfg OpenAICompatibleConfig) (*OpenAICompatibleClient, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, errors.New("llm base_url is required")
	}
	if strings.TrimSpace(cfg.APIKeyEnv) == "" {
		return nil, errors.New("llm api_key_env is required")
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return nil, errors.New("llm model is required")
	}
	apiKey := strings.TrimSpace(os.Getenv(cfg.APIKeyEnv))
	if apiKey == "" {
		return nil, fmt.Errorf("llm api key environment variable %s is not set", cfg.APIKeyEnv)
	}
	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 45 * time.Second
	}
	return &OpenAICompatibleClient{baseURL: strings.TrimRight(cfg.BaseURL, "/"), apiKey: apiKey, model: cfg.Model, maxRetries: cfg.MaxRetries, httpClient: &http.Client{Timeout: timeout}}, nil
}
```

Then add `PlanRetrieval`, `Narrate`, `ExtractMemories`, and a shared `chatJSON(ctx, systemPrompt, userPrompt string, out any) error` helper that POSTs to `<baseURL>/chat/completions` and unmarshals `choices[0].message.content` into `out`.

- [x] **Step 4: Add retry and parsing tests**

Add tests for `Narrate`, `ExtractMemories`, retry on HTTP 429 once, and invalid JSON returning an error. Use `httptest.NewServer` only.

- [x] **Step 5: Run package tests**

Run: `gofmt -w internal/llm/*.go && go test ./internal/llm`

Expected: PASS.

---

### Task 2: Online Session Construction And Default Config

**Files:**

- Modify: `cmd/tu-tien-cli/main.go`
- Modify: `cmd/tu-tien-cli/main_test.go`
- Create: `configs/llm.yaml`

**Interfaces:**

- Consumes: `config.Load(path string) (config.Config, error)`, `llm.NewOpenAICompatibleClient`, `memory.NewSQLiteStore`, `orchestrator.NewSession`.
- Produces: `buildSession(ctx context.Context, cfgPath string, name string) (*orchestrator.Session, func(), error)` and no `--offline` runtime path.

- [x] **Step 1: Write failing CLI construction tests**

Update `cmd/tu-tien-cli/main_test.go` so offline-specific tests are removed or renamed. Add tests that create a temporary config with an unset API key env and assert `buildSession` returns an error mentioning that env var.

- [x] **Step 2: Run tests and verify failure**

Run: `go test ./cmd/tu-tien-cli`

Expected: FAIL because `buildSession` does not exist and `buildOfflineSession` still exists.

- [x] **Step 3: Implement config-backed session construction**

Modify `cmd/tu-tien-cli/main.go`:

- Remove the `--offline` flag and early exit branch.
- Add `configPath := flag.String("config", "configs/llm.yaml", "runtime config path")`.
- Build a real LLM client from loaded config.
- Keep save and memory store setup as before, using `cfg.Storage.DataDir` when set and `./data/dev` as a fallback.

- [x] **Step 4: Add default runtime config**

Create `configs/llm.yaml`:

```yaml
llm:
  base_url: https://api.groq.com/openai/v1
  api_key_env: GROQ_API_KEY
  model: llama-3.1-70b-versatile
  timeout_seconds: 45
  max_retries: 2
storage:
  data_dir: ./data/dev
debug:
  log_llm_requests: false
  log_retrieval: true
```

- [x] **Step 5: Run focused tests**

Run: `gofmt -w cmd/tu-tien-cli/*.go && go test ./cmd/tu-tien-cli`

Expected: PASS without any real network calls.

---

### Task 3: Bubble Tea TUI Adapter

**Files:**

- Create: `cmd/tu-tien-cli/tui.go`
- Create: `cmd/tu-tien-cli/tui_test.go`
- Modify: `cmd/tu-tien-cli/main.go`
- Modify: `go.mod`
- Modify: `go.sum`

**Interfaces:**

- Consumes: `orchestrator.GameSession`, `orchestrator.PlayerInput`, `game.TurnResult`, helper semantics from existing `resolveSuggestedAction` and `commandForSuggestedAction`.
- Produces: `newTUIModel(session orchestrator.GameSession, providerLabel string) tuiModel` and `runTUI(ctx context.Context, session orchestrator.GameSession, providerLabel string) error`.

- [x] **Step 1: Add Bubble Tea dependencies**

Run: `go get github.com/charmbracelet/bubbletea@latest github.com/charmbracelet/lipgloss@latest`

- [x] **Step 2: Write TUI behavior tests**

Create `cmd/tu-tien-cli/tui_test.go` with fake session coverage:

- submitting free text calls `HandleTurn` once;
- submitting `1` maps to the latest suggested action;
- `/status` renders local state and does not call `HandleTurn`;
- a session error appends an error log entry.

- [x] **Step 3: Run tests and verify failure**

Run: `go test ./cmd/tu-tien-cli`

Expected: FAIL because `newTUIModel` and related types do not exist.

- [x] **Step 4: Implement Bubble Tea model**

Create `cmd/tu-tien-cli/tui.go` with:

- `type tuiModel struct` containing session, provider label, log entries, suggestions, input buffer, loading flag, width, height, and optional pending command result.
- `Init() tea.Cmd` returning nil.
- `Update(msg tea.Msg) (tea.Model, tea.Cmd)` handling key input, window size, submitted turns, and `/exit`.
- `View() string` rendering header, log, player panel, suggestions, input bar, and footer with Lip Gloss.
- `submitInput(text string) tea.Cmd` that calls `session.HandleTurn` in a command and returns a message.

- [x] **Step 5: Wire main to Bubble Tea**

Modify `main.go` so successful startup calls `runTUI(context.Background(), session, cfg.LLM.Model)` instead of `runInteractive`.

- [x] **Step 6: Run focused tests**

Run: `gofmt -w cmd/tu-tien-cli/*.go && go test ./cmd/tu-tien-cli`

Expected: PASS.

---

### Task 4: Documentation And Final Verification

**Files:**

- Modify: `README.md`
- Modify: `docs/cli-guide.md`
- Modify: `configs/example.yaml` if it should stay aligned with `configs/llm.yaml`.

**Interfaces:**

- Consumes: implemented runtime command `go run ./cmd/tu-tien-cli --name Nam` and config path `configs/llm.yaml`.
- Produces: accurate user-facing docs for online TUI play.

- [x] **Step 1: Update README**

Replace offline run instructions with:

```bash
export GROQ_API_KEY=your_key_here
go run ./cmd/tu-tien-cli --name Nam
```

Mention `--config configs/llm.yaml` only as an override path, not required for default use.

- [x] **Step 2: Update CLI guide**

Remove `--offline` documentation. Describe the Bubble Tea layout, commands, number selection, loading state, and API key requirement.

- [x] **Step 3: Run formatting and tests**

Run: `gofmt -w cmd/tu-tien-cli/*.go internal/llm/*.go && go test ./...`

Expected: PASS.

- [x] **Step 4: Manual startup check without secret leakage**

Run with `GROQ_API_KEY` unset:

```bash
env -u GROQ_API_KEY go run ./cmd/tu-tien-cli --name Nam
```

Expected: startup fails with a clear message naming `GROQ_API_KEY` and does not print any secret.

---

## Self-Review

- Spec coverage: online-only runtime is covered by Task 2; OpenAI-compatible client by Task 1; Bubble Tea TUI by Task 3; docs and verification by Task 4.
- Placeholder scan: no TBD/TODO placeholders remain.
- Type consistency: exported client/session/TUI names are introduced before use by later tasks.

## Verification Note (2026-08-04 audit)

All tasks re-audited against the current codebase and marked complete:

- `internal/llm/openai_compatible.go` implements `OpenAICompatibleConfig`/`NewOpenAICompatibleClient` with bearer auth, retry on 429/5xx, JSON/YAML decoding with prose/fence extraction, and now also `NarrateWithTools` for engine tool calls (an extension from the later narrator-yaml-output/friendly-tui-flow plans, additive and not conflicting with this plan's contract).
- `cmd/tu-tien-cli/main.go` has no `--offline` flag; `buildSession` loads `configs/llm.yaml` by default, fails clearly on a missing API key env var, and `--config` overrides the path.
- `configs/llm.yaml` exists as the default runtime config (left untouched: it currently has a user-local uncommitted edit pointing at a different local provider, which was preserved as instructed).
- `cmd/tu-tien-cli/tui.go` implements the Bubble Tea model (`newTUIModel`, `runTUI`) with header/history/summary/action rendering, numeric suggestion selection, and command routing; `tui_test.go` covers submission, numeric choice mapping, `/status`, and error-path behavior.
- README.md and docs/cli-guide.md both document the online-only run flow (`GROQ_API_KEY` + `go run ./cmd/tu-tien-cli --name Nam`) with no offline instructions remaining.
- Manual check re-run this session: `env -u GROQ_API_KEY -u CODEX_LB_API_KEY go run ./cmd/tu-tien-cli --name Nam` fails at startup naming the missing env var (`CODEX_LB_API_KEY`, since that's what the local uncommitted config currently points at) and prints no secret value.
- `gofmt -l` reports no files needing formatting, `go vet ./...` is clean, and `go test ./...` passes (61 tests across 7 packages) as of this audit.
