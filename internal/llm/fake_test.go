package llm

import (
	"context"
	"reflect"
	"strings"
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

func TestFakeClientNarrationIncludesPlayerAction(t *testing.T) {
	client := FakeClient{}
	resp, err := client.Narrate(context.Background(), NarratorRequest{PlayerAction: "Kiểm tra trạng thái"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp.Narration, "Kiểm tra trạng thái") {
		t.Fatalf("narration %q does not include player action", resp.Narration)
	}
}
