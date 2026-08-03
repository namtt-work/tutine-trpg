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
	changes := make([]StateChangeView, 0, len(effects))
	for _, effect := range effects {
		switch effect.Type {
		case EffectGrantItem:
			if !starterAllowedItems[effect.ItemID] {
				return nil, fmt.Errorf("unknown or disallowed item %q", effect.ItemID)
			}
			if effect.Amount <= 0 || effect.Amount > 3 {
				return nil, fmt.Errorf("invalid item amount %d", effect.Amount)
			}
			save.Inventory[effect.ItemID] += effect.Amount
			changes = append(changes, StateChangeView{Type: effect.Type, TargetID: effect.ItemID, Amount: effect.Amount, Message: "nhan vat pham"})
		case EffectRelationshipDelta:
			amount := clamp(effect.Amount, -3, 3)
			save.Player.Relationships[effect.TargetID] += amount
			changes = append(changes, StateChangeView{Type: effect.Type, TargetID: effect.TargetID, Amount: amount, Message: "quan he thay doi"})
		case EffectEnergyDelta:
			amount := clamp(effect.Amount, -save.Player.MaxEnergy, save.Player.MaxEnergy)
			save.Player.SpiritualEnergy = clamp(save.Player.SpiritualEnergy+amount, 0, save.Player.MaxEnergy)
			changes = append(changes, StateChangeView{Type: effect.Type, TargetID: "player", Amount: amount, Message: "linh luc thay doi"})
		default:
			return nil, fmt.Errorf("unknown effect type %q", effect.Type)
		}
	}
	return changes, nil
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
