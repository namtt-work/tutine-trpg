package llm

import (
	"context"
	"reflect"
	"testing"
)

func TestFakeClientReturnsDeterministicNarration(t *testing.T) {
	client := FakeClient{}
	request := NarratorRequest{PlayerAction: "ta quan sat cong mon"}
	resp, err := client.Narrate(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := client.Narrate(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(resp, second) {
		t.Fatalf("responses differ: %#v and %#v", resp, second)
	}
	if resp.Narration == "" || len(resp.SuggestedNextOptions) == 0 {
		t.Fatalf("unexpected response: %#v", resp)
	}
}
