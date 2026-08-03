package game

import (
	"errors"
	"fmt"
)

const (
	EffectGrantItem         = "grant_item"
	EffectRelationshipDelta = "relationship_delta"
	EffectEnergyDelta       = "energy_delta"
)

var starterAllowedItems = map[string]bool{
	"moonlit_grass":    true,
	"low_spirit_stone": true,
}

type Effect struct {
	Type     string `json:"type"`
	TargetID string `json:"target_id"`
	ItemID   string `json:"item_id,omitempty"`
	Amount   int    `json:"amount"`
	Reason   string `json:"reason,omitempty"`
}

func ApplyEffects(save *SaveGame, effects []Effect) ([]StateChangeView, error) {
	if save == nil {
		return nil, errors.New("save is nil")
	}
	working := cloneSave(*save)
	changes := make([]StateChangeView, 0, len(effects))
	for _, effect := range effects {
		switch effect.Type {
		case EffectGrantItem:
			if err := validateItemEffect(effect); err != nil {
				return nil, err
			}
			working.Inventory[effect.ItemID] += effect.Amount
			changes = append(changes, StateChangeView{Type: effect.Type, TargetID: effect.ItemID, Amount: effect.Amount, Message: "nhan vat pham"})
		case EffectRelationshipDelta:
			amount := clamp(effect.Amount, -3, 3)
			working.Player.Relationships[effect.TargetID] += amount
			changes = append(changes, StateChangeView{Type: effect.Type, TargetID: effect.TargetID, Amount: amount, Message: "quan he thay doi"})
		case EffectEnergyDelta:
			oldEnergy := working.Player.SpiritualEnergy
			requested := clamp(effect.Amount, -working.Player.MaxEnergy, working.Player.MaxEnergy)
			working.Player.SpiritualEnergy = clamp(oldEnergy+requested, 0, working.Player.MaxEnergy)
			amount := working.Player.SpiritualEnergy - oldEnergy
			changes = append(changes, StateChangeView{Type: effect.Type, TargetID: "player", Amount: amount, Message: "linh luc thay doi"})
		default:
			return nil, fmt.Errorf("unknown effect type %q", effect.Type)
		}
	}
	*save = working
	return changes, nil
}

func validateItemEffect(effect Effect) error {
	if !starterAllowedItems[effect.ItemID] {
		return fmt.Errorf("unknown or disallowed item %q", effect.ItemID)
	}
	if effect.Amount <= 0 || effect.Amount > 3 {
		return fmt.Errorf("invalid item amount %d", effect.Amount)
	}
	return nil
}

func cloneSave(save SaveGame) SaveGame {
	save.Inventory = cloneIntMap(save.Inventory)
	save.WorldFlags = cloneBoolMap(save.WorldFlags)
	save.Cooldowns = cloneIntMap(save.Cooldowns)
	save.Player.Traits = append([]string(nil), save.Player.Traits...)
	save.Player.Techniques = append([]string(nil), save.Player.Techniques...)
	save.Player.Artifacts = append([]string(nil), save.Player.Artifacts...)
	save.Player.Relationships = cloneIntMap(save.Player.Relationships)
	return save
}

func cloneIntMap(values map[string]int) map[string]int {
	clone := make(map[string]int, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}

func cloneBoolMap(values map[string]bool) map[string]bool {
	clone := make(map[string]bool, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}

func clamp(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
