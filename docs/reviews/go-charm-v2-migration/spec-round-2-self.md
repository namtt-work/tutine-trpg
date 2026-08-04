- **severity:** MAJOR  
  **location:** `docs/superpowers/specs/2026-08-04-go-charm-v2-migration-design.md:68-75, 98-114`  
  **evidence:** The current CLI enters full-window alternate-screen mode through `tea.NewProgram(model, tea.WithAltScreen())` in `cmd/tu-tien-cli/tui.go:103-108`. Under Bubble Tea v2, the spec correctly moves that behavior to `tuiModel.View()` via `tea.View.AltScreen`, but the verification plan neither requires an assertion that the returned view has `AltScreen == true` nor includes it in the manual smoke-test criteria. The current TUI tests have no alternate-screen assertion. Consequently, an implementation that returns the correct `View.Content` but omits `AltScreen` would compile and pass the listed keyboard/render checks while visibly regressing the full-screen TUI behavior.  
  **required change:** Add a v2 acceptance test that obtains `model.View()` and asserts both the expected `.Content` and `AltScreen == true`; include alternate-screen operation in the required manual smoke verification.

VERDICT: NEEDS_CHANGES
