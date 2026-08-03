package llm

import (
	"context"
	"fmt"
	"strings"

	"github.com/namtt/tutine-trpg/internal/game"
)

type FakeClient struct{}

func (FakeClient) PlanRetrieval(ctx context.Context, req PlannerRequest) (RetrievalPlan, error) {
	return RetrievalPlan{Intent: "recall_relevant_context", Entities: req.NearbyIDs, Tags: firstTags(req.AllowedTags, 3), Keywords: []string{req.PlayerAction}, TimeScope: "recent_or_important", MaxResults: 5}, nil
}

func (FakeClient) Narrate(ctx context.Context, req NarratorRequest) (NarratorResponse, error) {
	action := strings.TrimSpace(req.PlayerAction)
	if action == "" {
		action = "quan sát xung quanh"
	}
	return NarratorResponse{Narration: fmt.Sprintf("Bạn chọn: %s. Gió núi thổi qua cổng môn Thanh Vân Tông khi bạn cân nhắc hành động tiếp theo.", action), ProposedEffects: []game.Effect{{Type: game.EffectEnergyDelta, TargetID: "player", Amount: 0}}, SuggestedNextOptions: []string{"Quan sát xung quanh", "Hỏi đệ tử gác cổng", "Kiểm tra trạng thái"}}, nil
}

func (FakeClient) ExtractMemories(ctx context.Context, req ExtractorRequest) ([]MemoryDraft, error) {
	return []MemoryDraft{{Type: "turn_summary", Importance: 1, Entities: []string{"player"}, Tags: firstTags(req.AllowedTags, 1), Text: req.PlayerAction}}, nil
}

func firstTags(tags []string, n int) []string {
	if len(tags) < n {
		n = len(tags)
	}
	return append([]string{}, tags[:n]...)
}
