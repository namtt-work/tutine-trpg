package main

import "testing"

func TestBuildOfflineSession(t *testing.T) {
	session, cleanup, err := buildOfflineSession(t.TempDir(), "Nam")
	if err != nil {
		t.Fatalf("buildOfflineSession returned error: %v", err)
	}
	defer cleanup()
	if session.Save().Player.Name != "Nam" {
		t.Fatalf("player name = %q, want Nam", session.Save().Player.Name)
	}
}
