package main

import (
	"context"
	"testing"

	"github.com/namtt/tutine-trpg/internal/orchestrator"
)

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

func TestBuildOfflineSessionUsesDistinctSaveStorage(t *testing.T) {
	dataDir := t.TempDir()
	first, firstCleanup, err := buildOfflineSession(dataDir, "Nam")
	if err != nil {
		t.Fatalf("build first offline session: %v", err)
	}
	firstResult, err := first.HandleTurn(context.Background(), orchestrator.PlayerInput{Text: "ta quan sat cong mon"})
	firstCleanup()
	if err != nil {
		t.Fatalf("handle first turn: %v", err)
	}
	if len(firstResult.Warnings) != 0 {
		t.Fatalf("first turn warnings = %#v", firstResult.Warnings)
	}

	second, secondCleanup, err := buildOfflineSession(dataDir, "Nam")
	if err != nil {
		t.Fatalf("build second offline session: %v", err)
	}
	defer secondCleanup()
	secondResult, err := second.HandleTurn(context.Background(), orchestrator.PlayerInput{Text: "ta quan sat cong mon"})
	if err != nil {
		t.Fatalf("handle second turn: %v", err)
	}
	if first.Save().SaveID == second.Save().SaveID {
		t.Fatalf("save IDs match: %q", first.Save().SaveID)
	}
	if len(secondResult.Warnings) != 0 {
		t.Fatalf("second turn warnings = %#v", secondResult.Warnings)
	}
}
