package campaign

import "testing"

func TestLoadCampaign(t *testing.T) {
	camp, err := Load("../../campaigns/thanh-van-sect")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if camp.ID != "thanh-van-sect" {
		t.Fatalf("campaign ID = %q", camp.ID)
	}
	if !camp.HasTag("trust") || camp.HasTag("invented_tag") {
		t.Fatalf("tag vocabulary not loaded correctly: %#v", camp.Tags)
	}
}
