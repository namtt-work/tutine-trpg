package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type OpenAICompatibleConfig struct {
	BaseURL        string
	APIKeyEnv      string
	Model          string
	TimeoutSeconds int
	MaxRetries     int
}

type OpenAICompatibleClient struct {
	baseURL    string
	apiKey     string
	model      string
	maxRetries int
	httpClient *http.Client
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	Tools       []chatTool    `json:"tools,omitempty"`
	ToolChoice  string        `json:"tool_choice,omitempty"`
}

type chatMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content,omitempty"`
	ToolCalls  []chatToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

type chatTool struct {
	Type     string           `json:"type"`
	Function chatToolFunction `json:"function"`
}

type chatToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type chatToolCall struct {
	ID       string               `json:"id"`
	Type     string               `json:"type"`
	Function chatToolCallFunction `json:"function"`
}

type chatToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

const narratorSystemPrompt = `Bạn là người dẫn truyện và người điều khiển NPC cho một TRPG tu tiên bằng tiếng Việt. Bạn đang viết một lượt cảnh kế tiếp, không phải tóm tắt game, mô tả UI hay giải thích cho người chơi.

VAI TRÒ VÀ GIỚI HẠN
- Chỉ engine xác nhận state, scene, quest, reward, relationship và effect. Không tự tuyên bố các việc đó đã thay đổi nếu AuthoritativeState hoặc resolved_changes chưa xác nhận.
- AuthoritativeState là sự thật hiện tại. RecentTurns là diễn biến vừa xảy ra. RetrievedContext là lore/ký ức. Khi mâu thuẫn, ưu tiên theo đúng thứ tự đó.
- ID kỹ thuật như loc_outer_gate, npc_... hoặc tên kỹ năng nội bộ chỉ để hiểu context; tuyệt đối không chép chúng ra narration hay lời thoại.

LUẬT VIẾT MỖI LƯỢT
1. Đọc player_action, sau đó viết hệ quả của chính hành động ấy. Một hành động đã cung cấp thông tin thì NPC phải tiếp nhận, đánh giá, đáp lại hoặc hành động tiếp; không được hỏi người chơi nhắc lại chính thông tin đó.
2. Đẩy cảnh đi ít nhất một nhịp mới: NPC đưa ra quyết định có điều kiện, hé lộ thông tin, tạo áp lực/trở ngại, hoặc mở một lựa chọn cụ thể. Không chỉ đổi cách diễn đạt của cùng một câu hỏi.
3. Nối tiếp chi tiết chưa khép lại trong RecentTurns. Không quay về mô tả cổng, gió, ánh nắng, sự yên tĩnh hoặc thái độ "nhìn với tò mò" nếu chúng không làm thay đổi tình huống hiện tại.
4. Kể ở ngôi thứ hai ("bạn") hoặc gọi tên nhân vật. Không dùng "người chơi", "player", giọng thuyết minh game, hay lặp nguyên văn hành động của người chơi.
5. Dùng tiếng Việt thuần nhất, văn phong tiên hiệp tự nhiên: tông môn, đệ tử thủ môn, chấp sự, linh lực, tu vi, kinh mạch, pháp khí khi hợp cảnh. Không dùng "tu viện", "vệ sĩ", tiếng lóng hiện đại, fantasy phương Tây, tiếng Anh hoặc chữ Hán/CJK trong nội dung hiển thị.
6. Độ dài theo diễn biến: có thể dài khi cảnh cần đối thoại hay xung đột, nhưng mọi câu phải thêm hành động, thông tin, cảm xúc có căn cứ hoặc hậu quả. Không dùng văn sáo rỗng để kéo dài.

VÍ DỤ VỀ NHỊP ĐIỆU
Nếu lượt trước chấp sự đã hỏi tu vi và player_action trả lời "Luyện Khí tầng một, linh lực hai mươi phần", lượt này không được hỏi lại tu vi. Một phản hồi tốt là: chấp sự kiểm tra lệnh bài hoặc linh căn, nhận xét phù hợp, rồi đặt điều kiện hay chỉ ra bước kế tiếp. Khi state vẫn là cổng ngoài, có thể nói cổng mở hé để chờ kiểm tra, nhưng không nói bạn đã vào tông môn.

CÔNG CỤ
- Khi hành động của người chơi có kết quả không chắc chắn (thuyết phục, dò xét, né tránh, hành động rủi ro...), gọi công cụ roll_check với stat phù hợp và difficulty (0-20) TRƯỚC khi viết narration, rồi dùng success/roll/threshold trả về để mô tả đúng kết quả.
- Không tự bịa thành công hay thất bại của một việc rủi ro nếu chưa gọi roll_check. Nếu hành động chắc chắn thành công hoặc không có rủi ro, không cần gọi công cụ.

OUTPUT
- narration bắt buộc; chỉ gồm lời kể, không chép lại dialogue.
- npc_dialogue chỉ gồm lời trực tiếp của NPC; dùng [] khi không có.
- suggested_next_options gồm 2-3 hành động cụ thể, khác nhau và bám sát tình huống mới.
- proposed_effects chỉ dùng effect trong allowed_effects khi context đủ căn cứ. Mỗi effect dùng chính xác type, target_id, amount; grant_item cần thêm item_id. Nếu không có effect hợp lệ, dùng [] và không tạo entry rỗng.

Trả lời hoàn toàn bằng tiếng Việt và chỉ bằng YAML. Mọi string phải đặt trong dấu ngoặc kép trên một dòng. Không Markdown, không block scalar, không giải thích.
narration: "Lời kể của khung cảnh."
npc_dialogue:
  - npc_id: npc_guard
    text: "Dừng lại."
proposed_effects: []
suggested_next_options:
  - "Trình lệnh bài."
`

const plannerSystemPrompt = `Bạn là bộ lập kế hoạch truy xuất trí nhớ cho một TRPG tu tiên. Nhiệm vụ duy nhất: đọc player_action và scene_id rồi chọn tiêu chí truy vấn trí nhớ liên quan.

QUY TẮC
- Chỉ dùng tag nằm trong allowed_tags; nếu không có tag phù hợp, để tags rỗng.
- entities, locations, quest_ids, keywords chỉ là ID hoặc từ khóa ngắn, không viết thành câu.
- max_results là số nguyên từ 1 đến 8.

Chỉ trả lời bằng một object JSON thuần, không giải thích, không Markdown, không văn bản nào khác ngoài JSON, đúng các khóa sau:
{"intent": "...", "entities": [], "tags": [], "memory_types": [], "locations": [], "quest_ids": [], "keywords": [], "time_scope": "...", "max_results": 4}
`

const extractorSystemPrompt = `Bạn trích xuất trí nhớ dài hạn từ một lượt chơi TRPG tu tiên đã được engine xác nhận. resolved_changes là sự thật, narration chỉ để lấy ngữ cảnh diễn đạt.

QUY TẮC
- Chỉ ghi lại sự kiện có ý nghĩa lâu dài: quan hệ thay đổi, vật phẩm, lời hứa, manh mối, thông tin quan trọng. Nếu lượt chơi không có gì đáng nhớ, trả về mảng rỗng.
- Chỉ dùng tag nằm trong allowed_tags.
- text viết ngắn gọn bằng tiếng Việt, ở thì đã xảy ra, không chép lại nguyên văn narration.
- importance là số nguyên từ 1 (nhỏ) đến 5 (quan trọng).

Chỉ trả lời bằng một mảng JSON thuần, không giải thích, không Markdown, không văn bản nào khác ngoài JSON. Mỗi phần tử đúng dạng:
{"type": "turn_summary", "importance": 2, "entities": [], "tags": [], "facts_json": "[]", "text": "..."}
Nếu không có gì để ghi nhớ, chỉ trả lời: []
`

func NewOpenAICompatibleClient(cfg OpenAICompatibleConfig) (*OpenAICompatibleClient, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, errors.New("llm base_url is required")
	}
	if strings.TrimSpace(cfg.APIKeyEnv) == "" {
		return nil, errors.New("llm api_key_env is required")
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return nil, errors.New("llm model is required")
	}
	apiKey := strings.TrimSpace(os.Getenv(cfg.APIKeyEnv))
	if apiKey == "" {
		return nil, fmt.Errorf("llm api key environment variable %s is not set", cfg.APIKeyEnv)
	}
	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 45 * time.Second
	}
	return &OpenAICompatibleClient{baseURL: strings.TrimRight(cfg.BaseURL, "/"), apiKey: apiKey, model: cfg.Model, maxRetries: cfg.MaxRetries, httpClient: &http.Client{Timeout: timeout}}, nil
}

func (c *OpenAICompatibleClient) PlanRetrieval(ctx context.Context, req PlannerRequest) (RetrievalPlan, error) {
	var plan RetrievalPlan
	err := c.chatJSON(ctx, plannerSystemPrompt, marshalPrompt(req), 0.1, &plan)
	return plan, err
}

func (c *OpenAICompatibleClient) Narrate(ctx context.Context, req NarratorRequest) (NarratorResponse, error) {
	var response NarratorResponse
	err := c.chatNarratorYAML(ctx, marshalPrompt(req), 0.8, nil, nil, &response)
	return response, err
}

// NarrateWithTools drives an iterative tool-calling loop: the model may call
// one of tools zero or more times, each call executed against the
// authoritative game engine via exec, before it produces the final
// narration. This is what lets the LLM "see" an engine result (e.g. a
// probability check) and react to it, instead of guessing blind.
func (c *OpenAICompatibleClient) NarrateWithTools(ctx context.Context, req NarratorRequest, tools []ToolDefinition, exec ToolExecutor) (NarratorResponse, error) {
	var response NarratorResponse
	err := c.chatNarratorYAML(ctx, marshalPrompt(req), 0.8, toChatTools(tools), exec, &response)
	return response, err
}

func (c *OpenAICompatibleClient) ExtractMemories(ctx context.Context, req ExtractorRequest) ([]MemoryDraft, error) {
	drafts := []MemoryDraft{}
	err := c.chatJSON(ctx, extractorSystemPrompt, marshalPrompt(req), 0.1, &drafts)
	return drafts, err
}

func (c *OpenAICompatibleClient) chatJSON(ctx context.Context, systemPrompt string, userPrompt string, temperature float64, out any) error {
	return c.runChat(ctx, systemPrompt, userPrompt, temperature, nil, nil, func(content string) error {
		if err := json.Unmarshal([]byte(normalizeJSONContent(content)), out); err != nil {
			return fmt.Errorf("decode llm json: %w", err)
		}
		return nil
	})
}

func (c *OpenAICompatibleClient) chatNarratorYAML(ctx context.Context, userPrompt string, temperature float64, tools []chatTool, exec ToolExecutor, out *NarratorResponse) error {
	return c.runChat(ctx, narratorSystemPrompt, userPrompt, temperature, tools, exec, func(content string) error {
		decoder := yaml.NewDecoder(strings.NewReader(normalizeYAMLContent(content)))
		if err := decoder.Decode(out); err != nil {
			return fmt.Errorf("decode llm yaml: %w", err)
		}
		if strings.TrimSpace(out.Narration) == "" {
			return errors.New("decode llm yaml: narration is required")
		}
		return nil
	})
}

// maxToolCallRounds bounds how many tool-call round trips a single runChat
// call will make on top of c.maxRetries, so a model stuck calling tools
// (or a broken executor) can't loop forever burning tokens.
const maxToolCallRounds = 6

// runChat sends the chat request and hands each final (non-tool-call)
// response to decode. Transient provider errors (rate limit, 5xx) are
// retried unchanged. A decode failure is retried too: the malformed reply
// and a correction instruction are appended to the conversation so the
// model can fix its own output instead of losing the whole turn to a single
// formatting slip. When the model responds with tool_calls instead, each
// call is executed via exec and its result is fed back as a "tool" message
// before asking the model again.
func (c *OpenAICompatibleClient) runChat(ctx context.Context, systemPrompt string, userPrompt string, temperature float64, tools []chatTool, exec ToolExecutor, decode func(content string) error) error {
	messages := []chatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}
	toolChoice := ""
	if len(tools) > 0 {
		toolChoice = "auto"
	}

	maxSteps := c.maxRetries + maxToolCallRounds
	var lastErr error
	for step := 0; step <= maxSteps; step++ {
		body, err := json.Marshal(chatRequest{Model: c.model, Temperature: temperature, Messages: messages, Tools: tools, ToolChoice: toolChoice})
		if err != nil {
			return err
		}

		msg, err := c.sendChatMessage(ctx, body)
		if err != nil {
			lastErr = err
			if !isTransientProviderError(err) || step == maxSteps {
				return lastErr
			}
			continue
		}

		if len(msg.ToolCalls) > 0 {
			if exec == nil {
				return fmt.Errorf("llm requested tool %q but no executor is configured", msg.ToolCalls[0].Function.Name)
			}
			messages = append(messages, msg)
			for _, call := range msg.ToolCalls {
				result, execErr := exec(ctx, ToolCall{ID: call.ID, Name: call.Function.Name, Arguments: json.RawMessage(call.Function.Arguments)})
				content := result.Content
				if execErr != nil {
					content = fmt.Sprintf("lỗi công cụ: %v", execErr)
				}
				messages = append(messages, chatMessage{Role: "tool", ToolCallID: call.ID, Content: content})
			}
			continue
		}

		if decodeErr := decode(msg.Content); decodeErr != nil {
			lastErr = decodeErr
			if step == maxSteps {
				return lastErr
			}
			messages = append(messages,
				msg,
				chatMessage{Role: "user", Content: fmt.Sprintf("Phản hồi trước không hợp lệ (%v). Chỉ trả lời lại đúng định dạng đã yêu cầu, không thêm giải thích hay văn bản nào khác.", decodeErr)},
			)
			continue
		}
		return nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("vượt quá số bước gọi công cụ tối đa (%d)", maxSteps)
	}
	return lastErr
}

func (c *OpenAICompatibleClient) sendChatMessage(ctx context.Context, body []byte) (chatMessage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return chatMessage{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return chatMessage{}, err
	}
	defer resp.Body.Close()

	data, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return chatMessage{}, readErr
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return chatMessage{}, providerStatusError{statusCode: resp.StatusCode, body: strings.TrimSpace(string(data))}
	}

	var parsed chatResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return chatMessage{}, fmt.Errorf("decode llm response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return chatMessage{}, errors.New("llm response missing choices")
	}
	msg := parsed.Choices[0].Message
	if strings.TrimSpace(msg.Content) == "" && len(msg.ToolCalls) == 0 {
		return chatMessage{}, errors.New("llm response missing message content")
	}
	msg.Role = "assistant"
	return msg, nil
}

func toChatTools(tools []ToolDefinition) []chatTool {
	if len(tools) == 0 {
		return nil
	}
	chatTools := make([]chatTool, len(tools))
	for i, tool := range tools {
		chatTools[i] = chatTool{Type: "function", Function: chatToolFunction{Name: tool.Name, Description: tool.Description, Parameters: tool.Parameters}}
	}
	return chatTools
}

type providerStatusError struct {
	statusCode int
	body       string
}

func (e providerStatusError) Error() string {
	if e.body == "" {
		return fmt.Sprintf("llm provider returned status %d", e.statusCode)
	}
	return fmt.Sprintf("llm provider returned status %d: %s", e.statusCode, e.body)
}

func isTransientProviderError(err error) bool {
	var statusErr providerStatusError
	if errors.As(err, &statusErr) {
		return statusErr.statusCode == http.StatusTooManyRequests || statusErr.statusCode >= 500
	}
	return false
}

func marshalPrompt(value any) string {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Sprintf("%+v", value)
	}
	return string(data)
}

func normalizeJSONContent(content string) string {
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "```") {
		content = strings.TrimPrefix(content, "```")
		content = strings.TrimSpace(content)
		if newline := strings.IndexByte(content, '\n'); newline >= 0 {
			language := strings.TrimSpace(content[:newline])
			if language == "" || strings.EqualFold(language, "json") {
				content = content[newline+1:]
			}
		}
		content = strings.TrimSpace(content)
		content = strings.TrimSuffix(content, "```")
		content = strings.TrimSpace(content)
	}
	if strings.HasPrefix(content, "{") || strings.HasPrefix(content, "[") {
		return content
	}
	// Some models wrap the JSON in a stray sentence despite instructions
	// (e.g. "Đây là kết quả: {...}"). Pull out the first balanced
	// object/array instead of failing the whole decode on a preface.
	if extracted, ok := extractJSONValue(content); ok {
		return extracted
	}
	return content
}

// extractJSONValue returns the first balanced {...} or [...] substring in
// content, ignoring braces/brackets that appear inside JSON string literals.
func extractJSONValue(content string) (string, bool) {
	start := strings.IndexAny(content, "{[")
	if start < 0 {
		return "", false
	}
	open := content[start]
	close := byte('}')
	if open == '[' {
		close = ']'
	}
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(content); i++ {
		ch := content[i]
		if inString {
			switch {
			case escaped:
				escaped = false
			case ch == '\\':
				escaped = true
			case ch == '"':
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case open:
			depth++
		case close:
			depth--
			if depth == 0 {
				return content[start : i+1], true
			}
		}
	}
	return "", false
}

func normalizeYAMLContent(content string) string {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "```") {
		return content
	}

	trimmed := strings.TrimSpace(strings.TrimPrefix(content, "```"))
	newline := strings.IndexByte(trimmed, '\n')
	if newline < 0 {
		return content
	}
	language := strings.TrimSpace(trimmed[:newline])
	if !strings.EqualFold(language, "yaml") && !strings.EqualFold(language, "yml") {
		return content
	}
	return strings.TrimSpace(strings.TrimSuffix(trimmed[newline+1:], "```"))
}
