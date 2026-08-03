package memory

import (
	"context"
	"path/filepath"
	"testing"
)

func TestSQLiteStoreSearchesByTagEntityAndFTS(t *testing.T) {
	ctx := context.Background()
	store, err := NewSQLiteStore(ctx, filepath.Join(t.TempDir(), "game.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	err = store.Add(ctx, Memory{ID: "mem_1", SaveID: "save_1", CampaignID: "thanh-van-sect", Turn: 7, Type: "npc_event", Importance: 4, Text: "Luc Thanh Nghi biet bi mat linh can bien di.", Summary: "Luc Thanh Nghi knows the secret.", Entities: []string{"npc_luc_thanh_nghi", "player"}, Tags: []string{"secret", "trust"}})
	if err != nil {
		t.Fatal(err)
	}

	hits, err := store.Search(ctx, Query{SaveID: "save_1", Entities: []string{"npc_luc_thanh_nghi"}, Tags: []string{"secret"}, Keywords: []string{"linh can"}, MaxResults: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Memory.ID != "mem_1" {
		t.Fatalf("hits = %#v, want mem_1", hits)
	}
}
