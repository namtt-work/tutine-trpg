package game

import "testing"

func TestApplyEffectsRejectsUnknownItem(t *testing.T) {
	save := NewStarterSave(NewGameRequest{Name: "Nam", Traits: []string{"careful"}, CampaignID: "thanh-van-sect"})

	_, err := ApplyEffects(&save, []Effect{{Type: EffectGrantItem, TargetID: "player", ItemID: "heaven_sword", Amount: 1}})
	if err == nil {
		t.Fatal("expected unknown item to be rejected")
	}
	if len(save.Inventory) != 0 {
		t.Fatalf("inventory mutated after rejected effect: %#v", save.Inventory)
	}
}

func TestApplyEffectsClampsRelationshipDelta(t *testing.T) {
	save := NewStarterSave(NewGameRequest{Name: "Nam", Traits: []string{"careful"}, CampaignID: "thanh-van-sect"})

	changes, err := ApplyEffects(&save, []Effect{{Type: EffectRelationshipDelta, TargetID: "npc_luc_thanh_nghi", Amount: 99}})
	if err != nil {
		t.Fatalf("ApplyEffects returned error: %v", err)
	}
	if got := save.Player.Relationships["npc_luc_thanh_nghi"]; got != 3 {
		t.Fatalf("relationship = %d, want 3", got)
	}
	if len(changes) != 1 || changes[0].Amount != 3 {
		t.Fatalf("changes = %#v, want one clamped +3 change", changes)
	}
}

func TestApplyEffectsRejectsBatchWithoutPartialMutation(t *testing.T) {
	save := NewStarterSave(NewGameRequest{Name: "Nam", Traits: []string{"careful"}, CampaignID: "thanh-van-sect"})

	_, err := ApplyEffects(&save, []Effect{
		{Type: EffectGrantItem, TargetID: "player", ItemID: "moonlit_grass", Amount: 1},
		{Type: EffectGrantItem, TargetID: "player", ItemID: "heaven_sword", Amount: 1},
	})
	if err == nil {
		t.Fatal("expected invalid batch to be rejected")
	}
	if len(save.Inventory) != 0 {
		t.Fatalf("inventory mutated after rejected batch: %#v", save.Inventory)
	}
}

func TestApplyEffectsReportsActualEnergyDelta(t *testing.T) {
	save := NewStarterSave(NewGameRequest{Name: "Nam", Traits: []string{"careful"}, CampaignID: "thanh-van-sect"})
	save.Player.SpiritualEnergy = 19

	changes, err := ApplyEffects(&save, []Effect{{Type: EffectEnergyDelta, TargetID: "player", Amount: 20}})
	if err != nil {
		t.Fatalf("ApplyEffects returned error: %v", err)
	}
	if got := save.Player.SpiritualEnergy; got != 20 {
		t.Fatalf("energy = %d, want 20", got)
	}
	if len(changes) != 1 || changes[0].Amount != 1 {
		t.Fatalf("changes = %#v, want one actual +1 change", changes)
	}

	changes, err = ApplyEffects(&save, []Effect{{Type: EffectEnergyDelta, TargetID: "player", Amount: 20}})
	if err != nil {
		t.Fatalf("ApplyEffects returned error at energy cap: %v", err)
	}
	if len(changes) != 1 || changes[0].Amount != 0 {
		t.Fatalf("changes = %#v, want one zero change at energy cap", changes)
	}
}

func TestApplyEffectsRejectsEmptyTargetForPlayerOnlyEffects(t *testing.T) {
	tests := []Effect{
		{Type: EffectGrantItem, ItemID: "moonlit_grass", Amount: 1},
		{Type: EffectEnergyDelta, Amount: 1},
	}
	for _, effect := range tests {
		t.Run(effect.Type, func(t *testing.T) {
			save := NewStarterSave(NewGameRequest{Name: "Nam"})

			_, err := ApplyEffects(&save, []Effect{effect})
			if err == nil {
				t.Fatalf("ApplyEffects accepted empty target: %#v", effect)
			}
		})
	}
}

func TestApplyEffectsRejectsNonPlayerTargetForPlayerOnlyEffects(t *testing.T) {
	tests := []Effect{
		{Type: EffectGrantItem, TargetID: "npc_luc_thanh_nghi", ItemID: "moonlit_grass", Amount: 1},
		{Type: EffectEnergyDelta, TargetID: "npc_luc_thanh_nghi", Amount: 1},
	}
	for _, effect := range tests {
		t.Run(effect.Type, func(t *testing.T) {
			save := NewStarterSave(NewGameRequest{Name: "Nam"})

			_, err := ApplyEffects(&save, []Effect{effect})
			if err == nil {
				t.Fatalf("ApplyEffects accepted non-player target: %#v", effect)
			}
		})
	}
}

func TestApplyEffectsRejectsUnknownRelationshipTarget(t *testing.T) {
	tests := []string{"", "npc_unknown"}
	for _, targetID := range tests {
		t.Run(targetID, func(t *testing.T) {
			save := NewStarterSave(NewGameRequest{Name: "Nam"})

			_, err := ApplyEffects(&save, []Effect{{Type: EffectRelationshipDelta, TargetID: targetID, Amount: 1}})
			if err == nil {
				t.Fatalf("ApplyEffects accepted unknown relationship target %q", targetID)
			}
		})
	}
}
