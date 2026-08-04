package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/namtt/tutine-trpg/internal/game"
)

// FileStore is the filesystem Store implementation: one directory per save
// under <dataDir>/saves/<save_id>/.
type FileStore struct {
	dataDir string
}

func NewFileStore(dataDir string) *FileStore {
	return &FileStore{dataDir: dataDir}
}

func (fs *FileStore) saveDir(saveID string) string {
	return filepath.Join(fs.dataDir, "saves", saveID)
}

// validateSaveID rejects anything that isn't a single path segment. This
// closes the traversal case (../.., absolute paths) for every FileStore
// method that turns a caller-supplied ID into a path.
func validateSaveID(id string) error {
	if id == "" || id != filepath.Base(id) || id == "." || id == ".." {
		return fmt.Errorf("invalid save id %q", id)
	}
	return nil
}

func (fs *FileStore) SaveSnapshot(ctx context.Context, save game.SaveGame) error {
	if err := validateSaveID(save.SaveID); err != nil {
		return err
	}
	dir := fs.saveDir(save.SaveID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create save directory: %w", err)
	}
	data, err := json.MarshalIndent(save, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal save snapshot: %w", err)
	}

	tmp, err := os.CreateTemp(dir, "state-*.json.tmp")
	if err != nil {
		return fmt.Errorf("create temp snapshot file: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write temp snapshot file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("sync temp snapshot file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close temp snapshot file: %w", err)
	}
	if err := os.Rename(tmpPath, filepath.Join(dir, "state.json")); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename snapshot into place: %w", err)
	}
	return nil
}

// LoadSnapshot cross-checks the deserialized SaveGame.SaveID against the
// requested ID: a directory name is trusted by construction (only
// AcquireLock/SaveSnapshot create one, from an already-validated ID), but
// state.json is a plain file a player could hand-edit or a bug could write
// incorrectly, and callers derive other paths (game.db) from this same ID.
func (fs *FileStore) LoadSnapshot(ctx context.Context, saveID string) (game.SaveGame, error) {
	if err := validateSaveID(saveID); err != nil {
		return game.SaveGame{}, err
	}
	path := filepath.Join(fs.saveDir(saveID), "state.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return game.SaveGame{}, fmt.Errorf("save %q not found", saveID)
		}
		return game.SaveGame{}, fmt.Errorf("read snapshot for save %q: %w", saveID, err)
	}
	var save game.SaveGame
	if err := json.Unmarshal(data, &save); err != nil {
		return game.SaveGame{}, fmt.Errorf("decode snapshot for save %q: %w", saveID, err)
	}
	if save.SaveID != saveID {
		return game.SaveGame{}, fmt.Errorf("save %q is corrupted: embedded save id %q does not match its directory", saveID, save.SaveID)
	}
	return save, nil
}
