package llm

import (
	"context"
	"testing"
)

func TestFakeClientReturnsDeterministicNarration(t *testing.T) {
	client := FakeClient{}
	resp, err := client.Narrate(context.Background(), NarratorRequest{PlayerAction: "ta quan sat cong mon"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Narration == "" || len(resp.SuggestedNextOptions) == 0 {
		t.Fatalf("unexpected response: %#v", resp)
	}
}
