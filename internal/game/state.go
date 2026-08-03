package game

import (
	"fmt"
	"sync/atomic"
	"time"
)

var saveSequence uint64

type NewGameRequest struct {
	Name       string
	Traits     []string
	CampaignID string
}

type SaveGame struct {
	SaveID       string          `json:"save_id"`
	CampaignID   string          `json:"campaign_id"`
	CurrentTurn  int             `json:"current_turn"`
	CurrentScene string          `json:"current_scene"`
	Player       Player          `json:"player"`
	Inventory    map[string]int  `json:"inventory"`
	WorldFlags   map[string]bool `json:"world_flags"`
	Cooldowns    map[string]int  `json:"cooldowns"`
	CreatedAt    time.Time       `json:"created_at"`
}

type Player struct {
	ID              string         `json:"id"`
	Name            string         `json:"name"`
	Traits          []string       `json:"traits"`
	Realm           string         `json:"realm"`
	Stage           int            `json:"stage"`
	HP              int            `json:"hp"`
	MaxHP           int            `json:"max_hp"`
	SpiritualEnergy int            `json:"spiritual_energy"`
	MaxEnergy       int            `json:"max_spiritual_energy"`
	Stats           Stats          `json:"stats"`
	Techniques      []string       `json:"techniques"`
	Artifacts       []string       `json:"artifacts"`
	Relationships   map[string]int `json:"relationships"`
}

type Stats struct {
	Attack        int `json:"attack"`
	Defense       int `json:"defense"`
	Speed         int `json:"speed"`
	Comprehension int `json:"comprehension"`
	Luck          int `json:"luck"`
}

type StateChangeView struct {
	Type     string `json:"type"`
	TargetID string `json:"target_id"`
	Amount   int    `json:"amount"`
	Message  string `json:"message"`
}

type TurnResult struct {
	Narration        string            `json:"narration"`
	StateChanges     []StateChangeView `json:"state_changes"`
	SuggestedActions []string          `json:"suggested_actions"`
	Warnings         []string          `json:"warnings"`
	NeedsInput       *InputRequest     `json:"needs_input,omitempty"`
}

type InputRequest struct {
	Prompt  string   `json:"prompt"`
	Options []string `json:"options"`
}

func NewStarterSave(req NewGameRequest) SaveGame {
	name := req.Name
	if name == "" {
		name = "Vô Danh"
	}
	campaignID := req.CampaignID
	if campaignID == "" {
		campaignID = "thanh-van-sect"
	}
	return SaveGame{
		SaveID:       newSaveID(),
		CampaignID:   campaignID,
		CurrentTurn:  0,
		CurrentScene: "loc_outer_gate",
		Inventory:    map[string]int{},
		WorldFlags:   map[string]bool{},
		Cooldowns:    map[string]int{},
		CreatedAt:    time.Now().UTC(),
		Player: Player{
			ID:              "player",
			Name:            name,
			Traits:          append([]string{}, req.Traits...),
			Realm:           "qi_refining",
			Stage:           1,
			HP:              30,
			MaxHP:           30,
			SpiritualEnergy: 20,
			MaxEnergy:       20,
			Stats:           Stats{Attack: 6, Defense: 4, Speed: 5, Comprehension: 5, Luck: 5},
			Techniques:      []string{"basic_strike"},
			Artifacts:       []string{},
			Relationships:   map[string]int{"npc_luc_thanh_nghi": 0},
		},
	}
}

func (save SaveGame) Clone() SaveGame {
	save.Inventory = cloneIntMap(save.Inventory)
	save.WorldFlags = cloneBoolMap(save.WorldFlags)
	save.Cooldowns = cloneIntMap(save.Cooldowns)
	save.Player.Traits = append([]string(nil), save.Player.Traits...)
	save.Player.Techniques = append([]string(nil), save.Player.Techniques...)
	save.Player.Artifacts = append([]string(nil), save.Player.Artifacts...)
	save.Player.Relationships = cloneIntMap(save.Player.Relationships)
	return save
}

func newSaveID() string {
	return fmt.Sprintf("save_%d_%d", time.Now().UTC().UnixNano(), atomic.AddUint64(&saveSequence, 1))
}

func cloneIntMap(values map[string]int) map[string]int {
	if values == nil {
		return nil
	}
	clone := make(map[string]int, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}

func cloneBoolMap(values map[string]bool) map[string]bool {
	if values == nil {
		return nil
	}
	clone := make(map[string]bool, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}
