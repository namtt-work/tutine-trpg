package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	_ "modernc.org/sqlite"
)

type SQLiteStore struct {
	db *sql.DB
}

func NewSQLiteStore(ctx context.Context, path string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	store := &SQLiteStore{db: db}
	if err := store.initialize(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *SQLiteStore) initialize(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS memories (
			id TEXT PRIMARY KEY,
			save_id TEXT NOT NULL,
			campaign_id TEXT NOT NULL,
			turn INTEGER NOT NULL,
			type TEXT NOT NULL,
			scope TEXT NOT NULL,
			importance INTEGER NOT NULL,
			text TEXT NOT NULL,
			summary TEXT NOT NULL,
			entities_json TEXT NOT NULL,
			tags_json TEXT NOT NULL,
			facts_json TEXT NOT NULL,
			location_id TEXT NOT NULL,
			quest_id TEXT NOT NULL,
			npc_id TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS memories_save_id_idx ON memories(save_id);
		CREATE VIRTUAL TABLE IF NOT EXISTS memory_fts USING fts5(
			id UNINDEXED,
			text,
			summary
		);
	`)
	return err
}

func (s *SQLiteStore) Add(ctx context.Context, memory Memory) error {
	entities, err := json.Marshal(memory.Entities)
	if err != nil {
		return fmt.Errorf("marshal memory entities: %w", err)
	}
	tags, err := json.Marshal(memory.Tags)
	if err != nil {
		return fmt.Errorf("marshal memory tags: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO memories (
			id, save_id, campaign_id, turn, type, scope, importance, text, summary,
			entities_json, tags_json, facts_json, location_id, quest_id, npc_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		memory.ID, memory.SaveID, memory.CampaignID, memory.Turn, memory.Type,
		memory.Scope, memory.Importance, memory.Text, memory.Summary, string(entities),
		string(tags), memory.FactsJSON, memory.LocationID, memory.QuestID, memory.NPCID)
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx,
		"INSERT INTO memory_fts (id, text, summary) VALUES (?, ?, ?)",
		memory.ID, memory.Text, memory.Summary)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLiteStore) Search(ctx context.Context, query Query) ([]Hit, error) {
	statement, args := searchStatement(query)
	rows, err := s.db.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hits []Hit
	for rows.Next() {
		memory, err := scanMemory(rows)
		if err != nil {
			return nil, err
		}
		entityMatches := overlap(query.Entities, memory.Entities)
		tagMatches := overlap(query.Tags, memory.Tags)
		hits = append(hits, Hit{
			Memory: memory,
			Score:  float64(entityMatches*100+tagMatches*50+memory.Importance*5) + float64(memory.Turn)*0.01,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Score == hits[j].Score {
			return hits[i].Memory.ID < hits[j].Memory.ID
		}
		return hits[i].Score > hits[j].Score
	})
	if query.MaxResults > 0 && len(hits) > query.MaxResults {
		hits = hits[:query.MaxResults]
	}
	return hits, nil
}

func searchStatement(query Query) (string, []any) {
	where := []string{"m.save_id = ?"}
	args := []any{query.SaveID}

	for _, filter := range []struct {
		column string
		values []string
	}{
		{"m.type", query.Types},
		{"m.location_id", query.Locations},
		{"m.quest_id", query.QuestIDs},
	} {
		if len(filter.values) == 0 {
			continue
		}
		placeholders := strings.TrimRight(strings.Repeat("?,", len(filter.values)), ",")
		where = append(where, filter.column+" IN ("+placeholders+")")
		for _, value := range filter.values {
			args = append(args, value)
		}
	}

	from := "memories m"
	if len(query.Keywords) > 0 {
		from += " JOIN memory_fts ON memory_fts.id = m.id"
		where = append(where, "memory_fts MATCH ?")
		args = append(args, ftsQuery(query.Keywords))
	}

	return `SELECT m.id, m.save_id, m.campaign_id, m.turn, m.type, m.scope,
		m.importance, m.text, m.summary, m.entities_json, m.tags_json, m.facts_json,
		m.location_id, m.quest_id, m.npc_id FROM ` + from + " WHERE " + strings.Join(where, " AND "), args
}

func scanMemory(scanner interface{ Scan(...any) error }) (Memory, error) {
	var memory Memory
	var entitiesJSON, tagsJSON string
	err := scanner.Scan(
		&memory.ID, &memory.SaveID, &memory.CampaignID, &memory.Turn, &memory.Type,
		&memory.Scope, &memory.Importance, &memory.Text, &memory.Summary, &entitiesJSON,
		&tagsJSON, &memory.FactsJSON, &memory.LocationID, &memory.QuestID, &memory.NPCID,
	)
	if err != nil {
		return Memory{}, err
	}
	if err := json.Unmarshal([]byte(entitiesJSON), &memory.Entities); err != nil {
		return Memory{}, fmt.Errorf("unmarshal memory entities: %w", err)
	}
	if err := json.Unmarshal([]byte(tagsJSON), &memory.Tags); err != nil {
		return Memory{}, fmt.Errorf("unmarshal memory tags: %w", err)
	}
	return memory, nil
}

func ftsQuery(keywords []string) string {
	phrases := make([]string, 0, len(keywords))
	for _, keyword := range keywords {
		phrases = append(phrases, `"`+strings.ReplaceAll(keyword, `"`, `""`)+`"`)
	}
	return strings.Join(phrases, " AND ")
}

func overlap(queryValues, memoryValues []string) int {
	memorySet := make(map[string]struct{}, len(memoryValues))
	for _, value := range memoryValues {
		memorySet[value] = struct{}{}
	}
	seen := make(map[string]struct{}, len(queryValues))
	matches := 0
	for _, value := range queryValues {
		if _, duplicate := seen[value]; duplicate {
			continue
		}
		seen[value] = struct{}{}
		if _, found := memorySet[value]; found {
			matches++
		}
	}
	return matches
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}
