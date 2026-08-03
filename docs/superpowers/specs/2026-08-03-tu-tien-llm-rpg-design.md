# Tu Tien LLM RPG MVP Design

## Goal

Build a text-based xianxia RPG where the game rules are authoritative and the LLM acts as narrator, NPC actor, retrieval planner, and memory extractor. The first UI is a CLI, but the core must be reusable by future web and bot adapters.

## Product Scope

The MVP is a hybrid RPG:

- The player can choose a name and light personality/play-style traits.
- The starting campaign is fixed: a new low-rank cultivator entering a sect.
- The main arc covers joining the sect, early cultivation, first sect missions, a small secret realm, NPC conflict, and early breakthrough.
- Combat is medium-depth: HP, spiritual energy, basic stats, techniques, artifacts, enemy stats, and short turn/exchange resolution.
- Campaigns are data packs so later versions can add other backgrounds such as rogue cultivator, demonic path, clan cultivator, immortal world, urban cultivation, or survival secret realm.

## Architecture

```txt
cmd/tu-tien-cli
  CLI adapter only: read input, render results, expose debug commands.

internal/orchestrator
  Coordinates turn flow, retrieval, LLM calls, rule validation, saving, and memory extraction.

internal/game
  Source of truth for rules and state: player, NPCs, quests, inventory, cultivation, combat, rewards.

internal/memory
  SQLite FTS, metadata filters, controlled tags, retrieval plans, reranking.

internal/llm
  OpenAI-compatible client, JSON/text calls, retries, timeout, schema validation.

internal/storage
  Save/load, event log, SQLite database lifecycle.

internal/config
  Runtime config, provider config, campaign config.

campaigns/<campaign-id>
  Data pack: lore, tags, realms, techniques, items, NPCs, locations, quests.
```

`internal/game` must not know about CLI, web, bot, or any specific LLM provider. The LLM must never mutate state directly.

## Turn Flow

Exploration and roleplay turns:

```txt
player input
 -> retrieval planner builds a RetrievalPlan from current state and action
 -> memory store retrieves relevant lore/memories using entities, tags, FTS, recency, and importance
 -> orchestrator builds compact prompt context
 -> narrator LLM returns narration, dialogue, suggested options, and proposed effects
 -> game engine validates, rejects, clamps, or applies effects
 -> memory extractor creates durable memories/facts/tags from resolved turn
 -> storage saves state, event log, and accepted memories
 -> adapter renders TurnResult
```

Combat turns:

```txt
player input
 -> action parser identifies technique, target, and intent
 -> game engine validates cost/target and resolves hit, damage, enemy action, status changes
 -> narrator LLM describes the resolved exchange
 -> memory extractor stores only important combat events
```

Combat resolves before narration so the LLM cannot invent outcomes.

## Core State

`SaveGame` is the current source of truth:

- `save_id`
- `campaign_id`
- `current_turn`
- `current_scene`
- `player`
- `active_npcs`
- `active_quests`
- `world_flags`
- `inventory`
- `cooldowns`

`Player` includes:

- `id`
- `name`
- `traits`
- `realm`
- `stage`
- `hp`, `max_hp`
- `spiritual_energy`, `max_spiritual_energy`
- `stats`: attack, defense, speed, comprehension, luck
- `techniques`
- `artifacts`
- `relationships`

Facts that affect gameplay must live in structured state or structured memory, not only in prose.

## Event Log

The event log is append-only and used for audit, debugging, and possible replay.

```json
{
  "turn": 42,
  "type": "combat_result",
  "player_action": "ta dùng Hỏa Cầu Thuật đánh tên ma tu",
  "resolved_effects": [
    {"type": "damage", "target": "npc_blood_disciple_01", "amount": 18},
    {"type": "energy_cost", "target": "player", "amount": 7}
  ],
  "narration": "Hỏa cầu bùng lên, ép tên ma tu lùi nửa bước trước khi hắn kịp phản kích.",
  "created_at": "2026-08-03T10:15:00Z"
}
```

The event log is the full history. Memory entries are compact, queryable summaries.

## Memory And Retrieval

The MVP uses SQLite FTS plus structured metadata. This is intentionally designed as a "semantic cheat layer" before adding vector search.

Primary memory fields:

- `id`
- `save_id`
- `campaign_id`
- `turn`
- `type`
- `scope`
- `importance`
- `text`
- `summary`
- `entities_json`
- `tags_json`
- `facts_json`
- `location_id`
- `quest_id`
- `npc_id`
- `created_at`

FTS indexes:

- `text`
- `summary`
- `entities_text`
- `tags_text`

Memory example:

```json
{
  "type": "npc_event",
  "importance": 4,
  "entities": ["player", "npc_luc_thanh_nghi", "faction_thanh_van_tong"],
  "tags": ["secret", "trust", "sect_politics", "mutated_spiritual_root"],
  "facts": [
    {
      "subject": "npc_luc_thanh_nghi",
      "predicate": "knows_secret",
      "object": "player_mutated_spiritual_root"
    }
  ],
  "text": "Lục Thanh Nghi biết người chơi có linh căn biến dị và yêu cầu giữ bí mật."
}
```

Retrieval plan:

```json
{
  "intent": "recall_relevant_context",
  "entities": ["player", "npc_luc_thanh_nghi"],
  "tags": ["secret", "trust", "sect_politics"],
  "memory_types": ["npc_event", "quest_event", "lore"],
  "locations": ["loc_outer_sect"],
  "quest_ids": ["quest_join_sect"],
  "keywords": ["linh căn biến dị", "giữ bí mật"],
  "time_scope": "recent_or_important",
  "max_results": 8
}
```

Reranking order:

1. Exact entity matches.
2. Exact tag matches.
3. FTS score.
4. Importance.
5. Recency.

The campaign owns a controlled tag vocabulary. LLMs may propose tags, but the engine only accepts known tags. Unknown tags are mapped to a known tag if possible or dropped.

## Campaign Data

Campaigns are data packs:

```txt
campaigns/thanh-van-sect/
  campaign.yaml
  tags.yaml
  realms.yaml
  techniques.yaml
  items.yaml
  npcs.yaml
  locations.yaml
  quests.yaml
  lore/*.md
```

Example tag categories:

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

## LLM Contracts

The provider layer is OpenAI-compatible:

```yaml
llm:
  base_url: "https://api.groq.com/openai/v1"
  api_key_env: "GROQ_API_KEY"
  model: "llama-3.1-70b-versatile"
  timeout_seconds: 45
  max_retries: 2
  temperature:
    planner: 0.1
    narrator: 0.8
    extractor: 0.1
```

The system uses three main call types:

1. `RetrievalPlanner`: JSON output. Converts current state and player action into entities, tags, memory types, locations, quest IDs, keywords, time scope, and result limit.
2. `Narrator`: JSON output with narration, NPC dialogue, suggested next options, and proposed effects. Proposed effects are not applied directly.
3. `MemoryExtractor`: JSON output. Extracts important memories, facts, entities, and tags from the resolved turn.

Guardrails:

- Planner and extractor use strict JSON and low temperature.
- Invalid JSON triggers a repair/retry path, then a deterministic fallback.
- Unknown tags, entities, effect types, and fact predicates are rejected or mapped by rules.
- Narrator may only use facts from structured state, retrieved memories, and provided campaign lore.
- Rewards, quest completion, inventory changes, relationship changes, combat outcomes, and realm changes must be validated by `internal/game`.

## Rule Engine

### Cultivation

Realms are campaign data:

```yaml
realms:
  - id: qi_refining
    name: "Luyện Khí"
    stages: 9
    stat_multiplier: 1.0
    breakthrough_to: foundation_establishment
    breakthrough_requirements:
      min_cultivation_points: 100
      required_items: []
      base_chance: 0.35
```

Breakthrough chance is computed by rules:

```txt
chance =
  base_chance
  + comprehension_bonus
  + pill_bonus
  + location_bonus
  + quest_modifier
  - injury_penalty
```

The engine resolves success or failure. The LLM describes the result.

### Combat

Combat stats:

- `hp`, `max_hp`
- `spiritual_energy`, `max_spiritual_energy`
- `attack`
- `defense`
- `speed`
- `realm`
- `stage`
- `techniques`
- `artifacts`
- `status_effects`

Technique example:

```yaml
- id: fireball
  name: "Hỏa Cầu Thuật"
  energy_cost: 8
  power: 14
  accuracy: 0.85
  tags: ["fire", "basic_spell"]
```

Damage formula starts simple:

```txt
raw = technique.power + attacker.attack - defender.defense
damage = clamp(raw * realm_gap_modifier, min=1, max=cap)
```

### Rewards

Reward types:

- item
- spirit stones
- cultivation points
- relationship delta
- quest progress
- world flag unlock

Reward validation rules:

- Cannot grant items outside the item catalog.
- Cannot exceed danger or quest difficulty caps.
- Cannot complete a quest unless objectives are satisfied.
- Cannot change relationships beyond the per-turn limit.
- Cannot skip realm progression rules.

### Quests

Quests have explicit objectives:

```yaml
id: quest_outer_trial
objectives:
  - id: collect_herb
    type: collect_item
    item_id: moonlit_grass
    required: 3
  - id: report_elder
    type: talk_to_npc
    npc_id: elder_mo
```

Quest progress is based on resolved effects, never just narration text.

## CLI

System commands start with `/`:

```txt
/new
/load <save_id>
/save
/status
/inventory
/quests
/relations
/memories <query>
/config
/help
/exit
```

Roleplay input is free text:

```txt
ta hỏi Lục Thanh Nghi vì sao nàng lại giúp ta
ta dùng Hỏa Cầu Thuật công kích tên ma tu bên trái
ta ngồi xuống vận chuyển Thanh Mộc Quyết để tu luyện
```

Common turn result:

```go
type TurnResult struct {
    Narration        string
    StateChanges     []StateChangeView
    SuggestedActions []string
    Warnings         []string
    NeedsInput       *InputRequest
}
```

Debug commands should exist from the start:

```txt
/debug retrieve <query>
/debug tags
/debug state
/debug last-prompt
/debug last-effects
```

These are required for diagnosing hallucination, retrieval mistakes, invalid effects, and prompt context issues.

## Save Layout

```txt
data/
  saves/
    <save_id>/
      state.json
      events.jsonl
      game.db
      llm_logs/
```

`state.json` is a debuggable snapshot. `events.jsonl` is append-only history. `game.db` stores memories and FTS index. A later version may move all state into SQLite for stronger transactions.

## Future Web And Bot API

The reusable session boundary:

```go
type GameSession interface {
    StartNew(ctx context.Context, req NewGameRequest) (*TurnResult, error)
    HandleTurn(ctx context.Context, saveID string, input PlayerInput) (*TurnResult, error)
    GetStatus(ctx context.Context, saveID string) (*StatusView, error)
}
```

Future HTTP wrapper:

```txt
POST /sessions
POST /sessions/{id}/turn
GET  /sessions/{id}/status
GET  /sessions/{id}/quests
```

CLI calls the same boundary directly. Web and bot adapters can call it through HTTP or an embedded service.

## Testing Strategy

Use fake LLM clients by default. Tests must not call API providers unless explicitly marked integration.

Priority tests:

- Combat damage, cost, hit chance, and defeat conditions.
- Breakthrough chance bounds and state changes.
- Reward validation and illegal reward rejection.
- Quest objective progression.
- Memory insertion, tag/entity filters, FTS search, and rerank ordering.
- Orchestrator behavior when LLM proposes invalid effects.
- Invalid action handling that does not mutate state.
- Extractor output validation that drops unknown tags and predicates.

## Non-Goals For MVP

- Vector database.
- Multi-player.
- Full tactical combat grid.
- Full web UI.
- Bot integration.
- Procedural campaign generator.
- Long-term cloud sync.

## Extension Path

The memory layer should expose interfaces for future vector RAG without changing game core:

```go
type MemoryStore interface {
    Add(ctx context.Context, memory Memory) error
    Search(ctx context.Context, query RetrievalQuery) ([]MemoryHit, error)
}
```

Future implementation:

```txt
HybridMemoryStore
  SQLite FTS
  vector index
  score fusion
  recency and importance boosts
```

Optional Python tooling may be added later for offline prompt eval, embedding experiments, memory inspection, or batch tag cleanup, while Go remains the runtime.
