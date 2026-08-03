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
