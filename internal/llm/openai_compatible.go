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
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
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
	err := c.chatJSON(ctx, "Return only JSON for a retrieval plan. Use keys matching the provided schema.", marshalPrompt(req), 0.1, &plan)
	return plan, err
}

func (c *OpenAICompatibleClient) Narrate(ctx context.Context, req NarratorRequest) (NarratorResponse, error) {
	var response NarratorResponse
	err := c.chatNarratorYAML(ctx, narratorSystemPrompt, marshalPrompt(req), 0.8, &response)
	return response, err
}

func (c *OpenAICompatibleClient) ExtractMemories(ctx context.Context, req ExtractorRequest) ([]MemoryDraft, error) {
	drafts := []MemoryDraft{}
	err := c.chatJSON(ctx, "Extract durable RPG memories from the resolved turn. Return only a JSON array of memory drafts using allowed tags.", marshalPrompt(req), 0.1, &drafts)
	return drafts, err
}

func (c *OpenAICompatibleClient) chatJSON(ctx context.Context, systemPrompt string, userPrompt string, temperature float64, out any) error {
	body, err := json.Marshal(chatRequest{
		Model:       c.model,
		Temperature: temperature,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
	})
	if err != nil {
		return err
	}

	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		content, err := c.sendChat(ctx, body)
		if err == nil {
			if err := json.Unmarshal([]byte(normalizeJSONContent(content)), out); err != nil {
				return fmt.Errorf("decode llm json: %w", err)
			}
			return nil
		}
		lastErr = err
		if !isTransientProviderError(err) || attempt == c.maxRetries {
			break
		}
	}
	return lastErr
}

func (c *OpenAICompatibleClient) chatNarratorYAML(ctx context.Context, systemPrompt string, userPrompt string, temperature float64, out *NarratorResponse) error {
	body, err := json.Marshal(chatRequest{
		Model:       c.model,
		Temperature: temperature,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
	})
	if err != nil {
		return err
	}

	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		content, err := c.sendChat(ctx, body)
		if err == nil {
			decoder := yaml.NewDecoder(strings.NewReader(normalizeYAMLContent(content)))
			decoder.KnownFields(true)
			if err := decoder.Decode(out); err != nil {
				return fmt.Errorf("decode llm yaml: %w", err)
			}
			if strings.TrimSpace(out.Narration) == "" {
				return errors.New("decode llm yaml: narration is required")
			}
			return nil
		}
		lastErr = err
		if !isTransientProviderError(err) || attempt == c.maxRetries {
			break
		}
	}
	return lastErr
}

func (c *OpenAICompatibleClient) sendChat(ctx context.Context, body []byte) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	data, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return "", readErr
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", providerStatusError{statusCode: resp.StatusCode, body: strings.TrimSpace(string(data))}
	}

	var parsed chatResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", fmt.Errorf("decode llm response: %w", err)
	}
	if len(parsed.Choices) == 0 || strings.TrimSpace(parsed.Choices[0].Message.Content) == "" {
		return "", errors.New("llm response missing message content")
	}
	return parsed.Choices[0].Message.Content, nil
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
	if !strings.HasPrefix(content, "```") {
		return content
	}
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
	return strings.TrimSpace(content)
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
