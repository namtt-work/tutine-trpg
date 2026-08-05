# Spec Review — 2026-08-04-fix-review-findings.md (round 1)

Reviewer: independent review gate (read-only). Artifact: `docs/superpowers/plans/2026-08-04-fix-review-findings.md` (648 lines), HEAD `8834ad0`.

## Scope of review

- Artifact read in full. Goal matches the five findings (P1 shutdown race, P1 unbounded LLM reads, two P2 visual-layout deviations, P2 world-readable files). No scope creep; out-of-scope list (best-effort persistence, stale `.lock` cleanup, SIGINT-as-signal) is consistent with the persistence spec.
- Spec citations verified against the actual files:
  - Layout spec line 82 ("14-17 rows condensed: no section labels, viewport minimum 2") — cited accurately by Task 3.
  - Layout spec line 106 (compact pending = "one status line plus compact paging footer") — cited accurately by Task 4.
  - Persistence spec: `.lock` `O_CREATE|O_EXCL` (line 148), state.json temp+rename (CreateTemp), events.jsonl append-only (line 95), SaveSnapshot-before-AppendEvent with failure-to-warning (lines 114-131), best-effort contract (lines 131-144) — all preserved by the plan; Task 5 changes modes only, never flags or ordering.
- All source line references verified against the tree (exact matches): tui.go 72-99/73/151/197-203/234-235/344/360-362/423/433-435/441/443/578-597/589/691-693; main.go 38-57/102/105; openai_compatible.go 286/298-305/214/305; file_store.go 43/51/109/116/144/148; orchestrator session.go HandleTurn (103-157, SaveSnapshot before AppendEvent); tui turn cmd runs `HandleTurn(m.ctx, …)` (tui.go:302/406); `cleanup` releases lock/closes SQLite/log (main.go:133-137). The P1 race description is accurate.
- Test-first discipline: every task writes a failing test first; all new tests reuse existing helpers (`keyPress`, `assertQuitCommand`, `recordingSession`, `newTestOpenAIClient`, `httptest`); test constructions match existing committed patterns (`tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl})`, `model.View().Content` with `tea.View` struct, `writeTestConfig` convention in main_test.go). `View()` returns `tea.View` (struct with `Content`), `syncLayout()` is a pointer-receiver method on an addressable local — both compile.
- Cross-task consistency: Task 1's signal.NotifyContext + deferred cleanup ordering is sound (quit gated on `turnFinishedMsg`; ctx cancellation aborts in-flight LLM calls via request ctx; normal quit path unchanged). Task 5 keeps `.lock` lifecycle and persistence ordering untouched. Tasks 3+4 match their spec row budgets. Task 2's truncation is applied before constructing `providerStatusError`, and no existing test asserts full error-body text.

## Findings

### MAJOR — Task 4 breaks existing TestTUIVisualStateMatrixFitsTargetLayouts (pending-60x10) and does not account for it

- location: plan Task 4 (steps 3-4), `cmd/tu-tien-cli/tui_test.go:644`
- evidence: `tui_test.go:644` asserts the pending state at all four targets `{60,18},{60,17},{60,14},{60,10}` must contain `"ĐANG XỬ LÝ LƯỢT"`. The only source of that string at 60x10 is the compact pending branch `tui.go:691-693` ("ĐANG XỬ LÝ LƯỢT", spinner line, footer) that Task 4 deletes. After Task 4, the matrix test fails on `pending-60x10` with "state control ĐANG XỬ LÝ LƯỢT missing". Task 4 Step 4's run regex (`TestTUICompactPendingRendersOneStatusLinePlusPagingFooter|TestTUIPendingTurn|TestTUICompact`) does not match `TestTUIVisualStateMatrix...`, so the break surfaces only in Step 5's `go test ./...`, and — unlike Task 3's Step 4, which explicitly instructs updating conflicting assertions — Task 4 gives no instruction to update this test.
- required change: add an explicit instruction in Task 4 to update the pending-state expectation in `TestTUIVisualStateMatrixFitsTargetLayouts` (60x10 case: expect `"Đang xử lý lượt"` + `"PgUp/PgDn"` instead of `"ĐANG XỬ LÝ LƯỢT"`), and include the matrix test in Task 4 Step 4's run command.

### MAJOR — Task 5's main_test.go test cannot compile and cannot pass as written

- location: plan Task 5 Step 1 (`TestBuildSessionUsesPrivateDataPermissions`), `cmd/tu-tien-cli/main_test.go:259`
- evidence: three independent defects against the actual repository:
  1. `cfgPath := writeTestConfig(t, dataDir)` — the existing helper is `writeTestConfig(t *testing.T, dataDir string, apiKeyEnv string) string` (main_test.go:259); every existing call passes three args (e.g. main_test.go:18, 62). The two-arg call is a compile error.
  2. `dataDir := filepath.Join(t.TempDir(), "data")` — the subdirectory does not exist yet; `writeTestConfig` writes `dataDir/llm.yaml` via `os.WriteFile` with no `MkdirAll` (main_test.go:262-266), so the helper fails before `buildSession` ever runs.
  3. No `t.Setenv("TEST_GROQ_API_KEY", "secret-test-key")` — `buildSession` fails on missing API key (see `TestBuildSessionRejectsMissingAPIKey`, main_test.go:16), so even a corrected test cannot pass without it.
  The plan's self-review claims the helper is "the only named-but-unverified symbol … with an explicit fallback instruction to extract it" — but the helper exists with a different signature, so the fallback is inert and the mitigation is not actually in force.
- required change: rewrite the test to match the existing convention: `dataDir := t.TempDir()`, `t.Setenv("TEST_GROQ_API_KEY", "secret-test-key")`, `cfgPath := writeTestConfig(t, dataDir, "TEST_GROQ_API_KEY")`, then assert perms on `dataDir` and `dataDir/debug.log`.

### MINOR — Task 1 Step 4 cites a nonexistent pre-existing test

- location: plan Task 1 Step 4 ("including the pre-existing … `TestTUIExitCommand...` tests")
- evidence: no `TestTUIExitCommand*` test exists in `cmd/tu-tien-cli/tui_test.go`; the existing quit-path tests are `TestTUIKeyPressCtrlCQuits` (248), `TestTUIKeyPressEscQuitsWhenNoLayerIsOpen` (241), `TestTUIAmbiguousCompletionAllowsExitCommand` (838). The behavior claim (pending nil ⇒ quit stays immediate) is correct; only the test names are wrong.
- required change: list the actual existing test names, or drop the reference.

### MINOR — Task 2 Step 2 misdescribes the expected pre-implementation failure mode

- location: plan Task 2 Step 2 ("FAIL — no limit enforced, oversized body decodes into an unrelated error or the message is unbounded")
- evidence: `maxResponseBytes`/`maxErrorBodyBytes` are undefined until Step 3, so the two new tests fail at compile time, not with the described runtime behavior. Task 1's Step 2 correctly anticipates "compile error or …"; Task 2's does not, which is inconsistent with the plan's self-review claim that every step is concrete.
- required change: reword Step 2 to note the compile-error failure mode (constants undefined), mirroring Task 1.

## Non-findings verified sound

- Task 1: runTUI derivation of cancelable child ctx, no-op `cancel` default in `newTUIModel`, quitting gating in all three quit paths and at top of `applyTurnMsg`, `signal.NotifyContext` root in `run`; cleanup ordering untouched; known SIGINT-as-signal limitation is accurate and consistent with the accepted stale-lock recovery.
- Task 2: `io.LimitReader` cap and `4 KiB` error-body truncation with `…` marker; no existing test asserts full provider error bodies; retry semantics unaffected (existing small-body tests unaffected).
- Task 3: `-3 → -2` budget and `1 → 2` floor exactly match spec line 82; `wideBreakpoint = 90` guarantees 60x14-17 renders the condensed shell; viewport content after one resolved turn is ≥ 2 lines, so the new test's `vpRows >= 2` assertion is achievable; the only tests asserting "NHẬT KÝ HÀNH TRÌNH" run at 60x18 (narrow full, tui.go:562) or 100x30 (wide, tui.go:524), neither modified.
- Task 5: storage test `TestSaveSnapshotAndEventsUsePrivatePermissions` compiles (Event/EventTypeTurnResolved/NewFileStore/AcquireLock/Lock.Release all exist) and fails pre-fix for the right reasons; `state.json` is 0600 via `os.CreateTemp` + rename (file_store.go:51); existing perms-related tests only `chmod` explicit dirs and are unaffected.

VERDICT: NEEDS_CHANGES
