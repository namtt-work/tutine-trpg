- **severity:** MAJOR  
  **location:** `cmd/tu-tien-cli/tui_test.go:153-190`; approved spec `§ Testing And Verification`, lines 101-106  
  **evidence:** The specification requires Esc to close a temporary view *before* cancelling a recoverable draft. The tests exercise those states independently: `TestTUITempViewOpenCloseKeepsExistingDraft` has a temporary view but no recoverable draft, while `TestTUIKeyPressEscCancelsRecoverableDraft` has a recoverable draft but no temporary view. Therefore, a regression that reverses `handleEsc`’s precedence would still pass the suite.  
  **required change:** Add a `tea.KeyPressMsg` test with both `tempView != tempViewNone` and `recoverable == true`, then assert Esc only closes the temporary view, emits no quit command, and preserves the recoverable draft and notice.

- **severity:** MAJOR  
  **location:** approved spec `§ Testing And Verification`, lines 115-121; `cmd/tu-tien-cli/tui.go:103-108,278-301`  
  **evidence:** The supplied verification evidence covers formatting, automated tests, diff checking, and import searches; it does not include either required manual smoke test: missing-key startup validation or a configured terminal session that enters the alternate screen and restores the invoking terminal on exit. `TestTUIViewUsesAlternateScreen` verifies the model’s `tea.View.AltScreen` field, but does not verify Bubble Tea program lifecycle behavior in a real terminal.  
  **required change:** Perform and record the two required manual smoke checks without exposing secrets: (1) start with an intentionally missing API key and confirm validation fails without leaking secret material; (2) run a configured development session, verify alternate-screen entry and terminal restoration after exit.

VERDICT: NEEDS_CHANGES
