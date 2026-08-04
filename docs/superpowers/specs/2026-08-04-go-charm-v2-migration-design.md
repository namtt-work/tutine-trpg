# Go And Charm v2 Migration Design

## Goal

Upgrade Tutine TRPG's Go toolchain declaration and existing terminal UI stack to current stable releases without changing player-visible TUI behavior or crossing the CLI adapter boundary.

## Scope

- Raise the module Go version from `1.24.0` to `1.26.5`.
- Replace Bubble Tea v1 with `charm.land/bubbletea/v2 v2.0.8`.
- Replace Lip Gloss v1 with `charm.land/lipgloss/v2 v2.0.5`.
- Convert the existing Bubble Tea adapter and its tests to the v2 APIs.
- Update user-facing Go-version documentation.

## Non-Goals

- Do not add Bubbles as an unused dependency.
- Do not change layout, gameplay commands, persistence, orchestrator behavior, save semantics, or LLM calls.
- Do not add textarea, viewport, spinner, command-list, mouse, or streaming behavior. Those belong to the follow-up enhancement design.
- Do not change `configs/llm.yaml` or other local runtime configuration.

## Rationale

The current UI stack was added with `github.com/charmbracelet/bubbletea v1.3.10` and `github.com/charmbracelet/lipgloss v1.1.0`. The game architecture already keeps all Bubble Tea code in `cmd/tu-tien-cli`, so this breaking API migration is isolated from the game engine, LLM, memory, storage, and orchestration packages.

Current stable Go is `1.26.5`; the installed repository toolchain is `1.24.0`. Current Bubbles v2 requires at least Go 1.25, so the module needs a toolchain upgrade before the subsequent enhancement can use Bubbles v2.

## Dependency Target

`go.mod` will contain direct requirements equivalent to:

```text
go 1.26.5

charm.land/bubbletea/v2 v2.0.8
charm.land/lipgloss/v2 v2.0.5
```

The exact indirect dependency set is owned by `go mod tidy`; it must not retain legacy Bubble Tea or Lip Gloss modules after the imports have migrated.

The project does not need a `toolchain` directive unless maintainers specifically want to pin a downloaded toolchain. The `go 1.26.5` directive is the compatibility contract. Developers and CI must install Go 1.26.5 or use Go's automatic toolchain switching.

## API Migration

### Imports

Replace only the existing imports in `cmd/tu-tien-cli`:

```text
github.com/charmbracelet/bubbletea
  -> charm.land/bubbletea/v2

github.com/charmbracelet/lipgloss
  -> charm.land/lipgloss/v2
```

No `internal/*` package may import any Charm package.

### Keyboard Events

Bubble Tea v2 replaces `tea.KeyMsg` with `tea.KeyPressMsg`.

- `Update` handles `tea.KeyPressMsg`.
- Existing semantic controls remain unchanged: Ctrl+C quit, Esc contextual close/cancel/quit, Enter submit, Tab choose a suggestion, Backspace edit, printable text appends to draft.
- The migration must use v2 key data (`msg.String()` or `msg.Key().Code` / `msg.Key().Text`) rather than legacy `msg.Type` and `tea.KeyRunes`.
- Tests construct `tea.KeyPressMsg` events and retain coverage for all current interaction guarantees.

### View And Alternate Screen

Bubble Tea v2 models return `tea.View`, not a raw string.

- `tuiModel.View()` returns `tea.View`.
- Rendering remains built from the same current strings and Lip Gloss styles.
- The adapter sets `view.AltScreen = true` on the returned `tea.View` rather than passing `tea.WithAltScreen()` to `tea.NewProgram`.
- Tests inspect `model.View().Content` when asserting player-facing rendering.

## Behavior Preservation

The following behavior must be byte-for-byte equivalent where terminal rendering APIs permit it:

- Header, responsive summary placement, history clipping, suggestions, input prompt, command palette, and footer wording.
- Numeric suggestion selection and local execution of command-like suggestions.
- `/status`, `/inventory`, `/save`, `/help`, and `/exit` behavior.
- Pending-turn duplicate prevention.
- Friendly error without provider error leakage; restored editable draft after failure.
- No raw internal IDs in player-facing views.

No UI polish is intentionally introduced in this migration. A failed preservation test is a migration regression, not an opportunity to redesign the interface.

## Files

- Modify: `go.mod`
- Modify: `go.sum`
- Modify: `cmd/tu-tien-cli/tui.go`
- Modify: `cmd/tu-tien-cli/tui_test.go`
- Modify: `docs/cli-guide.md` to raise the Go requirement from 1.24 to 1.26.5.

## Testing And Verification

- Update existing tests before changing each corresponding v1 API call, then observe the expected compile/test failure under v2.
- Add and preserve the following v2 `tea.KeyPressMsg` behavior matrix. Each test asserts model state and emitted command, not only rendered content:
  - Ctrl+C returns `tea.Quit`.
  - Esc closes a temporary view before doing anything else; after a recoverable error it cancels the retained draft without quitting; only an otherwise idle screen returns `tea.Quit`.
  - Tab advances the selected suggestion and replaces the editable draft with that suggestion.
  - Backspace removes exactly one rune from a Vietnamese/multibyte draft and clears a transient notice.
  - Printable Vietnamese text appends to the draft exactly once per `KeyPressMsg`.
  - Enter submits a non-empty draft once and a second Enter while pending returns no additional turn command.
- `tuiModel.View()` returns the expected `tea.View.Content` and sets `AltScreen == true`.
- Run `gofmt` on changed Go files.
- Run `go mod tidy` with Go 1.26.5.
- Confirm no visible Go source imports legacy `github.com/charmbracelet/bubbletea` or `github.com/charmbracelet/lipgloss`.
- Run `go test ./cmd/tu-tien-cli` after each migration slice.
- Run `go test ./...` under Go 1.26.5 before completion.
- Run `git diff --check`.
- Manually start the TUI with an intentionally missing API key to confirm normal startup validation still fails without a secret leak; do not use an actual key for this check.
- Manually start a configured development session and confirm the game uses the alternate screen, then returns to the invoking terminal screen on exit.

## Risks

- Go 1.26.5 may expose unrelated compilation or module-resolution issues. These must be handled in the migration change, but unrelated product refactors remain out of scope.
- Existing terminal emulators can encode special keys differently. Tests cover v2 message handling; a smoke test must also verify Enter, Tab, Esc, backspace, and Vietnamese text on a real UTF-8 terminal.
- Bubbles v2 must not be added until the enhancement phase imports it, because `go mod tidy` will correctly remove unused packages.

## Review Log

- Round 1, `self`: NEEDS_CHANGES — added a concrete v2 keyboard-event acceptance matrix covering Ctrl+C, Esc precedence, Tab, rune-safe backspace, Vietnamese input, Enter, and pending duplicate prevention. Evidence: `docs/reviews/go-charm-v2-migration/spec-round-1-self.md`.
- Round 2, `self`: NEEDS_CHANGES — added direct `tea.View.AltScreen` test coverage and alternate-screen manual smoke verification. Evidence: `docs/reviews/go-charm-v2-migration/spec-round-2-self.md`.
- Round 3, `self`: APPROVE — no remaining blocker or major finding. Evidence: `docs/reviews/go-charm-v2-migration/spec-round-3-self.md`.
