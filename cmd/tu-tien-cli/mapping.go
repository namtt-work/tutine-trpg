package main

import "github.com/namtt/tutine-trpg/internal/game"

// This file is the CLI's player-facing mapping layer: it turns internal
// engine IDs (scene, realm, effect type) into Vietnamese labels a player can
// read. Unknown IDs fall back to a generic label instead of leaking a raw
// identifier into the UI. Kept separate from internal/orchestrator's own
// mapping, which exists only to brief the narrator LLM.

const (
	unknownSceneLabel  = "Khung cảnh chưa xác định"
	unknownRealmLabel  = "Cảnh giới chưa xác định"
	unknownEffectLabel = "Thay đổi khác"
)

var sceneNames = map[string]string{
	"loc_outer_gate": "Ngoại môn Thanh Vân Tông",
}

var sceneIntros = map[string]string{
	"loc_outer_gate": "Bạn đứng trước cổng môn, mây phủ lưng chừng núi.",
}

var initialSceneSuggestions = map[string][]string{
	"loc_outer_gate": {"Quan sát cổng môn", "Hỏi đệ tử gác cổng", "Kiểm tra trạng thái"},
}

const defaultSceneIntro = "Câu chuyện của bạn bắt đầu."

var defaultInitialSuggestions = []string{"Quan sát xung quanh", "Kiểm tra trạng thái"}

var realmNames = map[string]string{
	"qi_refining": "Luyện Khí",
}

var effectLabels = map[string]string{
	game.EffectEnergyDelta:       "Linh lực",
	game.EffectRelationshipDelta: "Quan hệ",
	game.EffectGrantItem:         "Vật phẩm",
}

func sceneName(id string) string {
	if name, ok := sceneNames[id]; ok {
		return name
	}
	return unknownSceneLabel
}

func sceneIntro(id string) string {
	if intro, ok := sceneIntros[id]; ok {
		return intro
	}
	return defaultSceneIntro
}

func initialSuggestionsFor(id string) []string {
	if suggestions, ok := initialSceneSuggestions[id]; ok {
		return append([]string{}, suggestions...)
	}
	return append([]string{}, defaultInitialSuggestions...)
}

func realmName(id string) string {
	if name, ok := realmNames[id]; ok {
		return name
	}
	return unknownRealmLabel
}

func effectLabel(effectType string) string {
	if label, ok := effectLabels[effectType]; ok {
		return label
	}
	return unknownEffectLabel
}

func inventoryCount(save game.SaveGame) int {
	total := 0
	for _, amount := range save.Inventory {
		total += amount
	}
	return total
}
