- **severity: MAJOR**  
  **location:** `docs/superpowers/specs/2026-08-04-agent-tui-enhancement-bubbles-v2-design.md:110-115`  
  **evidence:** The design says `Init` starts spinner ticks but the initial model has no pending turn. If the initial tick is ignored while non-pending, no subsequent tick command is scheduled. A turn submitted later can therefore display a static spinner indefinitely. The current asynchronous turn path begins only after Enter (`cmd/tu-tien-cli/tui.go:211-215`), so the pending transition needs to schedule the first spinner tick itself.  
  **required change:** Define the spinner command lifecycle explicitly: start/schedule `spinner.Tick` on the transition to pending, feed tick messages to the spinner only while pending, schedule its returned command while pending, and stop scheduling it on completion/error. Add a behavior test proving spinner ticks continue after a turn submitted after startup and do not create an additional `HandleTurn` call.

- **severity: MAJOR**  
  **location:** `docs/superpowers/specs/2026-08-04-agent-tui-enhancement-bubbles-v2-design.md:79-84, 96-106, 194-195`  
  **evidence:** The spec requires the action area to remain visible on very short terminals, but it does not give `textarea.Model` a bounded allocated height or define its scrolling behavior. A multiline textarea may grow with wrapped/pasted content and consume the height reserved for the footer or viewport. “Reserve … textarea … height” is not implementable deterministically without a maximum/minimum textarea height and a short-terminal allocation policy.  
  **required change:** Specify exact dimension allocation rules, including textarea minimum/maximum visible rows, textarea internal scroll behavior after its maximum, and deterministic degradation when the terminal cannot fit all persistent regions. Extend resize tests to use a long multiline/pasted draft on a short/narrow terminal and assert that the editor, recovery/error notice when present, and contextual footer remain visible.

- **severity: MAJOR**  
  **location:** `docs/superpowers/specs/2026-08-04-agent-tui-enhancement-bubbles-v2-design.md:103-106, 192-193`  
  **evidence:** “On failure” is not defined for an invalid completion from the sole gameplay boundary. `orchestrator.GameSession` permits `(*game.TurnResult, nil)` (`internal/orchestrator/session.go:22-25`), and the current TUI explicitly has a `nil`-result branch that only shows `lượt chơi không có kết quả` after clearing the draft (`cmd/tu-tien-cli/tui.go:247-263`). That leaves no recovery draft and no stated commitment status. Treating it as safely retryable without defining whether the session applied state could also duplicate a turn.  
  **required change:** Define the completion-validity contract and state transition for `nil result, nil error` (and any malformed/unrenderable result): whether it is a recoverable pre-application failure or a terminal/ambiguous session error, what the player sees, whether the exact draft is restored, and whether retry is permitted. Add fake-session tests for that outcome, including the required no-duplicate/no-unacknowledged-state behavior.

VERDICT: NEEDS_CHANGES
