# Agent Console Visual Layout Design for the TUI

## Goal

Turn Tutine into a terminal interface with clear visual hierarchy: players should immediately distinguish the current scene, character status, journey transcript, and action composer. This is presentation-only work; it does not change game rules, LLM orchestration, saves, memory, or commands.

### Compatibility impact

This visual shell raises the minimum fully interactive terminal height from eight rows to ten rows. At fewer than ten rows, the UI renders only its resize-required fallback. This intentionally replaces the earlier eight-row responsive guarantee because the framed, state-safe composer cannot truthfully preserve the required controls in eight or nine rows.

## Visual Direction

- Tone: a calm xianxia field journal, not a cyberpunk telemetry dashboard.
- One outer frame unifies the screen into a single game surface.
- Each region has a short uppercase title; color supplements loading, warning, and error labels.
- The composer has a dedicated separator/border from the transcript, making it feel anchored at the bottom.

## Wide Layout: 100x30 and above

```text
+-- TUTINE TRPG -------- Ngoại môn Thanh Vân Tông · Lượt 01 --------+
| Cảnh: Cổng môn phủ mây                                             |
+-----------------------------------------------+-------------------+
| NHẬT KÝ HÀNH TRÌNH                           | NHÂN VẬT          |
|                                               | Luyện Khí · tầng 1|
| Bạn đứng trước cổng môn, mây phủ lưng núi.   | HP        30 / 30 |
|                                               | Linh lực 20 / 20 |
|                                               | Túi đồ       0 món|
+-----------------------------------------------+-------------------+
| BẠN MUỐN LÀM GÌ?                                                  |
| 1. Quan sát cổng môn                                              |
| 2. Hỏi đệ tử gác cổng                                             |
| 3. Kiểm tra trạng thái                                            |
| > Bạn muốn làm gì?                                                |
| Enter gửi · Shift+Enter xuống dòng · Tab gợi ý · / lệnh · Esc thoát|
+-------------------------------------------------------------------+
```

- The character rail stays 26-30 cells wide; the transcript receives the remaining width.
- Suggestions always occupy three independent rows rather than being joined into one long line.

## Narrow Layout: 60x18

```text
+-- TUTINE TRPG · Lượt 01 ---------------------------------+
| Cổng môn phủ mây                                         |
+-- NHÂN VẬT -----------------------------------------------+
| Luyện Khí 1 · HP 30/30 · Linh lực 20/20 · Túi 0          |
+-- NHẬT KÝ HÀNH TRÌNH -------------------------------------+
| Bạn đứng trước cổng môn, mây phủ lưng núi.               |
+-- HÀNH ĐỘNG ----------------------------------------------+
| 1. Quan sát cổng môn                                     |
| 2. Hỏi đệ tử gác cổng                                    |
| 3. Kiểm tra trạng thái                                   |
| > Bạn muốn làm gì?                                       |
| Enter gửi · Tab gợi ý · / lệnh · Esc thoát               |
+----------------------------------------------------------+
```

- The rail becomes a one-line summary. It is independently ellipsized to its available width and never consumes a second row or composer height.
- The transcript is the only flexible-height region; all other regions have an explicit height budget.
- At 14 rows or more, all three suggestions render on separate rows. Each row may be horizontally ellipsized, but always retains its number, for example `2. Hỏi đệ tử gác...`.
- Pressing Tab inserts the complete suggestion into the editor; render-time ellipsis never discards data.
- If the product later provides more than three suggestions, show only the first three and state the available range in footer help. This is an explicit product limit, not silent clipping.

## Short Terminals and Long Drafts

- At 14 rows or more, show three separate suggestions and allocate one to three visible textarea rows. Long drafts soft-wrap through that budget, then scroll internally without pushing the footer off-screen.
- At 10-13 rows, enter compact-composer mode. Render only the selected suggestion on one row, for example `1. Quan sát cổng môn · Tab 1/3`; Tab changes the selection. Do not render all three rows because they would consume the transcript and editor area.
- At 9 rows and below, render only the resize-required fallback. It contains no editor, footer controls, palette, state-specific action area, or temporary-view content.
- Submitted actions, pending actions, and narration belong to the transcript and may wrap freely because the viewport owns scrolling.
- The current draft is the only content allowed to occupy textarea height. Its complete Unicode and pasted text must always be retained.

## Deterministic Row Budgets and State Degradation

The renderer calculates region heights before assigning the remaining rows to the viewport. It must not derive viewport height from an unbounded rendered string.

### 60x18 normal narrow screen

At 18 rows or more, use the full framed layout: outer frame (2), title/scene header (2), summary section (2), transcript label (1), viewport (minimum 3), composer label (1), three one-line suggestions (3), editor (1-3), footer (1), and an optional one-line notice. The editor receives more than one row only after reserving the three-row viewport. The initial example is omitted because the suggestions already communicate the first action.

At 14-17 rows, use a condensed normal layout without the outer frame, section labels, or blank separators: one-line header, one-line summary, viewport (minimum 2), three one-line suggestions, optional notice (1), editor (1-3), and footer (1). The editor grows only from rows left after these reservations. This range retains all three distinct suggestions but does not promise the decorative framed shell.

### Compact layouts: 60x10 and boundary heights

At 10-13 rows, use compact mode without an outer frame or section labels: one-line header, one-line summary, viewport (minimum 1), selected suggestion (1), notice or state line (1), one-row editor or state control (1), and footer (1). When no notice exists, its reserved row becomes viewport height. Unseen text is appended to the footer in compact mode rather than consuming a dedicated separator row.

Below ten rows, render the resize-required fallback only. Do not render a partial game screen, palette, pending state, recovery editor, locked-mode controls, or temporary view because their required controls cannot fit safely.

### Fixed-region overflow policy

Every fixed-height region uses Unicode cell-width-aware truncation with a single trailing ellipsis (`…`) and never soft-wraps. This policy applies to the title/scene header, compact summary, suggestions, notices, pending and locked status, palette descriptions, footer/help, and temporary-view return line.

- The full player-facing scene name remains available in the transcript's scene-intro content when the header is abbreviated.
- The full recovery or locked notice is retained as viewport content; the fixed composer/status line is only its short actionable summary.
- Palette command names never truncate; only their descriptions do. Filtering and selection continue to use complete names/descriptions held by `list.Model`.
- The footer switches to compact bindings before truncation, for example `Enter · Tab · / · Esc`.
- Temporary/help bodies are viewport content and may wrap there; only their fixed return line is truncated.

### State contracts at both target sizes

| State | 60x18 | 60x10 | Keyboard help |
| --- | --- | --- | --- |
| Normal | Transcript, three numbered suggestions, editor, footer. | Transcript, selected suggestion, one-row editor, footer. | `Enter`, `Tab`, `/`, `Esc`; show `PgUp/PgDn` only when room permits. |
| Palette | Palette replaces composer; filter row, visible choices, and selection help are bounded by composer budget. | Filter row, selected matching item, and `Enter`/`Esc` only; Up/Down changes selection. | `Enter chọn · ↑/↓ · Esc` at 18; `Enter · ↑/↓ · Esc` at compact heights. |
| Pending | Pending action remains in transcript; composer shows spinner/status and paging hint. | Pending action remains in viewport; one status line plus compact paging footer. | No submit help. |
| Recovery | Notice, restored editor, and retry/cancel footer; reduce editor before viewport. | Notice, one-row restored editor, retry/cancel footer; viewport stays at least 1 row. | `Enter thử lại · Esc huỷ`. |
| Locked | Ambiguity message occupies the state line; editor is absent; `/exit` palette is still reachable. | The short message is in the state line and full message in viewport; no editor or suggestion row. | `/exit · Esc thoát`. |
| Temporary view | Temporary body replaces transcript content in the same viewport; composer is one Esc-return line. | Same replacement with one viewport row and one Esc-return line. | `Esc quay lại`. |
| Unseen | Separator shows `↓ Có lượt mới · End để xem`. | Footer appends `End mới` when room permits; otherwise End remains functional but the indicator is omitted. | `End` only while unlocked. |

Long temporary/help content is always viewport content. It must never be emitted as an unbounded body above the composer.

## Suggestion Limit and Interaction Contract

The presentation layer caps `TurnResult.SuggestedActions` to the first three non-empty suggestions when the result is applied. `tuiModel.suggestions`, numeric selection, and Tab cycling operate only on that capped list. This creates one authoritative visible/selectable set, avoids inaccessible off-screen suggestions, and preserves the current product rule of at most three choices.

- At 60x18, render every capped suggestion as one independently numbered, horizontally ellipsized row.
- At 60x10, render only the current selected suggestion as `N. <ellipsized text> · Tab N/total`; Tab cycles within the capped list.
- Numeric input accepts only `1..len(m.suggestions)` at both sizes. A valid number selects the same full suggestion that Tab would insert; invalid numbers use the existing local validation notice.
- If the capped list is empty, render the existing free-text prompt instead of a suggestion row.

## Command Palette

```text
+-- LỆNH ---------------------------------------------------+
| > tìm lệnh...                                            |
|   /status     Xem trạng thái nhân vật                    |
|   /inventory  Xem túi đồ                                 |
|   /save       Xem tiến trình đã lưu                      |
|   /help       Hướng dẫn chơi                             |
|   /exit       Thoát game                                 |
| Enter chọn · ↑/↓ di chuyển · Esc quay lại                |
+----------------------------------------------------------+
```

The palette uses the composer region. It never covers the transcript or creates a modal overlay.

## Pending, Recovery, and Unseen State

- Pending title: `ĐANG XỬ LÝ LƯỢT`; show the spinner while the submitted action remains in the transcript.
- Recovery title: `THỬ LẠI HÀNH ĐỘNG`; restore the complete draft to the textarea and show a notice that no state change occurred.
- Place `↓ Có lượt mới · End để xem` in the separator immediately above the composer, not inside transcript content.

## Implementation Boundaries

- Keep all changes in `cmd/tu-tien-cli`.
- Continue using the existing Bubbles textarea, viewport, spinner, help/key, and list models.
- Add presentation-only Lip Gloss section styles and layout helpers. Do not add dependencies.
- Preserve the current authoritative game/session boundaries and every existing command.

## Acceptance Criteria

1. At 100x30 and 60x18, players can identify four persistent regions from section labels and borders alone.
2. At 60x18, all three capped suggestions render as separately numbered rows and ellipsize horizontally when necessary.
3. At 60x10, only the selected capped suggestion renders; the one-row editor, notice, and footer remain visible. At 14 and 17 rows, the documented condensed budget is honored. At 9 rows and below, only the resize-required fallback renders.
4. A 200-character Vietnamese draft does not exceed the composer height budget and remains fully editable and recoverable.
5. A long suggestion never wraps into a second composer row; its number and Tab affordance remain visible.
6. The transcript is the only flexible/scrolling region; palette, pending, recovery, locked, and temporary views preserve the same hierarchy.
7. Add layout tests for normal, palette, pending, recovery, locked, temporary, and unseen states at 60x18, 60x17, 60x14, and 60x10. Each test asserts total rendered height plus the required visible controls from the state table. Add tests that 60x9 and 60x8 render only the resize-required fallback.
8. Add tests with more than three long UTF-8 suggestions to prove capping, numeric selection, Tab cycling, and compact selected-suggestion rendering all use the same capped list.
9. Add long UTF-8 scene, notice, palette, and footer cases proving every fixed-height region remains one row; assert the full scene/notice/temp content is recoverable in the specified viewport location.
10. Perform a UTF-8 PTY/terminal smoke capture before declaring visual work complete.

## Incremental Implementation Plan

1. Add pure presentation helpers for Unicode cell-width truncation and fixed-height line rendering, with focused tests for Vietnamese text and ellipsis.
2. Add layout-mode calculation from width, height, and UI state. Test the row budget for 18, 17, 14, 10, 9, 8, and below-8 terminals before changing the main view.
3. Cap suggestions at turn-result ingestion; update numeric/Tab tests to prove all interaction uses that same list.
4. Render the wide, full narrow, condensed narrow, compact, and emergency compact shells while preserving the existing viewport/editor/list models.
5. Apply the state table to palette, pending, recovery, locked, temporary, and unseen views; add state-by-size regression tests.
6. Run formatter and the complete Go suite, then perform the required UTF-8 PTY smoke capture. No game engine, orchestration, persistence, or provider files are in scope.

## Open Assumptions

- Terminal width is measured in display cells by Lip Gloss-compatible helpers; tests treat ambiguous-width glyph behavior as owned by the terminal library.
- The existing `TurnResult.SuggestedActions` ordering is meaningful, so capping keeps the first three non-empty entries without reordering.
- The full player-facing scene/notice copy can be represented in transcript viewport content when a compact fixed region must abbreviate it.
- The product accepts a ten-row minimum fully interactive height in exchange for deterministic state-safe layout; users below that threshold must resize.

## Review Log

- Round 1, self: NEEDS_CHANGES. Added deterministic 60x18/60x10 row budgets, state-specific degradation and keyboard-help contracts, and an authoritative three-suggestion cap covering rendering, Tab, and numeric selection. Evidence: `docs/reviews/agent-tui-visual-layout/spec-round-1-self.md`.
- Round 2, self: NEEDS_CHANGES. Added feasible boundary budgets for 14-17 and 8-9 rows plus a one-line Unicode overflow/recovery contract for every fixed region. Evidence: `docs/reviews/agent-tui-visual-layout/spec-round-2-self.md`.
- Round 3, self: NEEDS_CHANGES. An unresolved MAJOR remained: state contracts for every palette/pending/recovery/locked/temporary/unseen case at 60x8 and 60x9 were not explicit enough to implement or test. Evidence: `docs/reviews/agent-tui-visual-layout/spec-round-3-self.md`.
- Restart decision: the interactive-height threshold is raised to ten rows, so 60x8 and 60x9 use the resize-required fallback. The spec gate restarted from round 1 with the same `self` reviewer.
- Restart round 1, self: NEEDS_CHANGES. Replaced an obsolete 8-13 / below-8 rule with the ten-row policy consistently. Evidence: `docs/reviews/agent-tui-visual-layout/spec-restart-round-1-self.md`.
- Restart round 2, self: APPROVE. No remaining BLOCKER or MAJOR findings. Evidence: `docs/reviews/agent-tui-visual-layout/spec-restart-round-2-self.md`.
