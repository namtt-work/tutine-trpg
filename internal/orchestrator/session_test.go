package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/namtt/tutine-trpg/internal/game"
	"github.com/namtt/tutine-trpg/internal/llm"
	"github.com/namtt/tutine-trpg/internal/memory"
	"github.com/namtt/tutine-trpg/internal/storage"
)

type extractorFailingClient struct {
	llm.FakeClient
}

func (extractorFailingClient) ExtractMemories(context.Context, llm.ExtractorRequest) ([]llm.MemoryDraft, error) {
	return nil, errors.New("extractor unavailable")
}

type plannedRetrievalClient struct {
	llm.FakeClient
	plan llm.RetrievalPlan
}

func (c plannedRetrievalClient) PlanRetrieval(context.Context, llm.PlannerRequest) (llm.RetrievalPlan, error) {
	return c.plan, nil
}

type invalidEffectsClient struct {
	llm.FakeClient
}

func (invalidEffectsClient) Narrate(context.Context, llm.NarratorRequest) (llm.NarratorResponse, error) {
	return llm.NarratorResponse{
		Narration: "Bạn bước tới cổng môn.",
		ProposedEffects: []game.Effect{
			{Type: game.EffectEnergyDelta, Amount: 1},
			{Type: game.EffectRelationshipDelta, Amount: 1},
		},
		SuggestedNextOptions: []string{"Quan sát xung quanh"},
	}, nil
}

type continuityClient struct {
	llm.FakeClient
	requests []llm.NarratorRequest
}

func (c *continuityClient) Narrate(_ context.Context, req llm.NarratorRequest) (llm.NarratorResponse, error) {
	c.requests = append(c.requests, req)
	if len(c.requests) == 1 {
		return llm.NarratorResponse{Narration: "Người gác cổng yêu cầu lệnh bài.", ProposedEffects: []game.Effect{{Type: game.EffectEnergyDelta, TargetID: "player", Amount: -1}}}, nil
	}
	return llm.NarratorResponse{Narration: "Người gác cổng chờ bạn trả lời."}, nil
}

// toolLoopClient is a ToolCapableClient test double: it exercises the tool
// call the same way a real provider's model would (call roll_check, read
// the engine's result, then narrate) so we can assert the loop is actually
// wired through Session, not just present in the llm package.
type toolLoopClient struct {
	llm.FakeClient
	calls []llm.ToolCall
}

func (c *toolLoopClient) NarrateWithTools(ctx context.Context, req llm.NarratorRequest, tools []llm.ToolDefinition, exec llm.ToolExecutor) (llm.NarratorResponse, error) {
	if len(tools) != 1 || tools[0].Name != "roll_check" {
		return llm.NarratorResponse{}, fmt.Errorf("unexpected tools = %#v", tools)
	}
	result, err := exec(ctx, llm.ToolCall{ID: "call_1", Name: "roll_check", Arguments: json.RawMessage(`{"stat":"comprehension","difficulty":5}`)})
	if err != nil {
		return llm.NarratorResponse{}, err
	}
	c.calls = append(c.calls, llm.ToolCall{Name: "roll_check", Arguments: json.RawMessage(result.Content)})
	return llm.NarratorResponse{Narration: fmt.Sprintf("Kết quả dò xét: %s", result.Content)}, nil
}

type recordingStore struct {
	query memory.Query
}

func (s *recordingStore) Add(context.Context, memory.Memory) error {
	return nil
}

func (s *recordingStore) Search(_ context.Context, query memory.Query) ([]memory.Hit, error) {
	s.query = query
	return nil, nil
}

func (*recordingStore) Close() error {
	return nil
}

type fakeStore struct {
	snapshotErr error
	eventErr    error
	snapshots   []game.SaveGame
	events      []storage.Event
	callOrder   []string
}

func (s *fakeStore) SaveSnapshot(_ context.Context, save game.SaveGame) error {
	s.callOrder = append(s.callOrder, "snapshot")
	s.snapshots = append(s.snapshots, save)
	return s.snapshotErr
}

func (s *fakeStore) LoadSnapshot(context.Context, string) (game.SaveGame, error) {
	return game.SaveGame{}, errors.New("fakeStore: LoadSnapshot not used by HandleTurn tests")
}

func (s *fakeStore) AppendEvent(_ context.Context, _ string, event storage.Event) error {
	s.callOrder = append(s.callOrder, "event")
	s.events = append(s.events, event)
	return s.eventErr
}

func (s *fakeStore) ListSaves(context.Context, string) ([]storage.SaveSummary, error) {
	return nil, nil
}

func (s *fakeStore) AcquireLock(context.Context, string) (storage.Lock, error) {
	return nil, errors.New("fakeStore: AcquireLock not used by HandleTurn tests")
}

func TestHandleTurnReturnsNarrationAndAdvancesTurn(t *testing.T) {
	ctx := context.Background()
	store, err := memory.NewSQLiteStore(ctx, filepath.Join(t.TempDir(), "game.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	save := game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})
	session := NewSession(save, llm.FakeClient{}, store, &fakeStore{}, []string{"trust", "secret"})

	result, err := session.HandleTurn(ctx, PlayerInput{Text: "ta quan sat cong mon"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Narration == "" {
		t.Fatal("expected narration")
	}
	if session.Save().CurrentTurn != 1 {
		t.Fatalf("turn = %d, want 1", session.Save().CurrentTurn)
	}
}

func TestHandleTurnProvidesRecentNarrationToNextTurn(t *testing.T) {
	client := &continuityClient{}
	session := NewSession(game.NewStarterSave(game.NewGameRequest{Name: "Nam"}), client, &recordingStore{}, &fakeStore{}, nil)

	if _, err := session.HandleTurn(context.Background(), PlayerInput{Text: "ta bước tới"}); err != nil {
		t.Fatalf("first HandleTurn returned error: %v", err)
	}
	if _, err := session.HandleTurn(context.Background(), PlayerInput{Text: "ta trình lệnh bài"}); err != nil {
		t.Fatalf("second HandleTurn returned error: %v", err)
	}

	if got, want := client.requests[1].RecentTurns, []llm.RecentTurn{{PlayerAction: "ta bước tới", Narration: "Người gác cổng yêu cầu lệnh bài.", ResolvedChanges: []game.StateChangeView{{Type: game.EffectEnergyDelta, TargetID: "player", Amount: -1, Message: "linh lực thay đổi"}}}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("RecentTurns = %#v, want %#v", got, want)
	}
	wantState := llm.NarratorState{CurrentTurn: 1, CurrentScene: "loc_outer_gate", Player: llm.NarratorPlayerState{Identity: "Người mới cầu nhập môn tại Thanh Vân Tông", Name: "Nam", Realm: "Luyện Khí", Stage: 1, HP: 30, MaxHP: 30, SpiritualEnergy: 19, MaxEnergy: 20, Attack: 6, Defense: 4, Speed: 5, Comprehension: 5, Luck: 5, Relationships: map[string]int{"npc_luc_thanh_nghi": 0}, Techniques: []string{"Đòn cơ bản"}}, Inventory: map[string]int{}, WorldFlags: map[string]bool{}, Cooldowns: map[string]int{}}
	if got := client.requests[1].AuthoritativeState; !reflect.DeepEqual(got, wantState) {
		t.Fatalf("AuthoritativeState = %#v, want %#v", got, wantState)
	}
}

func TestHandleTurnReturnsResolvedTurnWhenMemoryExtractionFails(t *testing.T) {
	ctx := context.Background()
	store, err := memory.NewSQLiteStore(ctx, filepath.Join(t.TempDir(), "game.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	save := game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})
	session := NewSession(save, extractorFailingClient{}, store, &fakeStore{}, []string{"trust", "secret"})

	result, err := session.HandleTurn(ctx, PlayerInput{Text: "ta quan sat cong mon"})
	if err != nil {
		t.Fatalf("HandleTurn returned error: %v", err)
	}
	if session.Save().CurrentTurn != 1 {
		t.Fatalf("turn = %d, want 1", session.Save().CurrentTurn)
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("warnings = %#v, want extraction warning", result.Warnings)
	}
}

func TestHandleTurnRejectsInvalidLLMEffectsWithoutFailingTurn(t *testing.T) {
	ctx := context.Background()
	store, err := memory.NewSQLiteStore(ctx, filepath.Join(t.TempDir(), "game.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	save := game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})
	session := NewSession(save, invalidEffectsClient{}, store, &fakeStore{}, []string{"trust", "secret"})

	result, err := session.HandleTurn(ctx, PlayerInput{Text: "bắt đầu"})
	if err != nil {
		t.Fatalf("HandleTurn returned error: %v", err)
	}
	if result.Narration != "Bạn bước tới cổng môn." {
		t.Fatalf("narration = %q", result.Narration)
	}
	if session.Save().CurrentTurn != 1 {
		t.Fatalf("turn = %d, want 1", session.Save().CurrentTurn)
	}
	if len(result.StateChanges) != 0 {
		t.Fatalf("state changes = %#v, want invalid effects rejected", result.StateChanges)
	}
	if len(result.Warnings) != 2 {
		t.Fatalf("warnings = %#v, want two rejected effect warnings", result.Warnings)
	}
}

func TestHandleTurnUsesToolCapableClientRollCheck(t *testing.T) {
	client := &toolLoopClient{}
	save := game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})
	session := NewSession(save, client, &recordingStore{}, &fakeStore{}, nil)
	session.rollFunc = func() int { return 10 }

	result, err := session.HandleTurn(context.Background(), PlayerInput{Text: "ta dò xét chấp sự"})
	if err != nil {
		t.Fatalf("HandleTurn returned error: %v", err)
	}
	if len(client.calls) != 1 || client.calls[0].Name != "roll_check" {
		t.Fatalf("calls = %#v, want one roll_check call", client.calls)
	}
	if !strings.Contains(result.Narration, `"success":true`) {
		t.Fatalf("narration = %q, want it to embed the engine's check result", result.Narration)
	}

	var payload game.CheckResult
	if err := json.Unmarshal(client.calls[0].Arguments, &payload); err != nil {
		t.Fatalf("decode roll_check result: %v", err)
	}
	if payload.Roll != 10 || !payload.Success {
		t.Fatalf("engine check result = %#v, want roll=10 success=true (comprehension 5, difficulty 5 -> threshold 50)", payload)
	}
}

func TestExecuteToolRejectsUnknownStat(t *testing.T) {
	save := game.NewStarterSave(game.NewGameRequest{Name: "Nam"})
	session := NewSession(save, llm.FakeClient{}, nil, &fakeStore{}, nil)

	_, err := session.executeTool(context.Background(), llm.ToolCall{ID: "call_1", Name: "roll_check", Arguments: json.RawMessage(`{"stat":"charisma","difficulty":5}`)})
	if err == nil {
		t.Fatal("expected error for unknown stat")
	}
}

func TestExecuteToolRejectsUnknownToolName(t *testing.T) {
	save := game.NewStarterSave(game.NewGameRequest{Name: "Nam"})
	session := NewSession(save, llm.FakeClient{}, nil, &fakeStore{}, nil)

	_, err := session.executeTool(context.Background(), llm.ToolCall{ID: "call_1", Name: "teleport", Arguments: json.RawMessage(`{}`)})
	if err == nil {
		t.Fatal("expected error for unknown tool")
	}
}

func TestSaveReturnsIndependentCopy(t *testing.T) {
	save := game.NewStarterSave(game.NewGameRequest{Name: "Nam", Traits: []string{"careful"}, CampaignID: "thanh-van-sect"})
	session := NewSession(save, llm.FakeClient{}, nil, &fakeStore{}, nil)

	returnedSave := session.Save()
	returnedSave.Inventory["moonlit_grass"] = 1
	returnedSave.Player.Traits[0] = "reckless"

	currentSave := session.Save()
	if len(currentSave.Inventory) != 0 {
		t.Fatalf("inventory mutation leaked into session: %#v", currentSave.Inventory)
	}
	if got := currentSave.Player.Traits[0]; got != "careful" {
		t.Fatalf("trait = %q, want %q", got, "careful")
	}
}

func TestHandleTurnForwardsAllRetrievalPlanFilters(t *testing.T) {
	store := &recordingStore{}
	plan := llm.RetrievalPlan{
		Entities:    []string{"npc_luc_thanh_nghi"},
		Tags:        []string{"secret"},
		MemoryTypes: []string{"npc_event"},
		Locations:   []string{"loc_outer_gate"},
		QuestIDs:    []string{"quest_entry"},
		Keywords:    []string{"linh can"},
		MaxResults:  3,
	}
	session := NewSession(game.NewStarterSave(game.NewGameRequest{Name: "Nam"}), plannedRetrievalClient{plan: plan}, store, &fakeStore{}, nil)

	_, err := session.HandleTurn(context.Background(), PlayerInput{Text: "ta hoi tham tin tuc"})
	if err != nil {
		t.Fatalf("HandleTurn returned error: %v", err)
	}
	want := memory.Query{
		SaveID:     session.Save().SaveID,
		Entities:   plan.Entities,
		Tags:       plan.Tags,
		Types:      plan.MemoryTypes,
		Locations:  plan.Locations,
		QuestIDs:   plan.QuestIDs,
		Keywords:   plan.Keywords,
		MaxResults: plan.MaxResults,
	}
	if !reflect.DeepEqual(store.query, want) {
		t.Fatalf("search query = %#v, want %#v", store.query, want)
	}
}

func TestHandleTurnRejectsEmptyInputWithoutAdvancingState(t *testing.T) {
	save := game.NewStarterSave(game.NewGameRequest{Name: "Nam"})
	session := NewSession(save, llm.FakeClient{}, &recordingStore{}, &fakeStore{}, nil)

	_, err := session.HandleTurn(context.Background(), PlayerInput{Text: " \t "})
	if err == nil {
		t.Fatal("HandleTurn accepted empty input")
	}
	after := session.Save()
	if after.CurrentTurn != save.CurrentTurn {
		t.Fatalf("turn = %d, want %d", after.CurrentTurn, save.CurrentTurn)
	}
	if after.Player.SpiritualEnergy != save.Player.SpiritualEnergy {
		t.Fatalf("energy = %d, want %d", after.Player.SpiritualEnergy, save.Player.SpiritualEnergy)
	}
}

func TestHandleTurnPersistsSnapshotAndEventInOrder(t *testing.T) {
	ctx := context.Background()
	memStore, err := memory.NewSQLiteStore(ctx, filepath.Join(t.TempDir(), "game.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer memStore.Close()

	save := game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})
	store := &fakeStore{}
	session := NewSession(save, llm.FakeClient{}, memStore, store, []string{"trust", "secret"})

	result, err := session.HandleTurn(ctx, PlayerInput{Text: "ta quan sat cong mon"})
	if err != nil {
		t.Fatalf("HandleTurn returned error: %v", err)
	}
	if result.Narration == "" {
		t.Fatal("expected narration")
	}
	if len(store.snapshots) != 1 || store.snapshots[0].CurrentTurn != 1 {
		t.Fatalf("snapshots = %#v, want one snapshot at turn 1", store.snapshots)
	}
	if len(store.events) != 1 || store.events[0].Turn != 1 || store.events[0].Type != storage.EventTypeTurnResolved {
		t.Fatalf("events = %#v, want one turn_resolved event at turn 1", store.events)
	}
	if !reflect.DeepEqual(store.callOrder, []string{"snapshot", "event"}) {
		t.Fatalf("call order = %#v, want snapshot before event", store.callOrder)
	}
}

func TestHandleTurnWarnsButDoesNotFailWhenPersistenceFails(t *testing.T) {
	ctx := context.Background()
	save := game.NewStarterSave(game.NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})
	store := &fakeStore{snapshotErr: errors.New("disk full"), eventErr: errors.New("disk full")}
	session := NewSession(save, llm.FakeClient{}, &recordingStore{}, store, nil)

	result, err := session.HandleTurn(ctx, PlayerInput{Text: "ta quan sat cong mon"})
	if err != nil {
		t.Fatalf("HandleTurn returned error: %v", err)
	}
	if result.Narration == "" {
		t.Fatal("expected narration despite persistence failure")
	}
	foundSnapshotWarning, foundEventWarning := false, false
	for _, w := range result.Warnings {
		if strings.Contains(w, "save persistence failed") {
			foundSnapshotWarning = true
		}
		if strings.Contains(w, "event log write failed") {
			foundEventWarning = true
		}
	}
	if !foundSnapshotWarning || !foundEventWarning {
		t.Fatalf("warnings = %#v, want both save and event failure warnings", result.Warnings)
	}
	if len(store.events) != 1 {
		t.Fatalf("events = %d, want AppendEvent still attempted once even though SaveSnapshot failed", len(store.events))
	}
}
