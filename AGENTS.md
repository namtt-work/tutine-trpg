# AGENTS.md

This is the root working guide for coding agents in `tutine-trpg`. Keep this file short. Use it as an operating manual and an index into the project docs, not as a duplicate of the design spec.

## Work Rules

- Read the relevant docs before changing files. Start with the index below, then open only the docs needed for the task.
- Make small, focused changes. Prefer the simplest correct implementation over new abstractions.
- Do not overwrite or revert user changes unless explicitly asked.
- Preserve the existing docs-first workflow: design decisions live in specs, implementation sequencing lives in plans.
- If a task touches game rules, memory, LLM orchestration, or persistence, check the MVP design spec first.
- If a task implements planned work, follow the matching plan step-by-step and keep checklist state accurate.
- Do not add real network calls to tests by default. LLM behavior should be tested through fakes or interfaces.

## Required Context Index

- `README.md`: human-facing project overview, current status, and planned architecture.
- `docs/superpowers/specs/2026-08-03-tu-tien-llm-rpg-design.md`: authoritative MVP product and architecture design.
- `docs/superpowers/plans/2026-08-03-tutine-trpg-repo-setup.md`: completed documentation-only repo setup history.
- `docs/superpowers/plans/2026-08-03-tutine-trpg-mvp-foundation.md`: implementation plan for the first runnable Go foundation.

## Project Invariants

- The game engine is the source of truth for player state, combat, inventory, quests, rewards, and NPC relationships.
- The LLM may narrate, act as NPCs, plan retrieval, and propose effects, but it must not mutate game state directly.
- Proposed LLM effects must be validated, rejected, clamped, or applied by rule-engine code.
- Combat outcomes are resolved by the engine before narration.
- SQLite FTS plus structured metadata is the MVP memory layer.
- The first UI is CLI, but core packages must remain reusable for future web or bot adapters.
- Provider integration should stay behind OpenAI-compatible interfaces.

## Planned Code Layout

- `cmd/tu-tien-cli`: CLI adapter only.
- `internal/game`: authoritative rules and state.
- `internal/orchestrator`: turn flow coordination.
- `internal/memory`: SQLite FTS memory and retrieval.
- `internal/llm`: provider contracts, fake client, and OpenAI-compatible boundary.
- `internal/storage`: save/load and event log lifecycle.
- `internal/config`: runtime and provider config.
- `campaigns/<campaign-id>`: campaign data packs.

## Verification

- For Go code, run `gofmt` on changed Go files before finishing.
- For focused package changes, run `go test ./path/to/package`.
- Before claiming a broad implementation is complete, run `go test ./...` when the Go module exists.
- For docs-only changes, inspect the rendered Markdown mentally and run no build unless the change affects generated docs or links that need tooling.

## Secrets And External Services

- Treat API keys, tokens, cookies, and local config values as secrets.
- Never commit local secret files or paste secret values into docs, commits, logs, or responses.
- If an LLM provider secret is required locally, reference the environment variable name only and ask the user to provide it outside source control.

## Git And Commits

- Do not commit unless the user explicitly asks.
- Before committing, inspect `git status` and `git diff`, then stage only intended files.
- Keep commit messages concise and project-scoped.
- Do not add AI-generated footers or co-author lines unless explicitly requested.
