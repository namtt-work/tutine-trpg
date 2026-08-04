# Agent-Quality TUI Enhancement With Bubbles v2 Design

## Goal

After the Go and Charm v2 migration is complete, make the terminal UI feel like a mature AI-agent console while preserving free-text roleplay as the primary interaction and keeping the game engine authoritative.

## Prerequisite

`docs/superpowers/specs/2026-08-04-go-charm-v2-migration-design.md` must be fully implemented and verified first. This design assumes:

```text
Go 1.26.5
charm.land/bubbletea/v2
charm.land/lipgloss/v2
```

This phase adds `charm.land/bubbles/v2 v2.1.1` because the UI will use its components directly.

## Product Principles

- The latest resolved game decision and the next action are more important than technical status.
- Free-text roleplay is always available; suggestions, commands, and history are accelerators.
- Players retain drafts and can see what is happening while an LLM turn is pending.
- Transcript history is navigable rather than destructively clipped.
- Color is supplemental: loading, warning, error, and state changes all retain text labels.
- TUI code remains inside `cmd/tu-tien-cli`; `orchestrator.GameSession` stays the sole gameplay boundary.
- Game state, combat, inventory, quests, rewards, NPC relationships, and persistence stay authoritative outside the UI.

## Non-Goals

- Do not add web UI, bot adapter, multiplayer, streaming narration, or model/provider switching.
- Do not change rule-engine validation, LLM prompt contracts, save format, memory retrieval, or campaign data.
- Do not expose debug data, raw save IDs, filesystem paths, or raw engine IDs in player-facing views.
- Do not add a tactical combat grid.

## Component Model

The root `tuiModel` coordinates focused presentation components rather than manually storing all terminal primitives as strings:

```text
transcript viewport    bubbles/v2/viewport
input editor           bubbles/v2/textarea
loading indicator      bubbles/v2/spinner
keyboard help          bubbles/v2/help and bubbles/v2/key
command palette        bubbles/v2/list
```

The existing presentation-only fields remain conceptually valid: turn blocks, suggestions, pending turn, recovery state, temporary view, terminal width, and height. The model also owns a focus/mode state so a key event is routed to exactly one active component at a time.

Suggested file split, performed only when it improves readability:

```text
cmd/tu-tien-cli/tui.go           root model, program startup, Update routing
cmd/tu-tien-cli/tui_layout.go    responsive rendering and dimension allocation
cmd/tu-tien-cli/tui_transcript.go viewport content and following-latest policy
cmd/tu-tien-cli/tui_input.go     textarea, suggestion insertion, action history
cmd/tu-tien-cli/tui_commands.go  command palette and temporary information views
cmd/tu-tien-cli/tui_keymap.go    contextual bindings and compact help
cmd/tu-tien-cli/tui_test.go      behavior-level tests with fake sessions
```

A single file is acceptable for the first component slice; do not split merely for ceremony.

## Screen Regions

### Persistent Game Screen

The regular game screen has four regions, ordered by player decision flow:

1. Header: game title, player-facing scene name, current turn, and optional compact pending indicator.
2. Player summary: realm, stage, HP, spiritual energy, inventory count; on wide terminals it is a right rail and on narrow terminals it appears under the header.
3. Scrollable transcript: scene intro, resolved turn blocks, pending player action, warnings, and state-change summaries.
4. Action area: suggestions, notice, multiline editor, and contextual keyboard help.

The active model/provider stays out of the primary header. It remains visible in `/help`, matching the existing player-focused design.

### Responsive Rules

- Wide layout: summary rail beside transcript.
- Narrow layout: header, summary, transcript, action area in one column.
- The action area must always remain visible for supported terminal heights of eight rows or more.
- The textarea has one visible row minimum and three visible rows maximum. It scrolls internally once its wrapped or pasted draft exceeds its allocated rows.
- At fourteen rows or more, render normal header, summary, suggestions, notice, one-to-three-row editor, footer, and allocate all remaining rows to the transcript viewport.
- At eight through thirteen rows, render a compact one-line summary, flatten suggestions to one line, force the editor to one row, remove inter-region blank lines, retain notice and footer, and allocate any remaining rows to the viewport.
- Below eight rows, render a compact terminal-too-short screen with the one-row editor and footer; do not claim that transcript, summary, or notices are simultaneously visible. The player must resize before a pending/recovery notice can be safely acted on.

## Transcript Viewport

Replace destructive `clipHistory` rendering with a Bubbles viewport.

- Content is regenerated from the current turn blocks and scene intro using the current viewport width before being assigned to the component.
- On new turn submission, append the pending action and follow the bottom.
- On a resolved turn or error, follow the bottom only if the player was already following the transcript.
- If the player scrolls upward and new content arrives, do not forcibly jump. Display a concise `↓ Có lượt mới` indicator near the action area; End or the indicator returns to the latest turn.
- Page Up/Page Down and standard viewport keys scroll history. The transcript must be keyboard-operable without a mouse.
- The old `Lịch sử cũ hơn đang ẩn.` line is removed once the viewport owns history; history is no longer discarded from the screen model.

## Multiline Input

Replace manual rune and backspace handling with `textarea.Model`.

- Input supports Unicode, paste, cursor movement, soft wrapping, and editing of multi-sentence actions.
- Placeholder: `Bạn muốn làm gì?` with the existing concise first-turn example displayed outside the focused editor.
- Enter submits a non-empty action.
- Shift+Enter inserts a newline. If terminal key reporting cannot distinguish Shift+Enter, document the fallback binding chosen during implementation.
- While a turn is pending, input becomes non-editable but the pending action remains visible in the transcript.
- On failure, restore the exact submitted text to the editor, focus it, and retain editability.
- Tab with no palette open cycles suggestions and replaces editor content exactly as the existing UI does; editing afterward remains possible.

## Loading State

Use `spinner.Model` only when a turn is pending.

- Do not start spinner ticks from `Init`, because the initial model has no pending turn.
- On the transition from editable input to `pending`, return the turn command together with the first `spinner.Tick` command.
- When a `spinner.TickMsg` arrives while pending, update the spinner and return its next tick command. Ignore stale tick messages once completion clears pending state.
- On a successful result, returned error, or invalid completion, clear pending state and schedule no further spinner tick.
- The action area shows a visible spinner and `Đang xử lý lượt chơi...`.
- This phase must not invent false planner/narrator/extractor phases. Granular phase text is deferred until `GameSession` emits truthful progress events.
- Duplicate Enter and other submit attempts while pending cause no extra command or transcript noise.

### Completion Validity

`GameSession.HandleTurn` returning `nil, nil` is a protocol violation. The concrete `orchestrator.Session` returns a non-nil `TurnResult` on every nil-error path, but the interface permits a fake or future implementation to violate that contract after an unknown point in turn application.

- Treat `nil, nil` and any result that cannot be rendered as an **ambiguous completion**, not as an ordinary recoverable failure.
- Clear pending and stop the spinner; do not restore the draft and do not offer retry, because resubmission could duplicate a turn whose state may already have changed.
- Enter a distinct ambiguous-completion mode. It disables textarea editing, suggestion selection, numeric choice resolution, and all `HandleTurn` submission for the rest of the process. It permits only non-game exit controls (`/exit`, Ctrl+C, and Esc when it maps to exit) so the player can reopen/restart the session.
- Render a player-facing message that the turn could not be confirmed and asks the player to restart/reopen the session before acting again, with contextual help that says input is locked. Keep raw implementation detail in the debug log only.
- Regular returned errors remain recoverable only under the existing session contract that an error means the turn was not applied; the exact draft is restored and editable.

## Command Palette And Temporary Views

### Palette

Typing `/` opens a `list.Model` command palette above the editor. It filters command names and descriptions and presents only commands the player can use:

```text
/status      Xem trạng thái nhân vật
/inventory   Xem túi đồ
/save        Xem tiến trình đã lưu
/help        Xem hướng dẫn chơi
/exit        Thoát game
```

- Enter chooses the highlighted command.
- Esc closes the palette and restores the unchanged draft.
- Typing a non-command action closes the palette and returns focus to the editor.
- The palette does not introduce commands whose data/action semantics do not exist yet; `/quests`, `/relations`, and `/memories` are deferred.

### Temporary Views

`/status`, `/inventory`, `/save`, and `/help` remain temporary player-facing views. They can be rendered as an in-content panel above the action area; a modal overlay is intentionally deferred until focus and accessibility behavior are proven stable.

- Esc closes the temporary view and restores the editor draft and focus.
- Temporary views never append a fake turn to the transcript.
- `/save` retains the existing policy: player sees confirmation and turn number, never raw save ID or path.

## Keymap And Help

Define semantic key bindings once using Bubbles key/help components. The compact footer shows only actions valid in the current mode.

Regular mode:

```text
Enter gửi · Shift+Enter xuống dòng · Tab gợi ý · / lệnh · PgUp/PgDn lịch sử · Esc thoát
```

Pending mode:

```text
Đang xử lý lượt chơi… · PgUp/PgDn lịch sử
```

Recovery mode:

```text
Enter thử lại · sửa nội dung trước khi gửi · Esc huỷ
```

Palette or temporary-view mode:

```text
Enter chọn · ↑/↓ di chuyển · Esc quay lại
```

The complete help view must describe free text, numeric selection, suggestions, multiline editing, transcript scrolling, command palette, and Esc precedence.

## Action History

This phase intentionally does not persist player action history. A per-process in-memory history is allowed only if it can be introduced without affecting saves or game memory:

- Up/Down retrieves previously submitted roleplay actions only when the editor is on its boundary and no component has higher key priority.
- Do not record local commands or duplicate consecutive actions.
- Ctrl+R reverse search is deferred; it needs a focused search interaction design and persistence policy.

If input-history routing complicates textarea focus semantics, omit it from the first enhancement implementation. Viewport, textarea, spinner, and palette are higher-value requirements.

## Testing

All tests use fake sessions; they never call a real provider.

New behavior coverage:

- The v2 textarea accepts Vietnamese printable text and a pasted/multiline draft without manual rune management.
- Enter submits a non-empty draft once; Shift+Enter preserves the draft as multiline text.
- The input stays disabled while pending, spinner ticks do not create duplicate requests, and a completed turn re-enables/focuses it.
- A turn submitted after startup receives a first spinner tick; successive pending ticks animate it, and ticks never produce an extra `HandleTurn` call.
- A failed turn restores the exact editable draft.
- A `nil, nil` or unrenderable completion is an ambiguous completion: it stops the spinner, logs diagnostic detail, offers no retry, restores no draft, and leaves no unacknowledged player action rendered as resolved.
- After ambiguous completion, printable input, suggestions, numeric choices, and Enter cannot call `HandleTurn`; only the declared exit controls remain available until process restart.
- Viewport height is recomputed on resize. Long multiline/pasted drafts on narrow/short supported terminals keep the bounded editor, recovery/error notice when applicable, and contextual footer visible; excess editor content scrolls internally.
- The transcript follows new content at the bottom, but preserves manual scroll position and displays the unseen-turn indicator when scrolled upward.
- Command palette filtering, selection, and Esc preserve draft text.
- Temporary views preserve draft and never submit a game turn.
- Key help changes by regular, pending, recovery, and palette/view mode.
- No player-facing view leaks raw scene, realm, effect, or save identifiers.

Keep all existing TUI behavior tests unless a test explicitly asserts the old clipped-history implementation. Replace that assertion with viewport behavior coverage rather than deleting it without a successor.

## Verification

- Add Bubbles v2 only when its first component import is added, then run `go mod tidy`.
- Run `gofmt` on all changed Go files.
- Run focused TUI tests after each interaction slice.
- Run `go test ./...` under Go 1.26.5.
- Run `git diff --check`.
- Manually exercise a real UTF-8 terminal with Vietnamese text: submit, multiline edit, Tab suggestion, command palette, transcript scroll, temporary view, failed provider retry, and resize.

## Delivery Sequence

1. Add textarea and preserve submit/recovery behavior.
2. Replace history clipping with viewport and responsive allocation.
3. Add spinner-backed pending rendering.
4. Introduce semantic keymap/help.
5. Replace static slash hint with command palette.
6. Perform a terminal smoke test after all behavior tests pass.

Each slice is independently test-first and must preserve the game-session boundary.

## Review Log

- Round 1, `self`: NEEDS_CHANGES — specified pending-transition spinner lifecycle, bounded textarea allocation and short-terminal degradation, and the non-retryable ambiguous-completion contract. Evidence: `docs/reviews/agent-tui-enhancement-bubbles-v2/spec-round-1-self.md`.
- Round 2, `self`: NEEDS_CHANGES — added an explicit locked ambiguous-completion mode and post-ambiguity input/submission prevention coverage. Evidence: `docs/reviews/agent-tui-enhancement-bubbles-v2/spec-round-2-self.md`.
- Round 3, `self`: APPROVE — no remaining blocker or major finding. Evidence: `docs/reviews/agent-tui-enhancement-bubbles-v2/spec-round-3-self.md`.
