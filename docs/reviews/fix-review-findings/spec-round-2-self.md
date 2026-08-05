# Spec Review — 2026-08-04-fix-review-findings.md (round 2)

Reviewer: independent review gate (read-only). Artifact: `docs/superpowers/plans/2026-08-04-fix-review-findings.md`, HEAD `8834ad0`. Amended plan re-read in full; every round-1 finding re-verified against the real repository (not the author's summary).

## Round-1 findings: fix verification (all four confirmed present and correct)

### Fix 1 (round-1 MAJOR, Task 4 matrix test) — CONFIRMED FIXED
- Plan Task 4 Step 3 now instructs updating the pending contains-list in `TestTUIVisualStateMatrixFitsTargetLayouts` (tui_test.go:644) from `[]string{"ĐANG XỬ LÝ LƯỢT", "PgUp/PgDn"}` to `[]string{"Đang xử lý lượt", "PgUp/PgDn"}`, with the rationale that no state split is needed; Step 4's run regex now includes `TestTUIVisualStateMatrixFitsTargetLayouts`.
- Verified the single-list change works at every target by tracing the actual render dispatch in `View()` (tui.go:497-507): 60x18 → `renderNarrowFullShell`; 60x17/60x14 → `renderCondensedShell`; 60x10 → `renderCompactShell`. All three pending paths were inspected:
  - 60x18/17/14 pending renders via `renderNarrowNormalAction` → `renderBoundedStateAction` (tui.go:634): `"ĐANG XỬ LÝ LƯỢT"`, `truncateCells(m.spinner.View()+" Đang xử lý lượt chơi...", width)`, `"PgUp/PgDn lịch sử"` — contains both new substrings (`Đang xử lý lượt` ⊂ `Đang xử lý lượt chơi...`).
  - 60x10 compact pending (tui.go:691-693) after the plan's change: `truncateCells(m.spinner.View()+" Đang xử lý lượt...", contentWidth)` + `"PgUp/PgDn lịch sử"` — contains both new substrings.
- Grep confirms the uppercase heading `ĐANG XỬ LÝ LƯỢT` is asserted by no other test (only tui.go:634, tui.go:692, and the matrix test itself), and the full-narrow title at tui.go:634 is intentionally kept per layout spec line 141 (18+ rows contract), so Task 4 Step 5's `go test ./...` is not at risk.

### Fix 2 (round-1 MAJOR, Task 5 main_test.go test) — CONFIRMED FIXED
- Plan Task 5 Step 1's test now matches the existing convention verbatim: `dataDir := t.TempDir()` (pre-exists; `writeTestConfig` at main_test.go:259-266 does `os.WriteFile(dataDir/llm.yaml)` with **no** `MkdirAll`), `t.Setenv("TEST_GROQ_API_KEY", "secret-test-key")` (required — `TestBuildSessionRejectsMissingAPIKey` at main_test.go:16), and the three-arg call `writeTestConfig(t, dataDir, "TEST_GROQ_API_KEY")` (signature at main_test.go:259). Asserts `perm&0o077 != 0` on `dataDir` and `dataDir/debug.log`.
- Implementation targets verified: main.go:102 `os.MkdirAll(dataDir, 0o755)` → `0o700`; main.go:105 debug.log `OpenFile(..., 0o644)` → `0o600`; file_store.go:43/109/144 `MkdirAll(..., 0o755)` → `0o700`; file_store.go:116 events.jsonl `0o644` → `0o600`; file_store.go:148 `.lock` `O_CREATE|O_EXCL|O_WRONLY, 0o644` → `0o600`. Modes only; flags and ordering untouched. `state.json` already 0600 via `os.CreateTemp` (file_store.go:51) — no change, correctly stated. The old inert "extract helper" fallback language is gone from the self-review.

### Fix 3 (round-1 MINOR, Task 1 Step 4 test names) — CONFIRMED FIXED
- Step 4 now names the real tests: `TestTUIKeyPressCtrlCQuits` (tui_test.go:248), `TestTUIKeyPressEscQuitsWhenNoLayerIsOpen` (241), `TestTUIAmbiguousCompletionAllowsExitCommand` (838) — all exist. Inspected all three: pending is nil at quit time in each (fresh model / after `applyTurnMsg` / palette-exit), so the "quit stays immediate" claim holds.

### Fix 4 (round-1 MINOR, Task 2 Step 2 failure mode) — CONFIRMED FIXED
- Step 2 now reads: "FAIL — compile error first (`maxResponseBytes`/`maxErrorBodyBytes` are undefined until Step 3); after Step 3 the runtime assertions take over" — mirrors Task 1's Step 2 and matches reality (the two new tests reference the constants at the top level, so they fail to build before Step 3).

## New finding

### MAJOR — Task 2's truncation test cannot pass against Step 3's own implementation (internally inconsistent)

- location: plan Task 2 Step 1 (`TestOpenAICompatibleClientTruncatesProviderErrorBody`) vs Step 3 (`sendChatMessage` replacement), `internal/llm/openai_compatible.go:300-306`
- evidence: the test's error handler writes `make([]byte, maxResponseBytes+1024)` (~8 MiB) with status 500. Step 3's implementation checks the size guard **before** the status branch:
  1. `io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))` reads `maxResponseBytes+1` bytes;
  2. `len(data) > maxResponseBytes` → returns `fmt.Errorf("llm provider response exceeds %d bytes", maxResponseBytes)` — a plain ASCII error that is **not** `providerStatusError` and contains **no `…` marker**;
  3. the status branch (which would truncate and append `…`) is never reached.
  The test then asserts `strings.Contains(err.Error(), "…")` — this fails. The `len(err.Error()) <= maxErrorBodyBytes*2` assertion happens to pass (~46 bytes), so the test deterministically fails on the `…` check. Task 2 Step 4's "Expected: PASS" is therefore wrong; the implementer following the plan literally cannot reach Step 4's acceptance criterion without deviating (either the test body must exceed `maxErrorBodyBytes` while staying under `maxResponseBytes`, e.g. `maxErrorBodyBytes+1024`, or the status branch must run before the size guard, or the `…` assertion must target the provider-error path only). `TestOpenAICompatibleClientRejectsOversizedResponse` (200 with oversized body) does pass as written — only the truncation test is affected.
- required change: make the two pieces mutually consistent — e.g. shrink the truncation test's error body to `maxErrorBodyBytes+1024` (exercises the 4 KiB truncation + `…` marker without tripping the size guard), keeping the oversized-body test for the guard path; and re-verify Step 4's "Expected: PASS".

## Spot-checks of round-1 approved parts (re-verified, no re-litigation)

- Spec citations accurate: layout spec line 82 (14-17 condensed: no section labels, viewport minimum 2); line 106 (compact pending: one status line + compact paging footer); persistence spec (`2026-08-04-persistence-session-lifecycle-design.md`) events.jsonl `O_APPEND|O_CREATE` (line 95), `.lock` `O_CREATE|O_EXCL` (line 148), SaveSnapshot-before-AppendEvent with failure-to-warning (lines 114-131), best-effort contract (lines 131-144).
- Source line references all exact: tui.go 73 (ctx field), 197-203 (`runTUI`), 234-236 (Ctrl-C), 360-362 (handleEsc default), 433-434 (`/exit`), 443 (`m.pending = nil`), 578-597 (`renderCondensedShell`, `-3→-2` / floor `1→2` at 585-586), 634 (`renderBoundedStateAction` pending), 691-693 (compact pending); main.go 38-57 (`run`), 102/105; openai_compatible.go 214 (`maxToolCallRounds`), 300 (`io.ReadAll`), 305/334-343 (`providerStatusError`), `newTestOpenAIClient` (test line 400), `maxRetries` field; file_store.go 43/109/116/144/148.
- Test-first discipline and helper reuse intact: `keyPress(tea.KeyEsc, "")`, `tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl})`, `assertQuitCommand`, `recordingSession`, `newTestOpenAIClient`, `httptest`, `writeTestConfig(t, dataDir, apiKeyEnv)` — all match committed patterns.
- Cross-task consistency: Task 1 `signal.NotifyContext` root + deferred `cleanup()` ordering sound; gating in all three quit paths with pending-nil fallback verified; Task 5 changes modes only, `.lock` lifecycle and persistence ordering untouched; Tasks 3+4 match spec row budgets; Task 2 truncation precedes `providerStatusError` construction (aside from the ordering defect above).
- Goal still matches the five findings (P1 shutdown race, P1 unbounded LLM reads, two P2 layout deviations, P2 world-readable files); out-of-scope list unchanged; no scope creep.

VERDICT: NEEDS_CHANGES
