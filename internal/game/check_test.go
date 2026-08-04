package game

import "testing"

func testSave() SaveGame {
	return NewStarterSave(NewGameRequest{Name: "Nam", CampaignID: "thanh-van-sect"})
}

func TestResolveCheckRejectsUnknownStat(t *testing.T) {
	_, err := ResolveCheck(testSave(), CheckRequest{Stat: "charisma", Difficulty: 5}, 50)
	if err == nil {
		t.Fatal("expected error for unknown stat")
	}
}

func TestResolveCheckRejectsRollOutOfRange(t *testing.T) {
	if _, err := ResolveCheck(testSave(), CheckRequest{Stat: CheckStatLuck, Difficulty: 5}, 0); err == nil {
		t.Fatal("expected error for roll below range")
	}
	if _, err := ResolveCheck(testSave(), CheckRequest{Stat: CheckStatLuck, Difficulty: 5}, 101); err == nil {
		t.Fatal("expected error for roll above range")
	}
}

func TestResolveCheckThresholdIsClampedAndDeterministic(t *testing.T) {
	save := testSave() // Comprehension: 5
	result, err := ResolveCheck(save, CheckRequest{Stat: CheckStatComprehension, Difficulty: 0}, 50)
	if err != nil {
		t.Fatalf("ResolveCheck returned error: %v", err)
	}
	if result.Threshold != 50 {
		t.Fatalf("threshold = %d, want 50", result.Threshold)
	}
	if !result.Success {
		t.Fatalf("result = %#v, want success (roll == threshold)", result)
	}

	extreme, err := ResolveCheck(save, CheckRequest{Stat: CheckStatComprehension, Difficulty: 100}, 10)
	if err != nil {
		t.Fatalf("ResolveCheck returned error: %v", err)
	}
	if extreme.Threshold != 5 {
		t.Fatalf("threshold = %d, want clamped to 5", extreme.Threshold)
	}
	if extreme.Success {
		t.Fatalf("result = %#v, want failure once threshold clamped below roll", extreme)
	}
}

func TestResolveCheckSuccessFailureBoundary(t *testing.T) {
	save := testSave()
	pass, err := ResolveCheck(save, CheckRequest{Stat: CheckStatLuck, Difficulty: 10}, 40)
	if err != nil {
		t.Fatalf("ResolveCheck returned error: %v", err)
	}
	if !pass.Success || pass.Margin != 0 {
		t.Fatalf("pass = %#v, want success with zero margin", pass)
	}

	fail, err := ResolveCheck(save, CheckRequest{Stat: CheckStatLuck, Difficulty: 10}, 41)
	if err != nil {
		t.Fatalf("ResolveCheck returned error: %v", err)
	}
	if fail.Success || fail.Margin != -1 {
		t.Fatalf("fail = %#v, want failure with margin -1", fail)
	}
}
