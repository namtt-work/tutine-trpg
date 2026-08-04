package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

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

func (fs *FileStore) AppendEvent(ctx context.Context, saveID string, event Event) error {
	if err := validateSaveID(saveID); err != nil {
		return err
	}
	dir := fs.saveDir(saveID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create save directory: %w", err)
	}
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	f, err := os.OpenFile(filepath.Join(dir, "events.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open event log: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write event: %w", err)
	}
	return f.Sync()
}

type fileLock struct {
	path string
}

func (l *fileLock) Release() error {
	return os.Remove(l.path)
}

// AcquireLock ensures the save directory exists (a brand new save has none
// yet, since game.NewStarterSave performs no I/O) before creating .lock with
// O_EXCL, so a second process racing on the same save fails immediately
// instead of silently overwriting state.
func (fs *FileStore) AcquireLock(ctx context.Context, saveID string) (Lock, error) {
	if err := validateSaveID(saveID); err != nil {
		return nil, err
	}
	dir := fs.saveDir(saveID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create save directory: %w", err)
	}
	lockPath := filepath.Join(dir, ".lock")
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return nil, fmt.Errorf("save %q is already open in another process (remove %s if that process is no longer running)", saveID, lockPath)
		}
		return nil, fmt.Errorf("acquire lock for save %q: %w", saveID, err)
	}
	_, writeErr := fmt.Fprintf(f, "%d", os.Getpid())
	closeErr := f.Close()
	if writeErr != nil || closeErr != nil {
		os.Remove(lockPath)
		if writeErr != nil {
			return nil, fmt.Errorf("write lock pid: %w", writeErr)
		}
		return nil, fmt.Errorf("close lock file: %w", closeErr)
	}
	return &fileLock{path: lockPath}, nil
}

// ListSaves reuses LoadSnapshot's SaveID cross-check while scanning: an
// entry whose state.json embeds a different SaveID than its directory name
// is silently skipped rather than returned, so a corrupted or malformed
// save never becomes an auto-resume candidate.
func (fs *FileStore) ListSaves(ctx context.Context, campaignID string) ([]SaveSummary, error) {
	savesRoot := filepath.Join(fs.dataDir, "saves")
	entries, err := os.ReadDir(savesRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read saves directory: %w", err)
	}

	var summaries []SaveSummary
	for _, entry := range entries {
		if !entry.IsDir() || validateSaveID(entry.Name()) != nil {
			continue
		}
		statePath := filepath.Join(savesRoot, entry.Name(), "state.json")
		info, err := os.Stat(statePath)
		if err != nil {
			continue
		}
		save, err := fs.LoadSnapshot(ctx, entry.Name())
		if err != nil {
			continue
		}
		if save.CampaignID != campaignID {
			continue
		}
		summaries = append(summaries, SaveSummary{
			SaveID:       save.SaveID,
			CampaignID:   save.CampaignID,
			PlayerName:   save.Player.Name,
			CurrentTurn:  save.CurrentTurn,
			CurrentScene: save.CurrentScene,
			UpdatedAt:    info.ModTime(),
		})
	}

	sort.Slice(summaries, func(i, j int) bool {
		if !summaries[i].UpdatedAt.Equal(summaries[j].UpdatedAt) {
			return summaries[i].UpdatedAt.After(summaries[j].UpdatedAt)
		}
		return summaries[i].SaveID > summaries[j].SaveID
	})
	return summaries, nil
}
