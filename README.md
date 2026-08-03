# Tutine TRPG

Tutine TRPG is a rule-driven LLM text RPG inspired by xianxia cultivation stories. The game is designed around a strict Go game engine, SQLite FTS memory retrieval, and OpenAI-compatible LLM providers.

The first playable interface will be a CLI. The CLI is only an adapter: the core game session, rules, memory, and LLM orchestration should remain reusable by future web and bot frontends.

## Direction

Tutine is a hybrid text RPG:

- The player can choose a name and light personality/play-style traits.
- The starting campaign is fixed around a new cultivator entering a sect.
- The rules stay authoritative for cultivation, combat, inventory, quests, rewards, and relationships.
- The LLM acts as narrator, NPC actor, retrieval planner, and memory extractor.
- SQLite FTS plus tags/entities/facts provides the first memory layer before any vector database is added.

## Planned Architecture

```txt
cmd/tu-tien-cli
  CLI adapter: read commands, send turns to the session layer, render results.

internal/game
  Rule engine and source of truth for player state, cultivation, combat, quests, inventory, rewards, and NPC relationships.

internal/orchestrator
  Turn coordination: retrieval, LLM calls, validation, state updates, event logging, and memory extraction.

internal/memory
  SQLite FTS store with metadata filters, controlled tags, facts, and reranking.

internal/llm
  OpenAI-compatible provider client with JSON/text calls, retries, timeouts, and schema validation.

internal/storage
  Save/load, event log, and SQLite database lifecycle.

campaigns/<campaign-id>
  Data packs for lore, realms, tags, techniques, items, NPCs, locations, and quests.
```

## Core Design Rules

- Game state is the source of truth.
- The LLM never mutates state directly.
- LLM-proposed effects must be validated, rejected, or clamped by the rule engine.
- Campaign tags are controlled vocabulary; unknown LLM tags are mapped or dropped.
- Combat outcomes are resolved by the engine before narration.
- Provider configuration uses an OpenAI-compatible API boundary.

## Current Status

This repository currently contains the approved MVP design documentation. Implementation has not started yet.

Design spec:

- [`docs/superpowers/specs/2026-08-03-tu-tien-llm-rpg-design.md`](docs/superpowers/specs/2026-08-03-tu-tien-llm-rpg-design.md)

Setup plan:

- [`docs/superpowers/plans/2026-08-03-tutine-trpg-repo-setup.md`](docs/superpowers/plans/2026-08-03-tutine-trpg-repo-setup.md)

## Name

`Tutine` is an anagram-inspired name from `tu tien`, short for the Vietnamese term `tu tien`/`tu tiên`, meaning cultivation or xianxia-style immortal practice. `TRPG` means text RPG in this project.
