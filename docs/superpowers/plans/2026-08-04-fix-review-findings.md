# Fix Review Findings Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the five review findings against `HEAD 8834ad0`: the pending-turn shutdown race (P1), unbounded LLM HTTP response reads (P1), two visual-layout spec deviations (P2), and world-readable save/debug files (P2).

**Architecture:** Four small, independent fixes. (1) The TUI gains a cancelable turn context plus a `quitting` flag so Ctrl-C/Esc/`/exit` cancel the in-flight turn and quit only after `turnFinishedMsg` arrives; `main` roots everything in a `signal.NotifyContext`. (2) The LLM client caps response and error bodies with `io.LimitReader`. (3)+(4) The condensed and compact shell renderers in `tui.go` are brought back in line with the visual-layout spec row budgets. (5) Data directories become `0700` and logs/event/lock files `0600`.

**Tech Stack:** Go, Bubble Tea v2 (`charm.land/bubbletea/v2`), `net/http`, `os/signal`, SQLite (`modernc.org/sqlite`), `gopkg.in/yaml.v3`.

## Global Constraints

- Module path is `github.com/namtt/tutine-trpg`; commit style is `type(scope): summary` (e.g. `fix(security):`, `fix(tui):`, `chore:`). No AI-generated footers/co-author lines.
- Follow TDD per task: write the failing test, watch it fail, implement minimally, watch it pass.
- Run `gofmt` on every changed Go file and `go test ./...` at the end of every task.
- No real network calls in tests; LLM behavior goes through `httptest`/fakes. No test touches the filesystem outside `t.TempDir()`.
- Never commit or log API keys; config values stay referenced by environment-variable name only.
- Player-facing UI copy stays Vietnamese and must not leak raw save IDs, paths, or internal identifiers.
- The persistence spec explicitly accepts `state.json`/`events.jsonl` being best-effort per-turn; do not change that contract.
- The layout spec (`docs/superpowers/specs/2026-08-04-agent-tui-visual-layout-design.md`) lines 82 and 106 are the authoritative row-budget requirements for Tasks 3 and 4.

---

### Task 1: Cancel-Aware Turn Lifecycle (P1 shutdown race)

**Files:**
- Modify: `cmd/tu-tien-cli/tui.go:72-99` (model struct), `cmd/tu-tien-cli/tui.go:151-203` (`newTUIModel`, `runTUI`), `cmd/tu-tien-cli/tui.go:232-236` (Ctrl-C), `cmd/tu-tien-cli/tui.go:344-363` (`handleEsc`), `cmd/tu-tien-cli/tui.go:423-439` (`handleCommand`), `cmd/tu-tien-cli/tui.go:441-455` (`applyTurnMsg`)
- Modify: `cmd/tu-tien-cli/main.go:38-57` (`run`)
- Test: `cmd/tu-tien-cli/tui_test.go`

**Interfaces:**
- Consumes: `tuiModel.ctx context.Context` (already exists, set by `runTUI`).
- Produces: `tuiModel.cancel context.CancelFunc`, `tuiModel.quitting bool`. `runTUI(ctx, session, providerLabel, logger)` derives a cancelable child context and stores both on the model.

- [ ] **Step 1: Write the failing tests**

Add to `cmd/tu-tien-cli/tui_test.go`:

```go
func TestTUICtrlCWhilePendingCancelsTurnAndQuitsAfterCompletion(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	model := newTUIModel(&recordingSession{save: game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})}, "test-model")
	model.ctx = ctx
	model.cancel = cancel
	model.pending = &pendingTurn{turnNumber: 1, action: "ta quan sát"}

	updated, cmd := model.Update(tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl}))
	model = updated.(tuiModel)
	if cmd != nil {
		t.Fatal("cmd is not nil, quit must wait for the pending turn to finish")
	}
	if !model.quitting {
		t.Fatal("quitting should be set so applyTurnMsg quits once the turn completes")
	}
	if ctx.Err() == nil {
		t.Fatal("pending turn context was not canceled")
	}

	model, cmd = model.applyTurnMsg(turnFinishedMsg{input: "ta quan sát", err: ctx.Err()})
	assertQuitCommand(t, cmd)
}

func TestTUIEscWhilePendingCancelsTurnAndQuitsAfterCompletion(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	model := newTUIModel(&recordingSession{save: game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})}, "test-model")
	model.ctx = ctx
	model.cancel = cancel
	model.pending = &pendingTurn{turnNumber: 1, action: "ta quan sát"}

	updated, cmd := model.Update(keyPress(tea.KeyEsc, ""))
	model = updated.(tuiModel)
	if cmd != nil {
		t.Fatal("cmd is not nil, quit must wait for the pending turn to finish")
	}
	if !model.quitting {
		t.Fatal("quitting should be set so applyTurnMsg quits once the turn completes")
	}
	if ctx.Err() == nil {
		t.Fatal("pending turn context was not canceled")
	}
}

func TestTUIExitCommandWhilePendingCancelsTurnAndWaits(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	model := newTUIModel(&recordingSession{save: game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})}, "test-model")
	model.ctx = ctx
	model.cancel = cancel
	model.pending = &pendingTurn{turnNumber: 1, action: "ta quan sát"}

	model, cmd := model.handleCommand("/exit")
	if cmd != nil {
		t.Fatal("cmd is not nil, /exit while pending must wait for the turn to finish")
	}
	if !model.quitting || ctx.Err() == nil {
		t.Fatalf("quitting = %v, ctx.Err() = %v, want quitting=true and canceled ctx", model.quitting, ctx.Err())
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./cmd/tu-tien-cli -run 'TestTUI(CtrlC|Esc|ExitCommand)WhilePending' -v`
Expected: FAIL — `tuiModel` has no field/method `cancel` (compile error) or `quitting` is never set.

- [ ] **Step 3: Implement the minimal lifecycle change**

In `cmd/tu-tien-cli/tui.go`:

Add two fields to `tuiModel` after `ctx` (line 73):

```go
	ctx           context.Context
	cancel        context.CancelFunc
	quitting      bool
```

In `newTUIModel`, set a safe no-op default in the returned struct (the real cancel is wired by `runTUI`):

```go
	return tuiModel{
		ctx:           context.Background(),
		cancel:        func() {},
		...
```

Replace `runTUI` (lines 197-203):

```go
func runTUI(ctx context.Context, session orchestrator.GameSession, providerLabel string, logger *log.Logger) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	model := newTUIModel(session, providerLabel)
	model.ctx = ctx
	model.cancel = cancel
	model.logger = logger
	_, err := tea.NewProgram(model).Run()
	return err
}
```

Replace the Ctrl-C branch (lines 234-236):

```go
	if keyPress.Code == 'c' && keyPress.Mod&tea.ModCtrl != 0 {
		if m.pending != nil {
			m.cancel()
			m.quitting = true
			return m, nil
		}
		return m, tea.Quit
	}
```

Replace the `default` branch of `handleEsc` (lines 360-362):

```go
	default:
		if m.pending != nil {
			m.cancel()
			m.quitting = true
			return m, nil
		}
		return m, tea.Quit
	}
```

Replace the `/exit` case in `handleCommand` (lines 433-434):

```go
	case "/exit":
		if m.pending != nil {
			m.cancel()
			m.quitting = true
			return m, nil
		}
		return m, tea.Quit
```

At the top of `applyTurnMsg`, after `m.pending = nil` (line 443):

```go
	m.pending = nil
	if m.quitting {
		return m, tea.Quit
	}
```

In `cmd/tu-tien-cli/main.go`, root everything in a signal-cancelable context. Add imports `os/signal` and `syscall`, then change `run` (lines 38-57):

```go
func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	name := flag.String("name", "Vô Danh", "player name for a new game")
	configPath := flag.String("config", "configs/llm.yaml", "runtime config path")
	saveID := flag.String("save", "", "resume a specific save, skipping auto-resume")
	forceNew := flag.Bool("new", false, "force a new game even if a save exists")
	flag.Parse()

	opts := StartupOptions{PlayerName: *name, SaveID: *saveID, ForceNew: *forceNew}
	session, logger, cleanup, err := buildSession(ctx, *configPath, opts)
	if err != nil {
		return err
	}
	defer cleanup()

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	return runTUI(ctx, session, cfg.LLM.Model, logger)
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./cmd/tu-tien-cli -run 'TestTUI' -v`
Expected: PASS, including the pre-existing `TestTUIKeyPressCtrlCQuits`, `TestTUIKeyPressEscQuitsWhenNoLayerIsOpen`, and `TestTUIAmbiguousCompletionAllowsExitCommand` tests (pending is nil there, so quit stays immediate).

- [ ] **Step 5: Verify formatting, race, and full suite**

Run: `gofmt -w cmd/tu-tien-cli/tui.go cmd/tu-tien-cli/main.go` then `go test -race ./cmd/tu-tien-cli` and `go test ./...`
Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/tu-tien-cli/tui.go cmd/tu-tien-cli/main.go cmd/tu-tien-cli/tui_test.go
git commit -m "fix(cli): cancel pending turn before quitting the TUI"
```

Known limitation to note in the commit body if desired: an external SIGINT delivered as a signal (not as the raw-mode Ctrl-C keypress the TUI handles) can still end the Bubble Tea program immediately; the root context cancellation at least stops in-flight LLM HTTP calls. Full signal-level hardening is out of scope for this task.

---

### Task 2: Bound LLM HTTP Response Size (P1)

**Files:**
- Modify: `internal/llm/openai_compatible.go:286-321` (`sendChatMessage`)
- Test: `internal/llm/openai_compatible_test.go`

**Interfaces:**
- Consumes: existing `providerStatusError` type and `chatMessage`.
- Produces: two package-level constants `maxResponseBytes` and `maxErrorBodyBytes` used by `sendChatMessage`.

- [ ] **Step 1: Write the failing tests**

Add to `internal/llm/openai_compatible_test.go`:

```go
func TestOpenAICompatibleClientRejectsOversizedResponse(t *testing.T) {
	t.Setenv("TEST_LLM_KEY", "secret-test-key")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(make([]byte, maxResponseBytes+1024))
	}))
	defer server.Close()

	client := newTestOpenAIClient(t, server.URL)
	_, err := client.PlanRetrieval(context.Background(), PlannerRequest{PlayerAction: "ta quan sat"})
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("err = %v, want bounded-size error", err)
	}
}

func TestOpenAICompatibleClientTruncatesProviderErrorBody(t *testing.T) {
	t.Setenv("TEST_LLM_KEY", "secret-test-key")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write(make([]byte, maxErrorBodyBytes+1024)) // over the 4 KiB error-body cap, under the 8 MiB size guard
	}))
	defer server.Close()

	client := newTestOpenAIClient(t, server.URL)
	client.maxRetries = 0
	_, err := client.PlanRetrieval(context.Background(), PlannerRequest{PlayerAction: "ta quan sat"})
	if err == nil {
		t.Fatal("expected provider error")
	}
	if len(err.Error()) > maxErrorBodyBytes*2 {
		t.Fatalf("error message length = %d, want bounded body in error", len(err.Error()))
	}
	if !strings.Contains(err.Error(), "…") {
		t.Fatalf("error message %q, want truncation marker", err.Error())
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/llm -run 'TestOpenAICompatibleClient(RejectsOversizedResponse|TruncatesProviderErrorBody)' -v`
Expected: FAIL — compile error first (`maxResponseBytes`/`maxErrorBodyBytes` are undefined until Step 3); after Step 3 the runtime assertions take over (oversized body rejected / error message bounded).

- [ ] **Step 3: Implement the size limit**

In `internal/llm/openai_compatible.go`, add constants near the other package-level declarations (e.g. after `maxToolCallRounds`, line 214):

```go
// maxResponseBytes caps a single provider response body so a compromised or
// misconfigured endpoint cannot exhaust process memory. maxErrorBodyBytes
// caps how much of a non-2xx body is retained in providerStatusError.
const (
	maxResponseBytes  = 8 << 20 // 8 MiB
	maxErrorBodyBytes = 4 << 10 // 4 KiB
)
```

Replace the body read and status handling in `sendChatMessage` (lines 300-306):

```go
	data, readErr := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if readErr != nil {
		return chatMessage{}, readErr
	}
	if len(data) > maxResponseBytes {
		return chatMessage{}, fmt.Errorf("llm provider response exceeds %d bytes", maxResponseBytes)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body := strings.TrimSpace(string(data))
		if len(body) > maxErrorBodyBytes {
			body = body[:maxErrorBodyBytes] + "…"
		}
		return chatMessage{}, providerStatusError{statusCode: resp.StatusCode, body: body}
	}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/llm -v`
Expected: PASS (new tests plus all existing `httptest`-based tests; responses in existing tests are far below the cap).

- [ ] **Step 5: Verify formatting and full suite**

Run: `gofmt -w internal/llm/openai_compatible.go` then `go test ./...`
Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/llm/openai_compatible.go internal/llm/openai_compatible_test.go
git commit -m "fix(llm): bound provider response and error body size"
```

---

### Task 3: Condensed Layout Spec Compliance (P2)

**Files:**
- Modify: `cmd/tu-tien-cli/tui.go:578-597` (`renderCondensedShell`)
- Test: `cmd/tu-tien-cli/tui_test.go`

**Interfaces:**
- Consumes: existing `m.renderCompactSummary`, `m.renderNarrowNormalAction`, `m.viewport`, `m.unseen`, `m.width`, `m.height`.
- Produces: condensed shell with no transcript section label and a viewport of at least 2 rows for heights 14-17.

- [ ] **Step 1: Write the failing test**

Add to `cmd/tu-tien-cli/tui_test.go`:

```go
func TestTUICondensedShellKeepsTwoViewportRowsWithoutSectionLabel(t *testing.T) {
	for _, height := range []int{14, 15, 16, 17} {
		t.Run(fmt.Sprintf("60x%d", height), func(t *testing.T) {
			model := newTUIModel(&recordingSession{save: game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})}, "test-model")
			model.width, model.height = 60, height
			model.syncLayout()
			model, _ = model.applyTurnMsg(turnFinishedMsg{input: "ta quan sát", result: &game.TurnResult{
				Narration:        "Bạn đứng trước cổng môn.",
				SuggestedActions: []string{"Quan sát cổng môn", "Hỏi đệ tử", "Kiểm tra trạng thái"},
			}})
			view := model.View().Content
			if strings.Contains(view, "NHẬT KÝ HÀNH TRÌNH") {
				t.Fatalf("condensed shell must not render the transcript section label:\n%s", view)
			}
			if rows := strings.Count(view, "\n") + 1; rows > model.height {
				t.Fatalf("rendered rows = %d, exceed height %d:\n%s", rows, model.height, view)
			}
			lines := strings.Split(view, "\n")
			vpRows := 0
			for _, line := range lines[2:] { // skip header + summary, stop at first suggestion row
				if strings.HasPrefix(line, "1. ") {
					break
				}
				vpRows++
			}
			if vpRows < 2 {
				t.Fatalf("condensed viewport rows = %d, want >= 2:\n%s", vpRows, view)
			}
		})
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./cmd/tu-tien-cli -run TestTUICondensedShellKeepsTwoViewportRowsWithoutSectionLabel -v`
Expected: FAIL — the shell contains `NHẬT KÝ HÀNH TRÌNH` and/or viewport rows < 2.

- [ ] **Step 3: Implement the spec-compliant renderer**

Replace `renderCondensedShell` (lines 578-597):

```go
func (m tuiModel) renderCondensedShell(save game.SaveGame) string {
	width := max(m.width, 20)
	action := m.renderNarrowNormalAction(width, false)
	actionLines := strings.Split(action, "\n")
	viewport := m.viewport
	viewport.SetWidth(width)
	viewport.SetHeight(max(m.height-len(actionLines)-2-boolToInt(m.unseen), 2))

	lines := []string{
		truncateCells("TUTINE TRPG · "+sceneName(save.CurrentScene)+fmt.Sprintf(" · Lượt %02d", save.CurrentTurn+1), width),
		truncateCells(renderCompactSummary(save), width),
	}
	lines = append(lines, strings.Split(viewport.View(), "\n")...)
	if m.unseen {
		lines = append(lines, "↓ Có lượt mới · End để xem")
	}
	lines = append(lines, actionLines...)
	return strings.Join(lines, "\n")
}
```

The budget change is `-3` → `-2` (the section-label row is gone) and the floor is `1` → `2`. At 60×14 the fixed regions are 2 header rows + 3 suggestions + 3 editor + 1 footer = 9, leaving 5 viewport rows; the floor only engages if fixed content ever exceeds the budget.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./cmd/tu-tien-cli -run 'TestTUICondensedShellKeepsTwoViewportRowsWithoutSectionLabel|TestTUIVisualShellFitsNarrowHeightBudget|TestTUINarrowVisualShellKeepsThreeSeparateSuggestions|TestTUIVisualLayouts|TestTUIVisualStateMatrix' -v`
Expected: PASS. If an existing test asserted the old row count or the label text, update that assertion to the new spec-compliant output (a removed line only reduces rendered rows, so budget tests stay valid).

- [ ] **Step 5: Verify formatting and full suite**

Run: `gofmt -w cmd/tu-tien-cli/tui.go` then `go test ./...`
Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/tu-tien-cli/tui.go cmd/tu-tien-cli/tui_test.go
git commit -m "fix(tui): make condensed layout spec-compliant at 14-17 rows"
```

---

### Task 4: Compact Pending State Spec Compliance (P2/Low)

**Files:**
- Modify: `cmd/tu-tien-cli/tui.go:691-693` (compact pending branch of `renderCompactShell`)
- Test: `cmd/tu-tien-cli/tui_test.go`

**Interfaces:**
- Consumes: `m.pending`, `m.spinner`, `truncateCells`, `contentWidth`.
- Produces: compact pending state as exactly one status line plus the paging footer.

- [ ] **Step 1: Write the failing test**

Add to `cmd/tu-tien-cli/tui_test.go`:

```go
func TestTUICompactPendingRendersOneStatusLinePlusPagingFooter(t *testing.T) {
	model := newTUIModel(&recordingSession{save: game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})}, "test-model")
	model.width, model.height = 60, 10
	model.pending = &pendingTurn{turnNumber: 1, action: "ta quan sát"}
	view := model.View().Content
	if strings.Contains(view, "ĐANG XỬ LÝ LƯỢT") {
		t.Fatalf("compact pending must not render a separate status heading:\n%s", view)
	}
	if !strings.Contains(view, "Đang xử lý lượt") || !strings.Contains(view, "PgUp/PgDn lịch sử") {
		t.Fatalf("compact pending must show one status line plus a paging footer:\n%s", view)
	}
	if rows := strings.Count(view, "\n") + 1; rows > model.height {
		t.Fatalf("rendered rows = %d, exceed height %d:\n%s", rows, model.height, view)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./cmd/tu-tien-cli -run TestTUICompactPendingRendersOneStatusLinePlusPagingFooter -v`
Expected: FAIL — the view still contains the separate `ĐANG XỬ LÝ LƯỢT` heading.

- [ ] **Step 3: Implement the spec-compliant pending branch**

Replace lines 691-693 in `renderCompactShell`:

```go
	if m.pending != nil {
		lines = append(lines, truncateCells(m.spinner.View()+" Đang xử lý lượt...", contentWidth), "PgUp/PgDn lịch sử")
		return strings.Join(lines, "\n")
	}
```

Also update the pending-state expectation in the existing matrix test `TestTUIVisualStateMatrixFitsTargetLayouts` (`cmd/tu-tien-cli/tui_test.go:644`): its `pending` contains-list currently asserts `[]string{"ĐANG XỬ LÝ LƯỢT", "PgUp/PgDn"}` at all four targets, but after this change the 60x10 compact branch no longer renders the `ĐANG XỬ LÝ LƯỢT` heading. Change the list to `[]string{"Đang xử lý lượt", "PgUp/PgDn"}` — both substrings are present at every target (`Đang xử lý lượt chơi...` is the spinner text in the full-narrow branch at `tui.go:634` and the new compact line; `PgUp/PgDn` appears in both branches), so no state split is needed.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./cmd/tu-tien-cli -run 'TestTUICompactPendingRendersOneStatusLinePlusPagingFooter|TestTUIPendingTurn|TestTUICompact|TestTUIVisualStateMatrixFitsTargetLayouts' -v`
Expected: PASS.

- [ ] **Step 5: Verify formatting and full suite**

Run: `gofmt -w cmd/tu-tien-cli/tui.go` then `go test ./...`
Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/tu-tien-cli/tui.go cmd/tu-tien-cli/tui_test.go
git commit -m "fix(tui): compact pending state renders one status line plus footer"
```

---

### Task 5: Restrict Save And Debug File Permissions (P2)

**Files:**
- Modify: `cmd/tu-tien-cli/main.go:98-109` (`buildSession` data dir + debug.log)
- Modify: `internal/storage/file_store.go:43, 109, 116, 144, 148` (MkdirAll/OpenFile modes)
- Test: `internal/storage/file_store_test.go`, `cmd/tu-tien-cli/main_test.go`

**Interfaces:**
- Consumes: existing `FileStore` methods and `buildSession`; no signature changes.
- Produces: directories `0700`, files `0600` for state/event/lock/debug artifacts created after this change.

- [ ] **Step 1: Write the failing tests**

Add to `internal/storage/file_store_test.go`:

```go
func TestSaveSnapshotAndEventsUsePrivatePermissions(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)
	save := game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})
	lock, err := store.AcquireLock(context.Background(), save.SaveID)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()
	if err := store.SaveSnapshot(context.Background(), save); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendEvent(context.Background(), save.SaveID, Event{Turn: 1, Type: EventTypeTurnResolved}); err != nil {
		t.Fatal(err)
	}
	for path, what := range map[string]string{
		filepath.Join(dir, "saves", save.SaveID):                     "save directory",
		filepath.Join(dir, "saves", save.SaveID, "state.json"):       "state.json",
		filepath.Join(dir, "saves", save.SaveID, "events.jsonl"):     "events.jsonl",
		filepath.Join(dir, "saves", save.SaveID, ".lock"):            ".lock",
	} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", what, err)
		}
		if perm := info.Mode().Perm(); perm&0o077 != 0 {
			t.Fatalf("%s permissions = %#o, want no group/other access", what, perm)
		}
	}
}
```

Add to `cmd/tu-tien-cli/main_test.go`, following the existing helper convention — `writeTestConfig(t, dataDir, apiKeyEnv)` takes **three** args (`main_test.go:259`) and `buildSession` fails without `TEST_GROQ_API_KEY` set (see `TestBuildSessionRejectsMissingAPIKey`, `main_test.go:16`):

```go
func TestBuildSessionUsesPrivateDataPermissions(t *testing.T) {
	t.Setenv("TEST_GROQ_API_KEY", "secret-test-key")
	dataDir := t.TempDir() // writeTestConfig writes dataDir/llm.yaml directly; no pre-created subdir
	cfgPath := writeTestConfig(t, dataDir, "TEST_GROQ_API_KEY")
	_, _, cleanup, err := buildSession(context.Background(), cfgPath, StartupOptions{PlayerName: "Nam"})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	for path, what := range map[string]string{
		dataDir:                             "data directory",
		filepath.Join(dataDir, "debug.log"): "debug.log",
	} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", what, err)
		}
		if perm := info.Mode().Perm(); perm&0o077 != 0 {
			t.Fatalf("%s permissions = %#o, want no group/other access", what, perm)
		}
	}
}
```

Note: `writeTestConfig` writes `dataDir/llm.yaml` with `os.WriteFile` and no `MkdirAll` (`main_test.go:262-266`), so `dataDir` must already exist — plain `t.TempDir()` (no nested `data` subdir) is required.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/storage -run TestSaveSnapshotAndEventsUsePrivatePermissions -v && go test ./cmd/tu-tien-cli -run TestBuildSessionUsesPrivateDataPermissions -v`
Expected: FAIL — permissions include group/other bits (`0755`/`0644`).

- [ ] **Step 3: Implement the permission changes**

In `cmd/tu-tien-cli/main.go`:

```go
	if err := os.MkdirAll(dataDir, 0o700); err != nil { // was 0o755
		return nil, nil, nil, err
	}
	logFile, err := os.OpenFile(filepath.Join(dataDir, "debug.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) // was 0o644
```

In `internal/storage/file_store.go`, change every mode constant:

- `SaveSnapshot` (line 43): `os.MkdirAll(dir, 0o700)` (was `0o755`)
- `AppendEvent` (line 109): `os.MkdirAll(dir, 0o700)` (was `0o755`)
- `AppendEvent` (line 116): `os.OpenFile(..., os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)` (was `0o644`)
- `AcquireLock` (line 144): `os.MkdirAll(dir, 0o700)` (was `0o755`)
- `AcquireLock` (line 148): `os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)` (was `0o644`)

`state.json` already gets `0600` from `os.CreateTemp` and keeps it through the rename; no change needed there. Existing directories created with `0755` before this fix are not chmod'd retroactively — fresh data dirs and saves get the private modes.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/storage -run 'TestSaveSnapshotAndEventsUsePrivatePermissions' -v && go test ./cmd/tu-tien-cli -run 'TestBuildSession' -v`
Expected: PASS.

- [ ] **Step 5: Verify formatting and full suite**

Run: `gofmt -w cmd/tu-tien-cli/main.go internal/storage/file_store.go` then `go test ./...`
Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/tu-tien-cli/main.go cmd/tu-tien-cli/main_test.go internal/storage/file_store.go internal/storage/file_store_test.go
git commit -m "fix(security): create save, event, lock, and debug files with private permissions"
```

---

## Self-Review

**Spec coverage:**
- Task 1 covers the P1 shutdown race (`main.go` contexts, TUI quit flow).
- Task 2 covers the P1 unbounded `io.ReadAll` (`openai_compatible.go:300`).
- Task 3 covers the two condensed-layout deviations (section label + viewport minimum) from layout spec line 82.
- Task 4 covers the compact pending state deviation from layout spec line 106.
- Task 5 covers world-readable dirs/logs/events (`main.go:102,105`; `file_store.go:43,109,116`).

**Out of scope, deliberately:** snapshot/event-log best-effort persistence (explicitly accepted by the persistence spec), stale `.lock` cleanup (accepted by the persistence spec with manual `rm`), and the SIGINT-as-signal race (documented in Task 1).

**Placeholder scan:** every step carries concrete code or an exact runnable command; the only named-but-unverified symbol is the config-file helper in `main_test.go`, with an explicit fallback instruction to extract it.

**Type consistency:** `tuiModel.cancel`/`tuiModel.quitting` are introduced once in Task 1 and used by the same task; constants `maxResponseBytes`/`maxErrorBodyBytes` are defined and used inside Task 2; no cross-task symbol drift.
