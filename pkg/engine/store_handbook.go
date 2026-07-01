package engine

import (
	"database/sql"
	"encoding/json"
	"time"
)

// ---------------------------------------------------------------------------
// WorkerHandbook persistence
// ---------------------------------------------------------------------------

func upsertHandbook(db *sql.DB, hb *WorkerHandbook) error {
	tech := mustMarshalJSON(hb.TechStack)
	know := mustMarshalJSON(hb.Knowledge)
	pit := mustMarshalJSON(hb.Pitfalls)
	_, err := db.Exec(
		`INSERT INTO worker_handbooks (namespace_id, worker_id, scope, tech_stack, knowledge, pitfalls, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(namespace_id, worker_id) DO UPDATE SET
		 scope=excluded.scope, tech_stack=excluded.tech_stack,
		 knowledge=excluded.knowledge, pitfalls=excluded.pitfalls,
		 updated_at=excluded.updated_at`,
		hb.NamespaceID, hb.WorkerID, hb.Scope, tech, know, pit,
		hb.CreatedAt.Format(time.RFC3339Nano), hb.UpdatedAt.Format(time.RFC3339Nano),
	)
	return err
}

func loadHandbooks(db *sql.DB) (map[string]map[string]*WorkerHandbook, error) {
	rows, err := db.Query(`SELECT namespace_id, worker_id, scope, tech_stack, knowledge, pitfalls, created_at, updated_at FROM worker_handbooks`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]map[string]*WorkerHandbook)
	for rows.Next() {
		var nsID, wid, scope, techStr, knowStr, pitStr, createdAtStr, updatedAtStr string
		if err := rows.Scan(&nsID, &wid, &scope, &techStr, &knowStr, &pitStr, &createdAtStr, &updatedAtStr); err != nil {
			return nil, err
		}
		createdAt, _ := time.Parse(time.RFC3339Nano, createdAtStr)
		updatedAt, _ := time.Parse(time.RFC3339Nano, updatedAtStr)
		hb := &WorkerHandbook{
			WorkerID:    wid,
			NamespaceID: nsID,
			Scope:       scope,
			TechStack:   mustUnmarshalStringSlice(techStr),
			Knowledge:   mustUnmarshalKnowledge(knowStr),
			Pitfalls:    mustUnmarshalPitfalls(pitStr),
			CreatedAt:   createdAt,
			UpdatedAt:   updatedAt,
		}
		if out[nsID] == nil {
			out[nsID] = make(map[string]*WorkerHandbook)
		}
		out[nsID][wid] = hb
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// WorkerDiary persistence
// ---------------------------------------------------------------------------

func upsertWorkerDiary(db *sql.DB, d *WorkerDiary) error {
	tags := mustMarshalJSON(d.Tags)
	_, err := db.Exec(
		`INSERT INTO worker_diaries (namespace_id, worker_id, date, task_id, content, tags, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(namespace_id, worker_id, date) DO UPDATE SET
		 task_id=excluded.task_id, content=excluded.content, tags=excluded.tags`,
		d.NamespaceID, d.WorkerID, d.Date, d.TaskID, d.Content, tags,
		d.CreatedAt.Format(time.RFC3339Nano),
	)
	return err
}

func loadWorkerDiaries(db *sql.DB) (map[string]map[string]*WorkerDiary, error) {
	rows, err := db.Query(`SELECT namespace_id, worker_id, date, task_id, content, tags, created_at FROM worker_diaries`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]map[string]*WorkerDiary)
	for rows.Next() {
		var nsID, wid, date, taskID, content, tagsStr, createdAtStr string
		if err := rows.Scan(&nsID, &wid, &date, &taskID, &content, &tagsStr, &createdAtStr); err != nil {
			return nil, err
		}
		createdAt, _ := time.Parse(time.RFC3339Nano, createdAtStr)
		d := &WorkerDiary{
			WorkerID:    wid,
			NamespaceID: nsID,
			Date:        date,
			TaskID:      taskID,
			Content:     content,
			Tags:        mustUnmarshalStringSlice(tagsStr),
			CreatedAt:   createdAt,
		}
		if out[nsID] == nil {
			out[nsID] = make(map[string]*WorkerDiary)
		}
		out[nsID][diaryKey(wid, date)] = d
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// LeaderDiary persistence
// ---------------------------------------------------------------------------

func upsertLeaderDiary(db *sql.DB, ld *LeaderDiary) error {
	entries := mustMarshalJSON(ld.Entries)
	_, err := db.Exec(
		`INSERT INTO leader_diaries (namespace_id, date, entries, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(namespace_id, date) DO UPDATE SET
		 entries=excluded.entries, updated_at=excluded.updated_at`,
		ld.NamespaceID, ld.Date, entries,
		ld.CreatedAt.Format(time.RFC3339Nano), ld.UpdatedAt.Format(time.RFC3339Nano),
	)
	return err
}

func loadLeaderDiaries(db *sql.DB) (map[string]map[string]*LeaderDiary, error) {
	rows, err := db.Query(`SELECT namespace_id, date, entries, created_at, updated_at FROM leader_diaries`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]map[string]*LeaderDiary)
	for rows.Next() {
		var nsID, date, entriesStr, createdAtStr, updatedAtStr string
		if err := rows.Scan(&nsID, &date, &entriesStr, &createdAtStr, &updatedAtStr); err != nil {
			return nil, err
		}
		createdAt, _ := time.Parse(time.RFC3339Nano, createdAtStr)
		updatedAt, _ := time.Parse(time.RFC3339Nano, updatedAtStr)
		var entries []DiaryEntry
		if err := json.Unmarshal([]byte(entriesStr), &entries); err != nil {
			entries = nil
		}
		ld := &LeaderDiary{
			NamespaceID: nsID,
			Date:        date,
			Entries:     entries,
			CreatedAt:   createdAt,
			UpdatedAt:   updatedAt,
		}
		if out[nsID] == nil {
			out[nsID] = make(map[string]*LeaderDiary)
		}
		out[nsID][date] = ld
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Unmarshal helpers for complex types
// ---------------------------------------------------------------------------

func mustUnmarshalKnowledge(s string) []KnowledgeItem {
	if s == "" || s == "null" {
		return nil
	}
	var items []KnowledgeItem
	if err := json.Unmarshal([]byte(s), &items); err != nil {
		return nil
	}
	return items
}

func mustUnmarshalPitfalls(s string) []PitfallItem {
	if s == "" || s == "null" {
		return nil
	}
	var items []PitfallItem
	if err := json.Unmarshal([]byte(s), &items); err != nil {
		return nil
	}
	return items
}
