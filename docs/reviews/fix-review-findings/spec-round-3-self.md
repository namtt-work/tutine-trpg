# Spec Review — 2026-08-04-fix-review-findings.md (round 3, final)

Reviewer: independent review gate (read-only). Artifact: `docs/superpowers/plans/2026-08-04-fix-review-findings.md`, HEAD `8834ad0`. Plan re-read in full; the round-2 MAJOR and all round-1 fixes re-verified against the real repository (not the author's summary). No files mutated; no git state changed.

## Round-2 finding: fix verification — CONFIRMED FIXED

### MAJOR (round 2) — Task 2 truncation test vs Step 3 implementation now mutually consistent

- location: plan Task 2 Step 1 (`TestOpenAICompatibleClientTruncatesProviderErrorBody`) vs Step 3 (`sendChatMessage` replacement + constants)
- evidence (traced against `internal/llm/openai_compatible.go` and `internal/llm/openai_compatible_test.go`):
  - The test's error handler now writes `make([]byte, maxErrorBodyBytes+1024)` with the inline comment "over the 4 KiB error-body cap, under the 8 MiB size guard". Constants (Step 3): `maxResponseBytes = 8 << 20` (8388608), `maxErrorBodyBytes = 4 << 10` (4096). Body = 5120 bytes → 5120 > 4096 and 5120 < 8388608, matching the comment exactly.
  - Step 3's implementation (size guard first, then status branch, preserving the existing order at openai_compatible.go:300-306): `io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))` reads the 5120-byte body fully (5120 < 8388609); `len(data) = 5120 > maxResponseBytes` is **false**, so the size guard does not trip; the status branch runs: `strings.TrimSpace(string(data))` leaves 5120 bytes (NUL bytes are not trimmed), `len(body) = 5120 > 4096` → `body = body[:4096] + "…"` (4097 bytes) → `providerStatusError{statusCode: 500, body: body}`.
  - `providerStatusError.Error()` (openai_compatible.go:338-343) = `"llm provider returned status 500: "` + 4097 bytes ≈ 4131 bytes → `strings.Contains(err.Error(), "…")` passes and `len(err.Error()) > maxErrorBodyBytes*2` (8192) is false, so the length assertion passes too.
  - Retry path confirmed: 500 ≥ 500 so `isTransientProviderError` is true; `client.maxRetries = 0` is a valid existing convention (same field mutated at openai_compatible_test.go:156, 354). `runChat` computes `maxSteps = 0 + maxToolCallRounds(6)` and returns `lastErr` **unwrapped** at `step == maxSteps` (openai_compatible.go:242-247), so the final error is the raw `providerStatusError` — no wrapping that could break the length bound or hide the `…` marker. Every attempt returns the identical error, so the test is deterministic.
  - Task 2 Step 4's "Expected: PASS" is therefore reachable; the implementer no longer needs to deviate from the plan.
- required change: none. (The size-guard-before-status ordering that caused round 2's defect is now harmless because the truncation body is deliberately under the guard; the plan correctly keeps the oversized-body test for the guard path.)

### Size-guard path still covered (round-2 check 2) — CONFIRMED
- `TestOpenAICompatibleClientRejectsOversizedResponse` (200 + `make([]byte, maxResponseBytes+1024)` = 8389632 bytes): LimitReader reads `maxResponseBytes+1` (8388609) bytes; `len(data) = 8388609 > 8388608` → `fmt.Errorf("llm provider response exceeds %d bytes", …)` contains "exceeds"; the error is not a `providerStatusError`, so it is non-transient and returned immediately. Unaffected by the truncation-test fix; both tests now exercise disjoint paths as intended.

## Round-1 fixes: spot-check (all four confirmed still present)

1. Task 4 Step 3 still updates the matrix contains-list in `TestTUIVisualStateMatrixFitsTargetLayouts` to `[]string{"Đang xử lý lượt", "PgUp/PgDn"}` with the no-state-split rationale; Task 4 Step 4's run regex still includes `TestTUIVisualStateMatrixFitsTargetLayouts`.
2. Task 5 Step 1 still uses the three-arg `writeTestConfig(t, dataDir, "TEST_GROQ_API_KEY")` convention, `t.Setenv("TEST_GROQ_API_KEY", "secret-test-key")`, and plain `t.TempDir()` (with the note that `writeTestConfig` does no `MkdirAll`).
3. Task 1 Step 4 still names the real pre-existing tests (`TestTUIKeyPressCtrlCQuits`, `TestTUIKeyPressEscQuitsWhenNoLayerIsOpen`, `TestTUIAmbiguousCompletionAllowsExitCommand`).
4. Task 2 Step 2 still carries the compile-error-first wording ("`maxResponseBytes`/`maxErrorBodyBytes` are undefined until Step 3") consistent with the tests referencing the constants at top level.

## No new inconsistencies introduced

- The test's inline comment matches the actual constant values and the guard/cap relationship; verified numerically.
- `maxErrorBodyBytes` is defined in Step 3 as a package-level constant next to `maxToolCallRounds` (openai_compatible.go:214, confirmed present), so once Step 3 lands the runtime assertions reference a defined symbol; before Step 3 the tests fail to compile exactly as Step 2 states. Both test files are `package llm`, so the unexported constants/field are reachable.
- Existing status-path tests (e.g. the 429 test at openai_compatible_test.go:346 with `http.Error(w, "rate limited", …)`) use small bodies far below 4 KiB, so the new truncation branch cannot disturb them; Task 2 Step 4's full-package PASS expectation holds.
- Cross-task consistency re-checked: constant and field introductions remain single-task; Task 1, 3, 4, 5 sections are unchanged from round 2's approved state; out-of-scope list and spec citations (layout spec lines 82/106) unchanged.

## Findings

No BLOCKER or MAJOR findings remain. No actionable MINOR findings.

VERDICT: APPROVE
