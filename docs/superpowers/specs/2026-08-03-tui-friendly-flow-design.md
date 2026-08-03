# TUI Friendly Flow Design

## Goal

Make the Bubble Tea TUI comfortable for both first-time and returning players while preserving free-text roleplay as the primary interaction. The interface should make the next action obvious, keep the current turn understandable, and provide a safe recovery path when an LLM request fails.

## Scope

- Improve the first-run, repeated-turn, command, status, inventory, loading, validation, and error flows in `cmd/tu-tien-cli`.
- Keep Bubble Tea code within the CLI adapter. The game engine and orchestrator boundaries remain unchanged.
- Keep free-text input as the primary path. Suggestions, numeric selection, and commands are accelerators.

## Non-Goals

- Change game rules, LLM orchestration, persistence, or campaign content.
- Add a graphical or web interface.
- Add an offline runtime mode.
- Add save/load management beyond communicating the existing session state.

## Design Principles

- The current decision is the visual focus: show the scene result, then make the next action clear.
- Players should recognize available actions without remembering commands or number-selection rules.
- A failed request must never lose the player's action or imply that state changed.
- Internal identifiers must not appear in player-facing UI.
- The layout must preserve the input and current choices in short or narrow terminals.

## Screen Structure

The screen has four persistent regions, ordered by game flow rather than implementation details:

1. A compact header with the game title, player-facing scene name, and current turn number.
2. A compact player summary with realm name, stage, HP, spiritual energy, and inventory count.
3. A scrollable turn history, with the latest complete turn visible above the action area.
4. An action area containing contextual suggestions, the free-text input, and contextual keyboard help.

On wide terminals, the player summary appears beside the turn history. On narrow terminals, it appears under the header before the history. The action area remains at the bottom in both layouts.

The active provider/model is not part of the primary header. It may appear in `/help` or a secondary diagnostic view because it does not help a player decide what to do next.

## First-Run Flow

The first screen establishes agency before asking for input:

```txt
Tutine TRPG | Ngoai mon Thanh Van Tong | Luot 01
Luyen Khi tang 1 | HP 30/30 | Linh luc 20/20 | Tui do 0 mon

Ban dung truoc cong son mon, may phu lung chung nui.

Bat dau:
  1. Quan sat cong mon
  2. Hoi de tu gac cong
  3. Kiem tra trang thai

Ban muon lam gi? Vi du: ta quan sat cong mon
> 
Enter gui | Tab chon goi y | / lenh | Esc thoat
```

- The initial suggestions are scene-appropriate and limited to three.
- The player may type any natural-language action immediately.
- The prompt includes one concise example, so reading external documentation is not required to begin.
- A command-like suggestion such as `Kiem tra trang thai` opens local information without consuming a game turn.

## Turn Flow

Each resolved turn is rendered as one distinct block:

```txt
Luot 02
Ban: ta quan sat cong mon

Gio nui luot qua. Mot de tu ao xanh dang ghi ten nguoi moi...

Thay doi: Linh luc +0
Tiep theo:
  1. Hoi de tu gac cong
  2. Quan sat bang cao thi
  3. Kiem tra trang thai
```

- The block order is player action, narration, resolved state changes, warnings, then suggested next actions.
- State changes use player-facing labels, such as `Linh luc`, instead of effect IDs such as `energy_delta`.
- Warnings remain in the current turn block and are labelled clearly.
- The action area appears after the latest suggestions and states the available input methods: free text, `Tab`, and numeric choice.
- Entering a number in the valid suggestion range selects that suggestion. A number outside the valid range is rejected locally with `Chon tu 1 den N, hoac nhap hanh dong bang chu.` It must not be submitted as a roleplay turn.

## Commands And Information Views

Typing `/` opens a concise command palette near the input:

```txt
Lenh:
  /status     Xem trang thai nhan vat
  /inventory  Xem tui do
  /help       Xem huong dan choi
  /exit       Thoat game
```

- The palette supports the existing commands and does not introduce new game behavior.
- `/status` and `/inventory` render as temporary information views in the main content area, rather than permanent transcript entries.
- `Esc` closes a temporary information view and returns focus to the input with the draft text preserved.
- `/help` explains free-text actions, numeric choices, `Tab`, and commands in short player-facing language.

## Loading And Error Recovery

When a roleplay turn is submitted:

- The submitted action is displayed in a pending turn block.
- The input becomes read-only and the action area displays `Dang xu ly luot choi...`.
- Duplicate submission is prevented without adding repeated log messages.

When the turn succeeds:

- The pending block becomes a resolved turn block.
- The input clears and receives focus for the next action.

When the turn fails:

```txt
Khong the hoan tat luot nay. Hanh dong chua duoc ap dung.
Kiem tra ket noi hoac thu lai.

> ta hoi de tu gac cong ve ky khao hach
Enter thu lai | Sua noi dung truoc khi gui | Esc huy
```

- The original action is restored to the input and remains editable.
- The message confirms that no game state was applied because a failed turn must not leave the player guessing.
- Provider details are not shown unless the error is actionable. Configuration errors identify the missing environment variable or configuration field; transient errors recommend retrying.
- `Esc` cancels the retained draft without exiting the application while an error recovery state is active.

## Responsive And History Behavior

- A viewport owns the turn history and reserves height for the header, summary, suggestions, input, and footer.
- History is clipped by rendered line count rather than number of log entries.
- When older history is hidden, the UI shows a concise indicator such as `Lich su cu hon dang an`.
- The latest resolved turn and active input remain visible after terminal resizing.
- Colour supplements, not replaces, labels for loading, warnings, errors, and state changes.

## Data Mapping

The CLI owns a small player-facing mapping layer for current known IDs:

- Scene IDs map to campaign scene names.
- Realm IDs map to Vietnamese realm names.
- Effect types map to concise state-change labels.
- Inventory count is derived from the authoritative save inventory and displayed in the summary.

Unknown IDs fall back to a readable generic label rather than exposing a raw internal identifier.

## Implementation Boundaries

- Extend `tuiModel` with explicit presentation state for the input draft, selected suggestion, pending turn, temporary view, and recoverable error.
- Keep `orchestrator.GameSession` as the only gameplay boundary used by the TUI.
- Do not move rendering or Bubble Tea messages into `internal/game`, `internal/llm`, `internal/memory`, or `internal/orchestrator`.
- Continue to obtain authoritative status and inventory data from `session.Save()`.

## Testing

Use fake sessions only. Add focused TUI tests for:

- Initial screen shows an example action and no more than three initial suggestions.
- Valid numeric choices route to their matching suggested action.
- Invalid numeric choices do not call `HandleTurn` and render an actionable range message.
- Command-like suggestions run locally without consuming a turn.
- `/status` and `/inventory` open and close temporary views while preserving an existing draft.
- A pending turn blocks duplicate submission and communicates loading in player-facing language.
- A failed turn restores the submitted action, confirms no application, and allows retry.
- Player-facing mappings do not render raw scene, realm, or effect IDs.
- Narrow layouts keep the action area visible and show a hidden-history indicator when necessary.

Run `gofmt` on changed Go files and `go test ./...` after implementation.
