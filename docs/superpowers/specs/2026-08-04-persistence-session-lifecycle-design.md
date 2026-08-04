# Persistence And Session Lifecycle Design

## Goal

Make game progress durable across process runs. Today every launch of `cmd/tu-tien-cli` calls `game.NewStarterSave` and creates a brand new save directory; nothing written during play survives the process exiting. This phase implements the `internal/storage` package and the save/load lifecycle already described in the MVP design's Save Layout and Event Log sections, so a player can close the game and resume the same character later.

## Scope

- Add `internal/storage`: a `Store` implementation that persists `state.json`, appends to `events.jsonl`, and lists existing saves under the configured data directory.
- Wire `orchestrator.Session` to persist a snapshot and an event after every resolved turn.
- Change CLI startup in `cmd/tu-tien-cli` to resume the most recently played save by default, or start a new one via a flag.
- Add a lightweight `/save` info command so players can confirm their progress location, matching the CLI command list in the MVP design.
- Add a single-process advisory lock per save directory so two CLI instances cannot silently corrupt the same save.

## Non-Goals

- Moving `SaveGame` state into SQLite transactions. `state.json` plus `events.jsonl` remain the source of truth this phase, per the MVP design's note that a later version may consolidate into SQLite.
- Mid-session hot-swap between saves (an in-game `/load <save_id>` that switches state without restarting the process). Switching saves means restarting the CLI with `--save <id>`, the same pattern already used for `--config`.
- An interactive save-picker screen inside the Bubble Tea TUI. Auto-resuming the latest save is sufficient for MVP single-character play; a picker UI is a future enhancement once multiple concurrent characters matter.
- Cloud sync, export/import, encryption.
- Full cross-file transactional consistency between `state.json` and `events.jsonl` (a write-ahead log, commit markers, or crash recovery that reconciles the two). This is a single-player local save file, not a database; see Consistency And Crash Behavior for the lighter guarantee this phase actually provides and why that is enough.
- Distributed or networked locking. The advisory lock added in scope only protects against two local processes on the same machine opening the same save; it is not a general concurrency-control system.
- Combat, cultivation breakthrough, quests, or reward catalog work. That is a separate phase.
- Changes to the memory/FTS store beyond continuing to key `game.db` by `save_id` as it does today.

## Current State (Why This Is Needed)

- `cmd/tu-tien-cli/main.go` `buildSession` always calls `game.NewStarterSave(...)`, generating a fresh `SaveID` on every run.
- `internal/memory.NewSQLiteStore` already creates `data/<dir>/saves/<save_id>/game.db`, so the save directory convention exists, but nothing writes `state.json` or `events.jsonl` into it.
- `orchestrator.Session` holds `save game.SaveGame` only in memory; `HandleTurn` mutates it via `game.ApplyEffects` and never persists it.
- The CLI has no way to list, resume, or select a save. Every session is single-use.

## Architecture

```txt
internal/storage
  Store interface: SaveSnapshot, AppendEvent, ListSaves, AcquireLock.
  FileStore: filesystem implementation rooted at the configured data directory.
```

`internal/storage` depends on `internal/game` for `SaveGame`, `StateChangeView`, and related types, the same way `internal/orchestrator` already does. It does not depend on `internal/llm`, `internal/memory`, or any CLI package.

```go
package storage

type Store interface {
    SaveSnapshot(ctx context.Context, save game.SaveGame) error
    LoadSnapshot(ctx context.Context, saveID string) (game.SaveGame, error)
    AppendEvent(ctx context.Context, saveID string, event Event) error
    ListSaves(ctx context.Context, campaignID string) ([]SaveSummary, error)
    AcquireLock(ctx context.Context, saveID string) (Lock, error)
}

type Lock interface {
    Release() error
}

type Event struct {
    Turn            int                    `json:"turn"`
    Type            string                 `json:"type"`
    PlayerAction    string                 `json:"player_action"`
    ResolvedEffects []game.StateChangeView `json:"resolved_effects"`
    Narration       string                 `json:"narration"`
    Warnings        []string               `json:"warnings,omitempty"`
    CreatedAt       time.Time              `json:"created_at"`
}

type SaveSummary struct {
    SaveID       string
    CampaignID   string
    PlayerName   string
    CurrentTurn  int
    CurrentScene string
    UpdatedAt    time.Time
}
```

`AppendEvent` takes `saveID` explicitly rather than adding a `SaveID` field to `Event`: `Event` describes what happened, not where it's filed, and an explicit parameter matches how `LoadSnapshot` already identifies its target. `Lock` is returned by `AcquireLock` and is the only supported way to acquire the per-save advisory lock described in Concurrent Access — callers never touch `.lock` directly, and `internal/orchestrator` and its tests only need `Store`, never the concrete `FileStore`.

`Event.Type` is `"turn_resolved"` for the roleplay/combat turns this phase covers. The MVP design's richer event types (e.g. `combat_result`) stay available for later phases that add real combat resolution.

## File Layout

Unchanged from the MVP design, and consistent with the directory `buildSession` already creates:

```txt
data/<env>/saves/<save_id>/
  state.json
  events.jsonl
  game.db
  .lock
```

- `state.json`: `json.MarshalIndent` of the current `game.SaveGame`, overwritten each turn. Written to a temp file in the same directory, `fsync`ed, then renamed into place, so a crash mid-write cannot leave a truncated or half-written `state.json`.
- `events.jsonl`: opened with `O_APPEND|O_CREATE`, one JSON object per resolved turn, `fsync`ed after each write. Never rewritten in place.
- `game.db`: unchanged, owned by `internal/memory`.
- `.lock`: created and removed via `Store.AcquireLock`/`Lock.Release`, held for the lifetime of the process holding the save; see Concurrent Access.

`SaveSummary.UpdatedAt` comes from `state.json`'s file modification time rather than a new field on `SaveGame`, so this phase does not need to change `internal/game`.

Directory-metadata `fsync` (guaranteeing the rename itself survives a full power loss, not just a process crash) is intentionally not implemented — see Consistency And Crash Behavior.

## Orchestrator Integration

`orchestrator.NewSession` takes an added `storage storage.Store` parameter, matching how it already takes `memory.Store`:

```go
func NewSession(save game.SaveGame, client llm.Client, memories memory.Store, store storage.Store, allowedTags []string) *Session
```

`HandleTurn` already runs, in order: apply proposed effects, increment `CurrentTurn`, remember the turn for narrator continuity, then extract and store memories (appending a warning to `warnings` on either failure, per the existing code). The persistence calls go immediately after that memory-extraction block, as the last thing `HandleTurn` does before building the returned `TurnResult` — matching the MVP design's turn flow order ("memory extractor creates durable memories... -> storage saves state, event log, and accepted memories"), and ensuring `Event.Warnings` reflects the turn's full warning list, not just the effect-application warnings:

```go
if err := s.storage.SaveSnapshot(ctx, s.save); err != nil {
    warnings = append(warnings, fmt.Sprintf("save persistence failed: %v", err))
}
if err := s.storage.AppendEvent(ctx, s.save.SaveID, storage.Event{
    Turn:            s.save.CurrentTurn,
    Type:            "turn_resolved",
    PlayerAction:    input.Text,
    ResolvedEffects: changes,
    Narration:       narration.Narration,
    Warnings:        warnings,
    CreatedAt:       time.Now().UTC(),
}); err != nil {
    warnings = append(warnings, fmt.Sprintf("event log write failed: %v", err))
}
return &game.TurnResult{Narration: narration.Narration, StateChanges: changes, SuggestedActions: narration.SuggestedNextOptions, Warnings: warnings}, nil
```

`SaveSnapshot` is always attempted before `AppendEvent`, and both are attempted regardless of whether the other succeeded — this ordering is deliberate, see Consistency And Crash Behavior for why `state.json` being current matters more than `events.jsonl` being complete. Note `Event.Warnings` intentionally cannot include the `AppendEvent` failure itself (the event is already constructed by the time that write is attempted); a failed event write is only visible via the `TurnResult.Warnings` returned to the player.

This follows the same pattern `HandleTurn` already uses for memory extraction failures: a persistence error becomes a player-visible warning on `TurnResult`, not a failed turn. The player's action was already resolved in memory; losing durability for one turn should not also discard the narration they just received.

A nil `Store` is not supported; tests use a fake `storage.Store` the same way they use a fake `memory.Store` and fake `llm.Client` today.

## Consistency And Crash Behavior

`state.json` and `events.jsonl` are written independently and this phase does not make them transactionally consistent with each other. A crash or write failure between the two calls in `HandleTurn` can leave `events.jsonl` missing the last entry, or (if `AppendEvent` fails after `SaveSnapshot` succeeds) leave a gap where `state.json` moved but the audit trail didn't record why. The reverse can also happen, since both writes are attempted independently regardless of the other's outcome (see Orchestrator Integration): if `SaveSnapshot` fails but `AppendEvent` still succeeds, `events.jsonl` gains an entry for a turn that `state.json` does not yet reflect. This is an accepted limitation, not an oversight, for two reasons:

- **`state.json` alone is sufficient for resume.** Startup (see CLI Startup And Resume Flow) only ever reads `state.json`. `events.jsonl` is never replayed to reconstruct state. So a gap in the event log does not put a resumed game in an inconsistent or unplayable state — the player's progress is exactly what `state.json` says it is.
- **`events.jsonl`'s job is audit and debugging, not correctness.** The MVP design describes it as being for "audit, debugging, and possible replay," not as a component the engine depends on to run. A best-effort audit trail that can occasionally miss or lag one entry after a crash is a reasonable trade for not building a write-ahead log or commit-marker protocol into a single-player local text file store.

Because of this, `SaveSnapshot` is called first on every turn: if only one of the two writes can succeed, it must be the one the game actually depends on. If a stronger guarantee (e.g. replay-based recovery, or event log as source of truth) becomes a real requirement, that is a redesign of the storage layer, not an incremental addition to it — flag it as a new spec rather than patching this one.

## Concurrent Access

Two processes are not allowed to hold the same save open at once. `FileStore.AcquireLock(ctx, saveID)` first ensures `data/<dir>/saves/<save_id>/` exists (`MkdirAll`, idempotent whether the save is new or already on disk), then creates `.lock` inside it with `O_CREATE|O_EXCL`, writes the current PID into it, and returns a `Lock` whose `Release()` removes the file. If `.lock` already exists, `AcquireLock` fails immediately with an actionable error identifying the save and instructing the player to close the other instance (or, if that process is confirmed dead, delete the `.lock` file manually).

`AcquireLock` owning directory creation matters specifically for new games: `game.NewStarterSave` only builds an in-memory `SaveGame` (`internal/game/state.go`), it performs no I/O, so nothing else has created `saves/<save_id>/` by the time `AcquireLock` runs. Putting `MkdirAll` in `AcquireLock` keeps that filesystem-lifecycle responsibility inside `internal/storage` rather than splitting it between the CLI and the store; the initial `SaveSnapshot` that follows can then assume the directory is already there, the same as it can for a resumed save.

Because a new save's ID does not exist until `game.NewStarterSave` creates it, the lock point differs by startup path (see CLI Startup And Resume Flow for the full sequence each path runs in):

- **Explicit `--save <id>`:** validate the ID, `AcquireLock(id)`, then `LoadSnapshot(id)`.
- **Auto-resume:** `ListSaves` to pick the latest save's ID, `AcquireLock(id)`, then `LoadSnapshot(id)`.
- **New game:** `game.NewStarterSave(...)` first (in-memory only), then `AcquireLock(save.SaveID)` (which creates `saves/<save_id>/`), then the initial `SaveSnapshot` described in CLI Startup And Resume Flow.

In every path, any error after `AcquireLock` succeeds (a failing `LoadSnapshot`, a failing initial `SaveSnapshot`, a failing `memory.NewSQLiteStore`) must `Release()` the lock before `buildSession` returns its error — otherwise a failed startup would leave the save locked out for the next attempt. `buildSession`'s `cleanup` closure calls `lock.Release()` unconditionally, independently of whether closing `memory.Store` or the debug log file also succeeds, so a failure to close one resource never skips releasing another.

This is intentionally a simple advisory lock, not a distributed or crash-safe locking system:

- It does not detect or reap a stale lock left behind by a killed process automatically. Given this is a local single-player tool, a manual `rm .lock` after a crash is an acceptable recovery step; automatic staleness detection (checking whether the PID is still alive) is deferred unless it turns out to be a frequent annoyance in practice. `AcquireLock` only knows that `.lock` exists, not whether the process that created it is still running — see Testing for how this is worded precisely.
- It only protects against two `cmd/tu-tien-cli` processes on the same machine racing on the same save directory. It says nothing about network filesystems or multiple machines sharing `data/`.

## Save ID Validation

`save_id` values reach `FileStore` from two places: internally generated (`game.newSaveID`, already a safe `save_<unixnano>_<seq>` shape) and directly from the player via `--save <id>`. The second is untrusted input, and every `FileStore` method that takes a save ID as a parameter (`SaveSnapshot`, `LoadSnapshot`, `AppendEvent`, `AcquireLock`) builds a path with `filepath.Join(dataDir, "saves", saveID, ...)`. An unvalidated `saveID` of `../../etc` or an absolute path escapes the saves root. `ListSaves` does not take a save ID — it discovers IDs by scanning `saves/` — so it is covered separately below rather than by this same input check.

`FileStore` validates the ID at the top of every method that accepts one, not just at CLI flag parsing, since `internal/storage` must not depend on its caller having already sanitized input:

```go
func validateSaveID(id string) error {
    if id == "" || id != filepath.Base(id) || id == "." || id == ".." {
        return fmt.Errorf("invalid save id %q", id)
    }
    return nil
}
```

`filepath.Base(id) != id` rejects any ID containing a path separator (so also rejects absolute paths, since `filepath.Base` of an absolute path never equals the original string unless it's a single trailing segment — combined with the empty/`.`/`..` checks this closes the traversal case). `--save <id>` that fails validation is a startup error, not a silent fallback to a new game.

**`LoadSnapshot` also cross-checks the loaded content, not just the input ID.** A directory name is trusted to be a valid ID by construction (it was created by `AcquireLock` from an already-validated ID), but the `state.json` inside it is a plain file a player could hand-edit or a bug could write incorrectly. `LoadSnapshot(ctx, requestedID)`:

1. Validates `requestedID` as above.
2. Deserializes `saves/<requestedID>/state.json`.
3. Requires the deserialized `SaveGame.SaveID` to equal `requestedID`; otherwise returns an actionable corruption error instead of the snapshot.

This matters because `cmd/tu-tien-cli` derives the `game.db` directory for `memory.NewSQLiteStore` from the *requested/canonical* save ID (the ID passed to `AcquireLock`/`LoadSnapshot`, or the one `ListSaves`/`NewStarterSave` produced) — never from re-reading `save.SaveID` off the loaded struct after the fact. Without the cross-check, a corrupted `state.json` whose embedded `SaveID` differs from its directory could otherwise cause `game.db` to be opened somewhere inconsistent with the state that was just loaded. `ListSaves` applies the same rule while scanning: a directory entry whose name fails `validateSaveID`, or whose `state.json` embeds a different `SaveID` than its directory name, is skipped rather than returned as a `SaveSummary` — a malformed entry should be invisible to auto-resume, not silently trusted.

## Save Discovery Ordering

`ListSaves` orders results by `UpdatedAt` descending, then by `SaveID` descending as a deterministic tie-break. `UpdatedAt` is a file modification time (see File Layout), and some filesystems have coarse mtime resolution — two saves written within the same tick can compare equal. Without an explicit tie-break, "the most recently updated save" would silently depend on directory-read order, which is not something callers should rely on being stable across runs or platforms. `SaveID` is generated from `save_<unixnano>_<seq>` (`game.newSaveID`), so sorting it descending as a plain string tie-break also happens to prefer the more recently *created* save when two are updated in the same tick — a reasonable secondary signal, though the tie-break's job is determinism, not correctness of that ordering.

## CLI Startup And Resume Flow

New flags on `cmd/tu-tien-cli`:

```txt
--name <name>      player name for a new game (existing flag, unchanged)
--config <path>    runtime config path (existing flag, unchanged)
--save <save_id>   resume a specific save, skipping auto-resume
--new              force a new game even if a save exists
```

`buildSession` takes a small `StartupOptions{PlayerName, SaveID, ForceNew}` instead of a bare `name string`. `SaveID` set together with `ForceNew` is rejected immediately, before any of the resolution steps below run, with an actionable error ("--save and --new cannot be used together"). Silently letting one win — as an implicit if/else-if would — could make `--save x --new` quietly resume `x` when the player's `--new` reads as an unambiguous request for a fresh character.

`buildSession` then resolves in this order:

1. If `SaveID` is set, validate it (see Save ID Validation), `store.AcquireLock(ctx, id)`, then `storage.LoadSnapshot` that save. Fail fast with a clear error if it does not exist, fails validation, or the lock is already held.
2. Else if `ForceNew` is set, create via `game.NewStarterSave` with `PlayerName`, `store.AcquireLock(ctx, save.SaveID)`, then the initial snapshot write described below.
3. Else, call `storage.ListSaves(ctx, "thanh-van-sect")`. If it returns at least one save, `store.AcquireLock` the one with the latest `UpdatedAt` (ties broken by `SaveID` descending, see Save Discovery Ordering) and `LoadSnapshot` it. If it returns none, fall back to step 2.

Every step acquires the lock before touching `game.db` or building the `orchestrator.Session`; see Concurrent Access for the full rationale and the error/cleanup rules that apply once a lock is held.

In all three cases, `internal/memory.NewSQLiteStore` opens `game.db` inside `data/<dir>/saves/<save_id>/`, where `<save_id>` is the canonical ID `buildSession` already resolved and passed to `AcquireLock`/`LoadSnapshot` in that step — not a value re-read from the loaded `SaveGame.SaveID` field, per the cross-check in Save ID Validation. This carries `game.db` over unchanged on resume, so retrieved memories and the FTS index remain available.

**A new save must exist on disk before the TUI starts, not only after its first resolved turn.** `HandleTurn` only persists on a completed turn (see Orchestrator Integration), so a player who starts a new game and quits before their first successful LLM call would otherwise have no `state.json` for `ListSaves` to find on the next launch — silently losing a character that was never actually "in progress" longer than a few seconds. To close this, `buildSession` calls `store.SaveSnapshot(ctx, save)` immediately after acquiring the lock in steps 2 and 3's new-game path, before constructing the `orchestrator.Session` or returning control to `runTUI`, and treats a failure there as a fatal startup error — the same way a failure to open `game.db` already is. The resume path (step 1) does not need this, since a save that can be loaded already has a `state.json`.

The startup log line (written via the existing `debug.log` logger) states which path was taken: `resumed save <id> at turn N` or `started new save <id>`. This is diagnostic only, not shown in the TUI.

## `/save` Command

Added to the CLI command palette alongside `/status` and `/inventory`, using the same temporary-information-view pattern from the friendly TUI flow design. The friendly-flow design's data-mapping rule ("internal identifiers must not appear in player-facing UI") applies here: `/save` must not render the raw `save_id` or filesystem path.

```txt
> /save
Tien trinh da duoc luu tu dong o luot 7.
```

Since every turn already persists automatically, `/save` does not trigger a new write. It renders only the current turn number from `session.Save()`, confirming to the player that progress is not lost if they quit. `/help` text is updated to mention it and to explain that saves resume automatically on next launch. The raw save ID and directory path remain available in `debug.log` (already written at startup) for anyone who needs to locate the file on disk; this phase does not add a player-facing view for them.

`/load` and `/new` are intentionally not added as in-game commands this phase; see Non-Goals.

## Testing

`internal/storage`:

- `SaveSnapshot` then `LoadSnapshot` round-trips an equivalent `game.SaveGame`.
- `LoadSnapshot` on an unknown `save_id` returns a clear not-found error.
- `AppendEvent` called multiple times produces one JSON object per line, each parseable independently.
- `ListSaves` filters by `campaign_id` and orders by `UpdatedAt` descending.
- `ListSaves` given two saves with equal `UpdatedAt` (set both `state.json` mtimes to the same value) orders them by `SaveID` descending, per Save Discovery Ordering, rather than leaving the order unspecified.
- A `SaveSnapshot` failure (e.g. read-only directory) does not leave a partially written `state.json`; the previous valid snapshot, if any, remains readable.
- `AcquireLock` on a save directory that already has a `.lock` file fails with an actionable error, regardless of whether the process that created it is still running — this phase does not check PID liveness, so the test asserts "an existing lock file blocks acquisition," not "a live process blocks acquisition."
- `Lock.Release()` removes `.lock`; a subsequent `AcquireLock` for the same save ID then succeeds.
- `AcquireLock` for a save ID that has no directory yet (a fresh `t.TempDir()`, simulating the new-game path) creates `saves/<save_id>/` and succeeds, rather than failing with a missing-directory error.
- `LoadSnapshot(ctx, requestedID)` on a `state.json` whose embedded `SaveGame.SaveID` does not match `requestedID` (write it directly to simulate corruption) returns an actionable error instead of the mismatched snapshot.
- `ListSaves` skips a `saves/` entry whose directory name fails `validateSaveID` and a `saves/<id>/` entry whose `state.json` embeds a different `SaveID` than `<id>`, rather than including either in the returned summaries.

`internal/orchestrator`:

- `HandleTurn` calls `SaveSnapshot` and `AppendEvent` exactly once per successful turn, with the resolved `changes` and incremented turn number, `SaveSnapshot` before `AppendEvent`.
- A fake `Store` that errors on `SaveSnapshot` or `AppendEvent` produces a warning on `TurnResult` and still returns the narration and state changes; it does not fail the turn.
- A fake `Store` that errors on `SaveSnapshot` still has `AppendEvent` called (both are attempted independently, per Consistency And Crash Behavior).

`cmd/tu-tien-cli`:

`buildSession` tests already use a real config file and a real `memory.SQLiteStore` rooted at `t.TempDir()` (see existing `main_test.go`), not fakes, because `buildSession` wires concrete dependencies rather than accepting injected ones. This phase keeps that convention rather than introducing a new fake-store seam: tests construct a real `storage.FileStore` over `t.TempDir()`, seed it directly (via `SaveSnapshot`) or via prior `buildSession` calls, then assert on the next `buildSession` call's outcome.

- With an empty data directory, `buildSession` with default `StartupOptions` starts a new game and its `state.json` exists on disk immediately after `buildSession` returns, before any `HandleTurn` call.
- With two saves already present with different `UpdatedAt` (seeded directly via `FileStore.SaveSnapshot`), default `StartupOptions` resumes the one with the latest `UpdatedAt`.
- `StartupOptions.SaveID` resumes that save regardless of what auto-resume would have picked.
- `StartupOptions.ForceNew` starts a new game even when saves exist.
- `StartupOptions{SaveID: "...", ForceNew: true}` (both set) fails with the actionable "cannot be used together" error, before any storage or lock call, regardless of whether `SaveID` refers to a real save.
- `StartupOptions.SaveID` set to an unknown or invalid (path-traversal-shaped) ID fails with an actionable error instead of silently starting a new game.
- `StartupOptions.SaveID` on a save that already has a `.lock` file (simulate by pre-creating it) fails with an actionable error instead of opening a second session on the same directory.
- A `buildSession` call that fails after acquiring the lock (e.g. `memory.NewSQLiteStore` returns an error) releases the lock before returning, so a subsequent `buildSession` call for the same save succeeds.
- `/save` renders the current turn without the raw save ID or path, and without calling `HandleTurn`.
- **Existing test update:** `TestBuildSessionUsesDistinctSaveStorage` currently asserts two consecutive `buildSession` calls with the same config produce distinct save IDs. Under default auto-resume that assumption no longer holds — the second call resumes the first call's save. Rewrite it to assert: (a) two default calls share the same save ID (resume) — calling `firstCleanup()` (which releases the lock) before the second `buildSession` call is required here, since the first save's lock is still held otherwise and the second call would correctly fail lock acquisition rather than resume, which is not what this case is testing; and (b) two calls with `ForceNew: true` produce distinct save IDs, where `firstCleanup()` before the second call is not load-bearing for the assertion but should still run to avoid leaking a held lock past the test.

Keep using a fake LLM client and fake `memory.Store`/`storage.Store` in `internal/orchestrator` tests; `cmd/tu-tien-cli` tests use real files under `t.TempDir()` per the existing convention. No test should touch the real filesystem outside `t.TempDir()` or call a real provider. Run `gofmt` on changed Go files and `go test ./...` after implementation.
