# Tutine TRPG MVP Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the first runnable Go foundation for Tutine TRPG: core state types, campaign config loading, SQLite FTS memory, LLM contracts, orchestrated turns with fake LLM support, and a CLI skeleton.

**Architecture:** The first implementation slice keeps CLI thin and puts reusable behavior behind `internal/orchestrator.Session`. `internal/game` owns authoritative state and validation, `internal/memory` owns SQLite FTS retrieval, and `internal/llm` owns OpenAI-compatible contracts behind interfaces so tests use fakes by default.

**Tech Stack:** Go 1.22+, SQLite via `modernc.org/sqlite`, YAML via `gopkg.in/yaml.v3`, standard `testing` package, OpenAI-compatible HTTP APIs planned behind an interface.

## Global Constraints

- Repository name is `tutine-trpg`.
- CLI is the first adapter, but the game core must remain reusable for web and bot adapters.
- Go is the selected implementation language.
- SQLite FTS is the MVP retrieval layer.
- LLM providers must use an OpenAI-compatible API boundary.
- The LLM never mutates game state directly.
- Combat outcomes are resolved by the engine before narration.
- Tests must not call real LLM APIs by default.

---

## File Structure

- `go.mod`: Go module declaration and dependencies.
- `cmd/tu-tien-cli/main.go`: CLI entrypoint and command loop.
- `internal/game/state.go`: Save, player, stats, quest, inventory, and turn result view types.
- `internal/game/effects.go`: Effect types, validation, and state mutation rules.
- `internal/config/config.go`: Runtime config structs and YAML loader.
- `internal/campaign/campaign.go`: Campaign data structs and loader.
- `internal/memory/memory.go`: Memory models and search query types.
- `internal/memory/sqlite_store.go`: SQLite schema, insert, FTS search, and reranking.
- `internal/llm/contracts.go`: Planner, narrator, extractor interfaces and DTOs.
- `internal/llm/fake.go`: Deterministic fake client for tests and offline CLI demo.
- `internal/orchestrator/session.go`: `GameSession` interface and turn orchestration.
- `campaigns/thanh-van-sect/*.yaml`: Minimal campaign pack.
- `configs/example.yaml`: Example runtime config.

---

### Task 1: Go Module And Core Game State

**Files:**

- Create: `go.mod`
- Create: `internal/game/state.go`
- Create: `internal/game/effects.go`
- Test: `internal/game/effects_test.go`

**Interfaces:**

- Produces: `game.SaveGame`, `game.Player`, `game.Stats`, `game.Effect`, `game.ApplyEffects(save *SaveGame, effects []Effect) ([]StateChangeView, error)`, `game.NewStarterSave(req NewGameRequest) SaveGame`.
- Consumes: No project code from earlier tasks.

- [x] **Step 1: Write the failing tests**

Create `internal/game/effects_test.go`:

```go
package game

import "testing"

func TestApplyEffectsRejectsUnknownItem(t *testing.T) {
	save := NewStarterSave(NewGameRequest{Name: "Nam", Traits: []string{"careful"}, CampaignID: "thanh-van-sect"})

	_, err := ApplyEffects(&save, []Effect{{Type: EffectGrantItem, TargetID: "player", ItemID: "heaven_sword", Amount: 1}})
	if err == nil {
		t.Fatal("expected unknown item to be rejected")
	}
	if len(save.Inventory) != 0 {
		t.Fatalf("inventory mutated after rejected effect: %#v", save.Inventory)
	}
}

func TestApplyEffectsClampsRelationshipDelta(t *testing.T) {
	save := NewStarterSave(NewGameRequest{Name: "Nam", Traits: []string{"careful"}, CampaignID: "thanh-van-sect"})

	changes, err := ApplyEffects(&save, []Effect{{Type: EffectRelationshipDelta, TargetID: "npc_luc_thanh_nghi", Amount: 99}})
	if err != nil {
		t.Fatalf("ApplyEffects returned error: %v", err)
	}
	if got := save.Player.Relationships["npc_luc_thanh_nghi"]; got != 3 {
		t.Fatalf("relationship = %d, want 3", got)
	}
	if len(changes) != 1 || changes[0].Amount != 3 {
		t.Fatalf("changes = %#v, want one clamped +3 change", changes)
	}
}
```

- [x] **Step 2: Run tests and verify they fail**

Run: `go test ./internal/game`

Expected: FAIL because `go.mod` and `internal/game` do not exist.

- [x] **Step 3: Create Go module**

Create `go.mod`:

```go
module github.com/namtt/tutine-trpg

go 1.22
```

- [x] **Step 4: Implement core state and effect validation**

Create `internal/game/state.go`:

```go
package game

import "time"

type NewGameRequest struct {
	Name       string
	Traits     []string
	CampaignID string
}

type SaveGame struct {
	SaveID       string            `json:"save_id"`
	CampaignID   string            `json:"campaign_id"`
	CurrentTurn  int               `json:"current_turn"`
	CurrentScene string            `json:"current_scene"`
	Player       Player            `json:"player"`
	Inventory    map[string]int    `json:"inventory"`
	WorldFlags   map[string]bool   `json:"world_flags"`
	Cooldowns    map[string]int    `json:"cooldowns"`
	CreatedAt    time.Time         `json:"created_at"`
}

type Player struct {
	ID              string         `json:"id"`
	Name            string         `json:"name"`
	Traits          []string       `json:"traits"`
	Realm           string         `json:"realm"`
	Stage           int            `json:"stage"`
	HP              int            `json:"hp"`
	MaxHP           int            `json:"max_hp"`
	SpiritualEnergy int            `json:"spiritual_energy"`
	MaxEnergy       int            `json:"max_spiritual_energy"`
	Stats           Stats          `json:"stats"`
	Techniques      []string       `json:"techniques"`
	Artifacts       []string       `json:"artifacts"`
	Relationships   map[string]int `json:"relationships"`
}

type Stats struct {
	Attack        int `json:"attack"`
	Defense       int `json:"defense"`
	Speed         int `json:"speed"`
	Comprehension int `json:"comprehension"`
	Luck          int `json:"luck"`
}

type StateChangeView struct {
	Type     string `json:"type"`
	TargetID string `json:"target_id"`
	Amount   int    `json:"amount"`
	Message  string `json:"message"`
}

type TurnResult struct {
	Narration        string            `json:"narration"`
	StateChanges     []StateChangeView `json:"state_changes"`
	SuggestedActions []string          `json:"suggested_actions"`
	Warnings         []string          `json:"warnings"`
	NeedsInput       *InputRequest     `json:"needs_input,omitempty"`
}

type InputRequest struct {
	Prompt  string   `json:"prompt"`
	Options []string `json:"options"`
}

func NewStarterSave(req NewGameRequest) SaveGame {
	name := req.Name
	if name == "" {
		name = "Vo Danh"
	}
	campaignID := req.CampaignID
	if campaignID == "" {
		campaignID = "thanh-van-sect"
	}
	return SaveGame{
		SaveID:       "local-dev",
		CampaignID:   campaignID,
		CurrentTurn:  0,
		CurrentScene: "loc_outer_gate",
		Inventory:    map[string]int{},
		WorldFlags:   map[string]bool{},
		Cooldowns:    map[string]int{},
		CreatedAt:    time.Now().UTC(),
		Player: Player{
			ID:              "player",
			Name:            name,
			Traits:          append([]string{}, req.Traits...),
			Realm:           "qi_refining",
			Stage:           1,
			HP:              30,
			MaxHP:           30,
			SpiritualEnergy: 20,
			MaxEnergy:       20,
			Stats:           Stats{Attack: 6, Defense: 4, Speed: 5, Comprehension: 5, Luck: 5},
			Techniques:      []string{"basic_strike"},
			Artifacts:       []string{},
			Relationships:   map[string]int{},
		},
	}
}
```

Create `internal/game/effects.go`:

```go
package game

import (
	"errors"
	"fmt"
)

const (
	EffectGrantItem         = "grant_item"
	EffectRelationshipDelta = "relationship_delta"
	EffectEnergyDelta       = "energy_delta"
)

var starterAllowedItems = map[string]bool{
	"moonlit_grass": true,
	"low_spirit_stone": true,
}

type Effect struct {
	Type     string `json:"type"`
	TargetID string `json:"target_id"`
	ItemID   string `json:"item_id,omitempty"`
	Amount   int    `json:"amount"`
	Reason   string `json:"reason,omitempty"`
}

func ApplyEffects(save *SaveGame, effects []Effect) ([]StateChangeView, error) {
	if save == nil {
		return nil, errors.New("save is nil")
	}
	changes := make([]StateChangeView, 0, len(effects))
	for _, effect := range effects {
		switch effect.Type {
		case EffectGrantItem:
			if !starterAllowedItems[effect.ItemID] {
				return nil, fmt.Errorf("unknown or disallowed item %q", effect.ItemID)
			}
			if effect.Amount <= 0 || effect.Amount > 3 {
				return nil, fmt.Errorf("invalid item amount %d", effect.Amount)
			}
			save.Inventory[effect.ItemID] += effect.Amount
			changes = append(changes, StateChangeView{Type: effect.Type, TargetID: effect.ItemID, Amount: effect.Amount, Message: "nhan vat pham"})
		case EffectRelationshipDelta:
			amount := clamp(effect.Amount, -3, 3)
			save.Player.Relationships[effect.TargetID] += amount
			changes = append(changes, StateChangeView{Type: effect.Type, TargetID: effect.TargetID, Amount: amount, Message: "quan he thay doi"})
		case EffectEnergyDelta:
			amount := clamp(effect.Amount, -save.Player.MaxEnergy, save.Player.MaxEnergy)
			save.Player.SpiritualEnergy = clamp(save.Player.SpiritualEnergy+amount, 0, save.Player.MaxEnergy)
			changes = append(changes, StateChangeView{Type: effect.Type, TargetID: "player", Amount: amount, Message: "linh luc thay doi"})
		default:
			return nil, fmt.Errorf("unknown effect type %q", effect.Type)
		}
	}
	return changes, nil
}

func clamp(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
```

- [x] **Step 5: Run tests and commit**

Run: `gofmt -w internal/game/*.go && go test ./internal/game`

Expected: PASS.

Commit:

```bash
git add go.mod internal/game/state.go internal/game/effects.go internal/game/effects_test.go
git commit -m "feat: add core game state and effects"
```

---

### Task 2: Campaign And Runtime Config Loading

**Files:**

- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`
- Create: `internal/campaign/campaign.go`
- Create: `internal/campaign/campaign_test.go`
- Create: `configs/example.yaml`
- Create: `campaigns/thanh-van-sect/campaign.yaml`
- Create: `campaigns/thanh-van-sect/tags.yaml`

**Interfaces:**

- Consumes: No runtime dependency on Task 1.
- Produces: `config.Load(path string) (Config, error)`, `campaign.Load(dir string) (Campaign, error)`, and campaign tag vocabulary for LLM validation.

- [x] **Step 1: Write failing config tests**

Create `internal/config/config_test.go`:

```go
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
```

Create `internal/campaign/campaign_test.go`:

```go
package campaign

import "testing"

func TestLoadCampaign(t *testing.T) {
	camp, err := Load("../../campaigns/thanh-van-sect")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if camp.ID != "thanh-van-sect" {
		t.Fatalf("campaign ID = %q", camp.ID)
	}
	if !camp.HasTag("trust") || camp.HasTag("invented_tag") {
		t.Fatalf("tag vocabulary not loaded correctly: %#v", camp.Tags)
	}
}
```

- [x] **Step 2: Run tests and verify they fail**

Run: `go test ./internal/config ./internal/campaign`

Expected: FAIL because packages do not exist.

- [x] **Step 3: Add YAML dependency**

Run: `go get gopkg.in/yaml.v3`

Expected: `go.mod` and `go.sum` include `gopkg.in/yaml.v3`.

- [x] **Step 4: Implement config loader**

Create `internal/config/config.go`:

```go
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
```

- [x] **Step 5: Implement campaign loader and seed data**

Create `internal/campaign/campaign.go`:

```go
package campaign

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Campaign struct {
	ID            string              `yaml:"id"`
	Name          string              `yaml:"name"`
	StartingScene string              `yaml:"starting_scene"`
	StartingRealm string              `yaml:"starting_realm"`
	StartingStage int                 `yaml:"starting_stage"`
	Tags          map[string][]string `yaml:"-"`
	tagSet        map[string]bool
}

func Load(dir string) (Campaign, error) {
	campaignData, err := os.ReadFile(filepath.Join(dir, "campaign.yaml"))
	if err != nil {
		return Campaign{}, err
	}
	var camp Campaign
	if err := yaml.Unmarshal(campaignData, &camp); err != nil {
		return Campaign{}, err
	}
	tagData, err := os.ReadFile(filepath.Join(dir, "tags.yaml"))
	if err != nil {
		return Campaign{}, err
	}
	var tags map[string][]string
	if err := yaml.Unmarshal(tagData, &tags); err != nil {
		return Campaign{}, err
	}
	camp.Tags = tags
	camp.tagSet = map[string]bool{}
	for _, values := range tags {
		for _, tag := range values {
			camp.tagSet[tag] = true
		}
	}
	return camp, nil
}

func (c Campaign) HasTag(tag string) bool {
	return c.tagSet[tag]
}
```

Create `campaigns/thanh-van-sect/campaign.yaml`:

```yaml
id: thanh-van-sect
name: Thanh Van Tong
starting_scene: loc_outer_gate
starting_realm: qi_refining
starting_stage: 1
```

Create `campaigns/thanh-van-sect/tags.yaml`:

```yaml
social:
  - trust
  - grudge
  - favor
  - betrayal
  - secret
cultivation:
  - breakthrough
  - bottleneck
  - spiritual_root
  - mutated_spiritual_root
faction:
  - sect_politics
  - outer_disciple_conflict
  - demonic_path
```

Create `configs/example.yaml`:

```yaml
llm:
  base_url: https://api.groq.com/openai/v1
  api_key_env: GROQ_API_KEY
  model: llama-3.1-70b-versatile
  timeout_seconds: 45
  max_retries: 2
storage:
  data_dir: ./data
debug:
  log_llm_requests: false
  log_retrieval: true
```

- [x] **Step 6: Run tests and commit**

Run: `gofmt -w internal/config/*.go internal/campaign/*.go && go test ./internal/config ./internal/campaign`

Expected: PASS.

Commit:

```bash
git add go.mod go.sum internal/config internal/campaign configs campaigns
git commit -m "feat: add config and campaign loading"
```

---

### Task 3: SQLite FTS Memory Store

**Files:**

- Create: `internal/memory/memory.go`
- Create: `internal/memory/sqlite_store.go`
- Test: `internal/memory/sqlite_store_test.go`

**Interfaces:**

- Produces: `memory.Store`, `memory.Memory`, `memory.Query`, `memory.Hit`, `memory.NewSQLiteStore(ctx context.Context, path string) (*SQLiteStore, error)`.
- Consumes: No project code from earlier tasks.

- [x] **Step 1: Write failing memory tests**

Create `internal/memory/sqlite_store_test.go`:

```go
package memory

import (
	"context"
	"path/filepath"
	"testing"
)

func TestSQLiteStoreSearchesByTagEntityAndFTS(t *testing.T) {
	ctx := context.Background()
	store, err := NewSQLiteStore(ctx, filepath.Join(t.TempDir(), "game.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	err = store.Add(ctx, Memory{ID: "mem_1", SaveID: "save_1", CampaignID: "thanh-van-sect", Turn: 7, Type: "npc_event", Importance: 4, Text: "Luc Thanh Nghi biet bi mat linh can bien di.", Summary: "Luc Thanh Nghi knows the secret.", Entities: []string{"npc_luc_thanh_nghi", "player"}, Tags: []string{"secret", "trust"}})
	if err != nil {
		t.Fatal(err)
	}

	hits, err := store.Search(ctx, Query{SaveID: "save_1", Entities: []string{"npc_luc_thanh_nghi"}, Tags: []string{"secret"}, Keywords: []string{"linh can"}, MaxResults: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Memory.ID != "mem_1" {
		t.Fatalf("hits = %#v, want mem_1", hits)
	}
}
```

- [x] **Step 2: Run tests and verify they fail**

Run: `go test ./internal/memory`

Expected: FAIL because memory package does not exist.

- [x] **Step 3: Add SQLite dependency**

Run: `go get modernc.org/sqlite`

Expected: `go.mod` and `go.sum` include `modernc.org/sqlite`.

- [x] **Step 4: Implement memory models**

Create `internal/memory/memory.go`:

```go
package memory

import "context"

type Memory struct {
	ID         string
	SaveID     string
	CampaignID string
	Turn       int
	Type       string
	Scope      string
	Importance int
	Text       string
	Summary    string
	Entities   []string
	Tags       []string
	FactsJSON  string
	LocationID string
	QuestID    string
	NPCID      string
}

type Query struct {
	SaveID      string
	Entities    []string
	Tags        []string
	Types       []string
	Locations   []string
	QuestIDs    []string
	Keywords    []string
	MaxResults  int
}

type Hit struct {
	Memory Memory
	Score  float64
}

type Store interface {
	Add(ctx context.Context, memory Memory) error
	Search(ctx context.Context, query Query) ([]Hit, error)
	Close() error
}
```

- [x] **Step 5: Implement SQLite store**

Create `internal/memory/sqlite_store.go` with schema using `memories` and `memory_fts`, JSON-encoded tags/entities, and a simple score of `entityMatches*100 + tagMatches*50 + importance*5 + turn*0.01`.

Required function signatures:

```go
func NewSQLiteStore(ctx context.Context, path string) (*SQLiteStore, error)
func (s *SQLiteStore) Add(ctx context.Context, memory Memory) error
func (s *SQLiteStore) Search(ctx context.Context, query Query) ([]Hit, error)
func (s *SQLiteStore) Close() error
```

Use `database/sql`, import `_ "modernc.org/sqlite"`, create the FTS5 table, and query candidates with SQL filters plus in-Go reranking for tag/entity overlap.

- [x] **Step 6: Run tests and commit**

Run: `gofmt -w internal/memory/*.go && go test ./internal/memory`

Expected: PASS.

Commit:

```bash
git add go.mod go.sum internal/memory
git commit -m "feat: add sqlite fts memory store"
```

---

### Task 4: LLM Contracts And Fake Client

**Files:**

- Create: `internal/llm/contracts.go`
- Create: `internal/llm/fake.go`
- Test: `internal/llm/fake_test.go`

**Interfaces:**

- Produces: `llm.Client`, `llm.RetrievalPlan`, `llm.NarratorResponse`, `llm.MemoryDraft`, deterministic `llm.FakeClient`.
- Consumes: `game.Effect` from Task 1.

- [x] **Step 1: Write failing fake LLM test**

Create `internal/llm/fake_test.go`:

```go
package llm

import (
	"context"
	"testing"
)

func TestFakeClientReturnsDeterministicNarration(t *testing.T) {
	client := FakeClient{}
	resp, err := client.Narrate(context.Background(), NarratorRequest{PlayerAction: "ta quan sat cong mon"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Narration == "" || len(resp.SuggestedNextOptions) == 0 {
		t.Fatalf("unexpected response: %#v", resp)
	}
}
```

- [x] **Step 2: Run tests and verify they fail**

Run: `go test ./internal/llm`

Expected: FAIL because llm package does not exist.

- [x] **Step 3: Implement contracts**

Create `internal/llm/contracts.go`:

```go
package llm

import (
	"context"

	"github.com/namtt/tutine-trpg/internal/game"
)

type Client interface {
	PlanRetrieval(ctx context.Context, req PlannerRequest) (RetrievalPlan, error)
	Narrate(ctx context.Context, req NarratorRequest) (NarratorResponse, error)
	ExtractMemories(ctx context.Context, req ExtractorRequest) ([]MemoryDraft, error)
}

type PlannerRequest struct {
	PlayerAction string
	SceneID      string
	AllowedTags  []string
	NearbyIDs    []string
}

type RetrievalPlan struct {
	Intent      string
	Entities    []string
	Tags        []string
	MemoryTypes []string
	Locations   []string
	QuestIDs    []string
	Keywords    []string
	TimeScope   string
	MaxResults  int
}

type NarratorRequest struct {
	PlayerAction     string
	SceneID          string
	StateSummary     string
	RetrievedContext []string
	AllowedEffects   []string
}

type DialogueLine struct {
	NPCID string `json:"npc_id"`
	Text  string `json:"text"`
}

type NarratorResponse struct {
	Narration            string
	NPCDialogue          []DialogueLine
	ProposedEffects      []game.Effect
	SuggestedNextOptions []string
}

type ExtractorRequest struct {
	PlayerAction    string
	Narration       string
	ResolvedChanges []game.StateChangeView
	AllowedTags     []string
}

type MemoryDraft struct {
	Type       string
	Importance int
	Entities   []string
	Tags       []string
	FactsJSON  string
	Text       string
}
```

- [x] **Step 4: Implement fake client**

Create `internal/llm/fake.go`:

```go
package llm

import (
	"context"

	"github.com/namtt/tutine-trpg/internal/game"
)

type FakeClient struct{}

func (FakeClient) PlanRetrieval(ctx context.Context, req PlannerRequest) (RetrievalPlan, error) {
	return RetrievalPlan{Intent: "recall_relevant_context", Entities: req.NearbyIDs, Tags: firstTags(req.AllowedTags, 3), Keywords: []string{req.PlayerAction}, TimeScope: "recent_or_important", MaxResults: 5}, nil
}

func (FakeClient) Narrate(ctx context.Context, req NarratorRequest) (NarratorResponse, error) {
	return NarratorResponse{Narration: "Gio nui thoi qua cong mon Thanh Van Tong khi ban can nhac hanh dong tiep theo.", ProposedEffects: []game.Effect{{Type: game.EffectEnergyDelta, TargetID: "player", Amount: 0}}, SuggestedNextOptions: []string{"Quan sat xung quanh", "Hoi de tu gac cong", "Kiem tra trang thai"}}, nil
}

func (FakeClient) ExtractMemories(ctx context.Context, req ExtractorRequest) ([]MemoryDraft, error) {
	return []MemoryDraft{{Type: "turn_summary", Importance: 1, Entities: []string{"player"}, Tags: firstTags(req.AllowedTags, 1), Text: req.PlayerAction}}, nil
}

func firstTags(tags []string, n int) []string {
	if len(tags) < n {
		n = len(tags)
	}
	return append([]string{}, tags[:n]...)
}
```

- [x] **Step 5: Run tests and commit**

Run: `gofmt -w internal/llm/*.go && go test ./internal/llm`

Expected: PASS.

Commit:

```bash
git add internal/llm
git commit -m "feat: add llm contracts and fake client"
```

---

### Task 5: Orchestrator Session With Fake LLM

**Files:**

- Create: `internal/orchestrator/session.go`
- Test: `internal/orchestrator/session_test.go`

**Interfaces:**

- Consumes: `game.NewStarterSave`, `game.ApplyEffects`, `llm.Client`, `memory.Store`.
- Produces: `orchestrator.GameSession`, `orchestrator.Session`, `orchestrator.NewSession(save game.SaveGame, llm llm.Client, memories memory.Store, allowedTags []string) *Session`.

- [x] **Step 1: Write failing orchestrator test**

Create `internal/orchestrator/session_test.go`:

```go
package orchestrator

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/namtt/tutine-trpg/internal/game"
	"github.com/namtt/tutine-trpg/internal/llm"
	"github.com/namtt/tutine-trpg/internal/memory"
)

func TestHandleTurnReturnsNarrationAndAdvancesTurn(t *testing.T) {
	ctx := context.Background()
	store, err := memory.NewSQLiteStore(ctx, filepath.Join(t.TempDir(), "game.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	save := game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})
	session := NewSession(save, llm.FakeClient{}, store, []string{"trust", "secret"})

	result, err := session.HandleTurn(ctx, PlayerInput{Text: "ta quan sat cong mon"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Narration == "" {
		t.Fatal("expected narration")
	}
	if session.Save().CurrentTurn != 1 {
		t.Fatalf("turn = %d, want 1", session.Save().CurrentTurn)
	}
}
```

- [x] **Step 2: Run test and verify it fails**

Run: `go test ./internal/orchestrator`

Expected: FAIL because orchestrator package does not exist.

- [x] **Step 3: Implement session orchestration**

Create `internal/orchestrator/session.go`:

```go
package orchestrator

import (
	"context"
	"fmt"

	"github.com/namtt/tutine-trpg/internal/game"
	"github.com/namtt/tutine-trpg/internal/llm"
	"github.com/namtt/tutine-trpg/internal/memory"
)

type PlayerInput struct {
	Text string `json:"text"`
}

type GameSession interface {
	HandleTurn(ctx context.Context, input PlayerInput) (*game.TurnResult, error)
	Save() game.SaveGame
}

type Session struct {
	save        game.SaveGame
	client      llm.Client
	memories    memory.Store
	allowedTags []string
}

func NewSession(save game.SaveGame, client llm.Client, memories memory.Store, allowedTags []string) *Session {
	return &Session{save: save, client: client, memories: memories, allowedTags: append([]string{}, allowedTags...)}
}

func (s *Session) Save() game.SaveGame {
	return s.save
}

func (s *Session) HandleTurn(ctx context.Context, input PlayerInput) (*game.TurnResult, error) {
	plan, err := s.client.PlanRetrieval(ctx, llm.PlannerRequest{PlayerAction: input.Text, SceneID: s.save.CurrentScene, AllowedTags: s.allowedTags, NearbyIDs: []string{"player"}})
	if err != nil {
		return nil, err
	}

	hits, err := s.memories.Search(ctx, memory.Query{SaveID: s.save.SaveID, Entities: plan.Entities, Tags: plan.Tags, Keywords: plan.Keywords, MaxResults: plan.MaxResults})
	if err != nil {
		return nil, err
	}
	contextLines := make([]string, 0, len(hits))
	for _, hit := range hits {
		contextLines = append(contextLines, hit.Memory.Summary)
	}

	narration, err := s.client.Narrate(ctx, llm.NarratorRequest{PlayerAction: input.Text, SceneID: s.save.CurrentScene, StateSummary: fmt.Sprintf("%s %s stage %d", s.save.Player.Name, s.save.Player.Realm, s.save.Player.Stage), RetrievedContext: contextLines, AllowedEffects: []string{game.EffectEnergyDelta, game.EffectRelationshipDelta, game.EffectGrantItem}})
	if err != nil {
		return nil, err
	}

	changes, err := game.ApplyEffects(&s.save, narration.ProposedEffects)
	if err != nil {
		return nil, err
	}
	s.save.CurrentTurn++

	drafts, err := s.client.ExtractMemories(ctx, llm.ExtractorRequest{PlayerAction: input.Text, Narration: narration.Narration, ResolvedChanges: changes, AllowedTags: s.allowedTags})
	if err != nil {
		return nil, err
	}
	for i, draft := range drafts {
		_ = s.memories.Add(ctx, memory.Memory{ID: fmt.Sprintf("turn_%d_%d", s.save.CurrentTurn, i), SaveID: s.save.SaveID, CampaignID: s.save.CampaignID, Turn: s.save.CurrentTurn, Type: draft.Type, Importance: draft.Importance, Text: draft.Text, Summary: draft.Text, Entities: draft.Entities, Tags: filterTags(draft.Tags, s.allowedTags), FactsJSON: draft.FactsJSON})
	}

	return &game.TurnResult{Narration: narration.Narration, StateChanges: changes, SuggestedActions: narration.SuggestedNextOptions}, nil
}

func filterTags(tags []string, allowed []string) []string {
	allowedSet := map[string]bool{}
	for _, tag := range allowed {
		allowedSet[tag] = true
	}
	out := []string{}
	for _, tag := range tags {
		if allowedSet[tag] {
			out = append(out, tag)
		}
	}
	return out
}
```

- [x] **Step 4: Run tests and commit**

Run: `gofmt -w internal/orchestrator/*.go && go test ./internal/orchestrator ./internal/...`

Expected: PASS.

Commit:

```bash
git add internal/orchestrator
git commit -m "feat: add orchestrated turn session"
```

---

### Task 6: CLI Skeleton And README Update

**Files:**

- Create: `cmd/tu-tien-cli/main.go`
- Modify: `README.md`
- Test: `cmd/tu-tien-cli/main_test.go`

**Interfaces:**

- Consumes: `orchestrator.NewSession`, `game.NewStarterSave`, `llm.FakeClient`, `memory.NewSQLiteStore`.
- Produces: Runnable `go run ./cmd/tu-tien-cli --offline` demo with fake LLM.

- [x] **Step 1: Write failing CLI smoke test**

Create `cmd/tu-tien-cli/main_test.go`:

```go
package main

import "testing"

func TestBuildOfflineSession(t *testing.T) {
	session, cleanup, err := buildOfflineSession(t.TempDir(), "Nam")
	if err != nil {
		t.Fatalf("buildOfflineSession returned error: %v", err)
	}
	defer cleanup()
	if session.Save().Player.Name != "Nam" {
		t.Fatalf("player name = %q, want Nam", session.Save().Player.Name)
	}
}
```

- [x] **Step 2: Run test and verify it fails**

Run: `go test ./cmd/tu-tien-cli`

Expected: FAIL because CLI package does not exist.

- [x] **Step 3: Implement CLI offline mode**

Create `cmd/tu-tien-cli/main.go`:

```go
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/namtt/tutine-trpg/internal/game"
	"github.com/namtt/tutine-trpg/internal/llm"
	"github.com/namtt/tutine-trpg/internal/memory"
	"github.com/namtt/tutine-trpg/internal/orchestrator"
)

func main() {
	offline := flag.Bool("offline", true, "run with fake LLM client")
	name := flag.String("name", "Vo Danh", "player name")
	dataDir := flag.String("data-dir", "./data/dev", "data directory")
	flag.Parse()
	if !*offline {
		fmt.Fprintln(os.Stderr, "online provider is not wired in this foundation build")
		os.Exit(2)
	}

	session, cleanup, err := buildOfflineSession(*dataDir, *name)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer cleanup()

	fmt.Println("Tutine TRPG")
	fmt.Println("Nhap /exit de thoat, /status de xem nhan vat.")
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			continue
		}
		if text == "/exit" {
			break
		}
		if text == "/status" {
			save := session.Save()
			fmt.Printf("%s - %s tang %d | HP %d/%d | Linh luc %d/%d\n", save.Player.Name, save.Player.Realm, save.Player.Stage, save.Player.HP, save.Player.MaxHP, save.Player.SpiritualEnergy, save.Player.MaxEnergy)
			continue
		}
		result, err := session.HandleTurn(context.Background(), orchestrator.PlayerInput{Text: text})
		if err != nil {
			fmt.Println("Loi:", err)
			continue
		}
		fmt.Println(result.Narration)
		for _, change := range result.StateChanges {
			fmt.Printf("- %s: %+d\n", change.Type, change.Amount)
		}
		for i, option := range result.SuggestedActions {
			fmt.Printf("%d. %s\n", i+1, option)
		}
	}
}

func buildOfflineSession(dataDir string, name string) (*orchestrator.Session, func(), error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, nil, err
	}
	store, err := memory.NewSQLiteStore(context.Background(), filepath.Join(dataDir, "game.db"))
	if err != nil {
		return nil, nil, err
	}
	save := game.NewStarterSave(game.NewGameRequest{Name: name, CampaignID: "thanh-van-sect", Traits: []string{"careful"}})
	session := orchestrator.NewSession(save, llm.FakeClient{}, store, []string{"trust", "secret", "sect_politics"})
	return session, func() { _ = store.Close() }, nil
}
```

- [x] **Step 4: Update README status and run instructions**

Modify `README.md` status section to say the foundation CLI can run offline after this task. Add a `## Chạy Thử Offline` section with this command:

```bash
go run ./cmd/tu-tien-cli --offline --name Nam
```

Then add this sentence below the command: `Offline mode dùng fake LLM client nên không cần API key. Online provider sẽ được nối ở các task sau.`

- [x] **Step 5: Run tests, smoke test CLI, and commit**

Run:

```bash
gofmt -w cmd/tu-tien-cli/*.go
go test ./...
printf '/status\nta quan sat cong mon\n/exit\n' | go run ./cmd/tu-tien-cli --offline --name Nam --data-dir ./data/test-smoke
```

Expected: tests pass and CLI prints status plus fake narration.

Commit:

```bash
git add cmd README.md
git commit -m "feat: add offline cli skeleton"
```

---

## Verification Note (2026-08-04 audit)

All tasks in this plan were re-audited against the current codebase and marked complete:

- `go.mod`, `internal/game`, `internal/config`, `internal/campaign`, `internal/memory`, `internal/llm`, `internal/orchestrator`, and `cmd/tu-tien-cli` all exist with the interfaces this plan specifies (state has grown a `Clone()` method and the LLM/orchestrator layers gained tool-calling and richer narrator state for the later online/TUI plans, but the Task 1-5 contracts described here are all present).
- The Task 6 offline CLI (`--offline`, `buildOfflineSession`) was superseded by the online plan's `buildSession`/`runTUI`; there is no remaining offline-only code path, which is expected once `docs/superpowers/plans/2026-08-03-online-llm-bubbletea-tui.md` Task 2 replaced it.
- `gofmt -l` reports no files needing formatting, `go vet ./...` is clean, and `go test ./...` passes (61 tests across 7 packages) as of this audit.
