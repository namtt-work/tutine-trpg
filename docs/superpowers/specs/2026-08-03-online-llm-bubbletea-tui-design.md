# Online LLM And Bubble Tea TUI Design

## Goal

Make Tutine TRPG playable through a real LLM-backed terminal UI. The CLI must no longer expose an offline play mode. Runtime play uses an OpenAI-compatible provider configured from `configs/llm.yaml`, while deterministic fake LLM behavior remains available only for tests.

## Runtime Behavior

- `go run ./cmd/tu-tien-cli --name Nam` starts the Bubble Tea TUI.
- The default config path is `configs/llm.yaml`; users may override it with `--config <path>`.
- The default provider config targets Groq through an OpenAI-compatible Chat Completions API.
- The configured API key is read from the environment variable named by `llm.api_key_env`, defaulting to `GROQ_API_KEY` in `configs/llm.yaml`.
- If the config file is missing, the API key env var is unset, or required LLM fields are empty, startup fails with a clear message. The CLI must not silently fall back to `FakeClient`.

## LLM Client

`internal/llm` adds an OpenAI-compatible HTTP client that implements the existing `llm.Client` interface:

- `PlanRetrieval` sends a compact prompt and expects JSON matching `RetrievalPlan`.
- `Narrate` sends state, retrieved context, player action, and allowed effects; it expects JSON matching `NarratorResponse`.
- `ExtractMemories` sends the resolved turn and expects a JSON array of `MemoryDraft`.
- Requests use the configured model, base URL, timeout, and retry count.
- Transient HTTP failures and rate limits are retried up to `max_retries`.
- Invalid JSON, missing response content, and non-transient provider errors return errors to the caller.

The LLM still cannot mutate game state directly. Proposed effects flow through `game.ApplyEffects`, which validates, rejects, or clamps state changes.

## TUI Design

The CLI adapter is replaced with a Bubble Tea full-screen terminal UI. It remains an adapter only and calls `orchestrator.GameSession` for all gameplay turns.

The screen contains:

- Header: game title, current scene, and active model/provider hint.
- Main log: narration, system messages, warnings, and errors in chronological order.
- Player panel: name, realm, stage, HP, spiritual energy, and compact inventory count.
- Suggested actions panel: numbered actions from the latest turn.
- Input bar: free-text action input with command support.
- Footer: concise shortcuts such as `/help`, `/status`, `/inventory`, `/exit`, and number selection.

## Interaction Rules

- Free text sends a turn to the session.
- A number selects the latest suggested action. If that action maps to an internal command, no LLM call is made.
- `/help`, `/status`, `/inventory`, and `/exit` remain supported.
- While an LLM turn is in flight, the UI shows a loading state and prevents duplicate submission.
- LLM or validation errors are appended to the log and do not crash the app.
- On narrow terminals, the layout collapses into a single-column log, player summary, suggestions, and input bar.

## Tests

- Unit tests continue to use `llm.FakeClient` or test doubles and must not call real provider APIs.
- LLM provider tests use fake HTTP servers.
- TUI behavior tests cover command routing, number selection, and error rendering without network calls.
- Before completion, run `gofmt` on Go files and `go test ./...`.

## Boundaries

- Do not add real network calls to default tests.
- Do not let Bubble Tea code leak into `internal/game`, `internal/llm`, `internal/memory`, or `internal/orchestrator`.
- Do not add offline runtime flags or fake fallback paths to CLI play.
