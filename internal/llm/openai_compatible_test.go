package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
)

func TestNewOpenAICompatibleClientRejectsMissingAPIKey(t *testing.T) {
	t.Setenv("MISSING_LLM_KEY", "")

	_, err := NewOpenAICompatibleClient(OpenAICompatibleConfig{
		BaseURL:        "https://api.groq.com/openai/v1",
		APIKeyEnv:      "MISSING_LLM_KEY",
		Model:          "llama-3.1-70b-versatile",
		TimeoutSeconds: 45,
		MaxRetries:     2,
	})
	if err == nil || !strings.Contains(err.Error(), "MISSING_LLM_KEY") {
		t.Fatalf("err = %v, want missing key env error", err)
	}
}

func TestOpenAICompatibleClientPlansRetrievalWithBearerToken(t *testing.T) {
	t.Setenv("TEST_LLM_KEY", "secret-test-key")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret-test-key" {
			t.Fatalf("Authorization = %q", got)
		}
		writeChatContent(t, w, `{"intent":"recall_relevant_context","entities":["player"],"tags":["trust"],"memory_types":[],"locations":[],"quest_ids":[],"keywords":["gate"],"time_scope":"recent_or_important","max_results":4}`)
	}))
	defer server.Close()

	client := newTestOpenAIClient(t, server.URL)
	plan, err := client.PlanRetrieval(context.Background(), PlannerRequest{PlayerAction: "ta quan sat", SceneID: "loc_outer_gate", AllowedTags: []string{"trust"}})
	if err != nil {
		t.Fatalf("PlanRetrieval returned error: %v", err)
	}
	if plan.Intent != "recall_relevant_context" || len(plan.Keywords) != 1 || plan.Keywords[0] != "gate" || plan.MaxResults != 4 {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestOpenAICompatibleClientNarratesYAMLResponse(t *testing.T) {
	t.Setenv("TEST_LLM_KEY", "secret-test-key")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request chatRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if len(request.Messages) == 0 || !strings.Contains(request.Messages[0].Content, "narration:") || !strings.Contains(request.Messages[0].Content, "npc_dialogue:") || !strings.Contains(request.Messages[0].Content, "Trả lời hoàn toàn bằng tiếng Việt") || !strings.Contains(request.Messages[0].Content, "văn phong tiên hiệp") || !strings.Contains(request.Messages[0].Content, "Độ dài theo diễn biến") || !strings.Contains(request.Messages[0].Content, "không được hỏi người chơi nhắc lại") || !strings.Contains(request.Messages[0].Content, "Không dùng \"người chơi\"") || !strings.Contains(request.Messages[0].Content, "tu viện") || !strings.Contains(request.Messages[0].Content, "chữ Hán/CJK") || !strings.Contains(request.Messages[0].Content, "AuthoritativeState") || !strings.Contains(request.Messages[0].Content, "RecentTurns") || !strings.Contains(request.Messages[0].Content, "RetrievedContext") || !strings.Contains(request.Messages[0].Content, "target_id") {
			t.Fatalf("system prompt = %q, want grounded progressive YAML response shape", request.Messages[0].Content)
		}
		writeChatContent(t, w, `narration: Gió núi nổi lên.
npc_dialogue:
  - npc_id: npc_guard
    text: Đứng lại.
proposed_effects:
  - type: energy_delta
    target_id: player
    amount: 0
suggested_next_options:
  - Quan sát`)
	}))
	defer server.Close()

	client := newTestOpenAIClient(t, server.URL)
	response, err := client.Narrate(context.Background(), NarratorRequest{PlayerAction: "ta quan sat", SceneID: "loc_outer_gate"})
	if err != nil {
		t.Fatalf("Narrate returned error: %v", err)
	}
	if response.Narration != "Gió núi nổi lên." || len(response.NPCDialogue) != 1 || response.NPCDialogue[0].NPCID != "npc_guard" || len(response.ProposedEffects) != 1 || response.ProposedEffects[0].TargetID != "player" || len(response.SuggestedNextOptions) != 1 || response.SuggestedNextOptions[0] != "Quan sát" {
		t.Fatalf("response = %#v", response)
	}
}

func TestOpenAICompatibleClientAcceptsFencedYAMLContent(t *testing.T) {
	t.Setenv("TEST_LLM_KEY", "secret-test-key")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeChatContent(t, w, "```yaml\nnarration: Mây tím tụ lại.\nnpc_dialogue: []\nproposed_effects: []\nsuggested_next_options:\n  - Tiến lên\n```")
	}))
	defer server.Close()

	client := newTestOpenAIClient(t, server.URL)
	response, err := client.Narrate(context.Background(), NarratorRequest{PlayerAction: "ta tiến lên", SceneID: "loc_outer_gate"})
	if err != nil {
		t.Fatalf("Narrate returned error: %v", err)
	}
	if response.Narration != "Mây tím tụ lại." || len(response.SuggestedNextOptions) != 1 || response.SuggestedNextOptions[0] != "Tiến lên" {
		t.Fatalf("response = %#v", response)
	}
}

func TestOpenAICompatibleClientRejectsMalformedYAMLContent(t *testing.T) {
	t.Setenv("TEST_LLM_KEY", "secret-test-key")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeChatContent(t, w, "narration: [không hợp lệ")
	}))
	defer server.Close()

	client := newTestOpenAIClient(t, server.URL)
	_, err := client.Narrate(context.Background(), NarratorRequest{PlayerAction: "ta bước tới", SceneID: "loc_outer_gate"})
	if err == nil || !strings.Contains(err.Error(), "decode llm yaml") {
		t.Fatalf("err = %v, want decode llm yaml error", err)
	}
}

func TestOpenAICompatibleClientRejectsYAMLWithoutNarration(t *testing.T) {
	t.Setenv("TEST_LLM_KEY", "secret-test-key")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeChatContent(t, w, `npc_dialogue:
  - npc_id: npc_guard
    text: "Trình lệnh bài."
proposed_effects: []
suggested_next_options: []`)
	}))
	defer server.Close()

	client := newTestOpenAIClient(t, server.URL)
	_, err := client.Narrate(context.Background(), NarratorRequest{PlayerAction: "ta bước tới", SceneID: "loc_outer_gate"})
	if err == nil || !strings.Contains(err.Error(), "decode llm yaml") {
		t.Fatalf("err = %v, want decode llm yaml error", err)
	}
}

func TestOpenAICompatibleClientExtractsMemoryDrafts(t *testing.T) {
	t.Setenv("TEST_LLM_KEY", "secret-test-key")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeChatContent(t, w, `[{"type":"turn_summary","importance":2,"entities":["player"],"tags":["trust"],"facts_json":"[]","text":"Người chơi quan sát cổng môn."}]`)
	}))
	defer server.Close()

	client := newTestOpenAIClient(t, server.URL)
	drafts, err := client.ExtractMemories(context.Background(), ExtractorRequest{PlayerAction: "ta quan sat", Narration: "Gió núi nổi lên."})
	if err != nil {
		t.Fatalf("ExtractMemories returned error: %v", err)
	}
	if len(drafts) != 1 || drafts[0].Type != "turn_summary" || drafts[0].Importance != 2 {
		t.Fatalf("drafts = %#v", drafts)
	}
}

func TestOpenAICompatibleClientRetriesRateLimit(t *testing.T) {
	t.Setenv("TEST_LLM_KEY", "secret-test-key")
	var attempts int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&attempts, 1) == 1 {
			http.Error(w, "rate limited", http.StatusTooManyRequests)
			return
		}
		writeChatContent(t, w, `{"intent":"recall_relevant_context","entities":[],"tags":[],"memory_types":[],"locations":[],"quest_ids":[],"keywords":[],"time_scope":"recent_or_important","max_results":1}`)
	}))
	defer server.Close()

	client := newTestOpenAIClient(t, server.URL)
	client.maxRetries = 1
	_, err := client.PlanRetrieval(context.Background(), PlannerRequest{PlayerAction: "ta quan sat"})
	if err != nil {
		t.Fatalf("PlanRetrieval returned error: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
}

func TestOpenAICompatibleClientRejectsInvalidJSONContent(t *testing.T) {
	t.Setenv("TEST_LLM_KEY", "secret-test-key")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeChatContent(t, w, `not json`)
	}))
	defer server.Close()

	client := newTestOpenAIClient(t, server.URL)
	_, err := client.PlanRetrieval(context.Background(), PlannerRequest{PlayerAction: "ta quan sat"})
	if err == nil || !strings.Contains(err.Error(), "decode llm json") {
		t.Fatalf("err = %v, want decode llm json error", err)
	}
}

func TestOpenAICompatibleClientIntegrationNarrate(t *testing.T) {
	if os.Getenv("TUTINE_LLM_INTEGRATION") != "1" {
		t.Skip("set TUTINE_LLM_INTEGRATION=1 to run real provider smoke test")
	}
	model := os.Getenv("GROQ_MODEL")
	if model == "" {
		model = "llama-3.3-70b-versatile"
	}
	client, err := NewOpenAICompatibleClient(OpenAICompatibleConfig{BaseURL: "https://api.groq.com/openai/v1", APIKeyEnv: "GROQ_API_KEY", Model: model, TimeoutSeconds: 45, MaxRetries: 1})
	if err != nil {
		t.Fatalf("NewOpenAICompatibleClient returned error: %v", err)
	}
	response, err := client.Narrate(context.Background(), NarratorRequest{PlayerAction: "ta quan sát cổng môn", SceneID: "loc_outer_gate", StateSummary: "Nam qi_refining stage 1", AllowedEffects: []string{"energy_delta", "relationship_delta", "grant_item"}})
	if err != nil {
		t.Fatalf("Narrate integration returned error: %v", err)
	}
	if strings.TrimSpace(response.Narration) == "" {
		t.Fatalf("Narration is empty: %#v", response)
	}
}

func newTestOpenAIClient(t *testing.T, baseURL string) *OpenAICompatibleClient {
	t.Helper()
	client, err := NewOpenAICompatibleClient(OpenAICompatibleConfig{BaseURL: baseURL, APIKeyEnv: "TEST_LLM_KEY", Model: "test-model", TimeoutSeconds: 5})
	if err != nil {
		t.Fatalf("NewOpenAICompatibleClient returned error: %v", err)
	}
	return client
}

func writeChatContent(t *testing.T, w http.ResponseWriter, content string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	data, err := json.Marshal(chatResponse{Choices: []struct {
		Message chatMessage `json:"message"`
	}{{Message: chatMessage{Role: "assistant", Content: content}}}})
	if err != nil {
		t.Fatalf("marshal chat response: %v", err)
	}
	_, _ = w.Write(data)
}

func equalStringSlices(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func equalDialogueLines(left []DialogueLine, right []DialogueLine) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
