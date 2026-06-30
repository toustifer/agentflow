package engine

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// ---------------------------------------------------------------------------
// SQLite helpers — called from Engine when cfg.DBPath is non-empty.
// ---------------------------------------------------------------------------

const schemaSQL = `
CREATE TABLE IF NOT EXISTS namespaces (
	id         TEXT PRIMARY KEY,
	name       TEXT NOT NULL,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	metadata   TEXT NOT NULL DEFAULT '{}'
);

CREATE TABLE IF NOT EXISTS tasks (
	id                  TEXT NOT NULL,
	namespace_id        TEXT NOT NULL,
	title               TEXT NOT NULL,
	description         TEXT NOT NULL DEFAULT '',
	state               TEXT NOT NULL DEFAULT 'assigned',
	assigned_worker     TEXT NOT NULL DEFAULT '',
	acceptance_criteria TEXT NOT NULL DEFAULT '[]',
	output_files        TEXT NOT NULL DEFAULT '[]',
	worker_agent_id     TEXT NOT NULL DEFAULT '',
	review_cycle        INTEGER NOT NULL DEFAULT 0,
	created_at          TEXT NOT NULL,
	updated_at          TEXT NOT NULL,
	metadata            TEXT NOT NULL DEFAULT '{}',
	PRIMARY KEY (namespace_id, id),
	FOREIGN KEY (namespace_id) REFERENCES namespaces(id)
);

CREATE TABLE IF NOT EXISTS events (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	namespace_id TEXT NOT NULL,
	task_id     TEXT NOT NULL,
	transition  TEXT NOT NULL,
	from_state  TEXT NOT NULL,
	to_state    TEXT NOT NULL,
	timestamp   TEXT NOT NULL,
	actor       TEXT NOT NULL DEFAULT '',
	reason      TEXT NOT NULL DEFAULT '',
	metadata    TEXT NOT NULL DEFAULT '{}'
);`

// openSQLite opens (or creates) a SQLite database at dbPath and ensures the
// schema tables exist.  The caller must call db.Close() when finished.
func openSQLite(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(schemaSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}
	return db, nil
}

// ---------------------------------------------------------------------------
// Namespace persistence
// ---------------------------------------------------------------------------

func insertNamespace(db *sql.DB, ns *Namespace) error {
	meta := mustMarshalJSON(ns.Metadata)
	_, err := db.Exec(
		`INSERT INTO namespaces (id, name, created_at, updated_at, metadata) VALUES (?, ?, ?, ?, ?)`,
		ns.ID, ns.Name, ns.CreatedAt.Format(time.RFC3339Nano), ns.UpdatedAt.Format(time.RFC3339Nano), meta,
	)
	return err
}

func loadNamespaces(db *sql.DB) (map[string]*Namespace, error) {
	rows, err := db.Query(`SELECT id, name, created_at, updated_at, metadata FROM namespaces`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]*Namespace)
	for rows.Next() {
		var (
			id, name, createdAtStr, updatedAtStr, metaStr string
		)
		if err := rows.Scan(&id, &name, &createdAtStr, &updatedAtStr, &metaStr); err != nil {
			return nil, err
		}
		createdAt, _ := time.Parse(time.RFC3339Nano, createdAtStr)
		updatedAt, _ := time.Parse(time.RFC3339Nano, updatedAtStr)
		ns := &Namespace{
			ID:        id,
			Name:      name,
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
			Metadata:  mustUnmarshalStringMap(metaStr),
		}
		out[id] = ns
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Task persistence
// ---------------------------------------------------------------------------

func insertTask(db *sql.DB, task *Task) error {
	ac := mustMarshalJSON(task.AcceptanceCriteria)
	of := mustMarshalJSON(task.OutputFiles)
	meta := mustMarshalJSON(task.Metadata)
	_, err := db.Exec(
		`INSERT INTO tasks (id, namespace_id, title, description, state, assigned_worker,
		 acceptance_criteria, output_files, worker_agent_id, review_cycle,
		 created_at, updated_at, metadata) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		task.ID, task.NamespaceID, task.Title, task.Description, string(task.State),
		task.AssignedWorker, ac, of, task.WorkerAgentID, task.ReviewCycle,
		task.CreatedAt.Format(time.RFC3339Nano), task.UpdatedAt.Format(time.RFC3339Nano), meta,
	)
	return err
}

func updateTask(db *sql.DB, task *Task) error {
	ac := mustMarshalJSON(task.AcceptanceCriteria)
	of := mustMarshalJSON(task.OutputFiles)
	meta := mustMarshalJSON(task.Metadata)
	_, err := db.Exec(
		`UPDATE tasks SET title=?, description=?, state=?, assigned_worker=?,
		 acceptance_criteria=?, output_files=?, worker_agent_id=?, review_cycle=?,
		 updated_at=?, metadata=? WHERE namespace_id=? AND id=?`,
		task.Title, task.Description, string(task.State),
		task.AssignedWorker, ac, of, task.WorkerAgentID, task.ReviewCycle,
		task.UpdatedAt.Format(time.RFC3339Nano), meta,
		task.NamespaceID, task.ID,
	)
	return err
}

func loadTasks(db *sql.DB) (map[string]map[string]*Task, error) {
	rows, err := db.Query(`SELECT id, namespace_id, title, description, state, assigned_worker,
		acceptance_criteria, output_files, worker_agent_id, review_cycle,
		created_at, updated_at, metadata FROM tasks`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]map[string]*Task)
	for rows.Next() {
		var (
			id, nsID, title, desc, stateStr, worker string
			acStr, ofStr, waID, createdAtStr, updatedAtStr, metaStr string
			reviewCycle int
		)
		if err := rows.Scan(&id, &nsID, &title, &desc, &stateStr, &worker,
			&acStr, &ofStr, &waID, &reviewCycle,
			&createdAtStr, &updatedAtStr, &metaStr); err != nil {
			return nil, err
		}
		createdAt, _ := time.Parse(time.RFC3339Nano, createdAtStr)
		updatedAt, _ := time.Parse(time.RFC3339Nano, updatedAtStr)
		task := &Task{
			ID:                id,
			NamespaceID:       nsID,
			Title:             title,
			Description:       desc,
			State:             TaskState(stateStr),
			AssignedWorker:    worker,
			AcceptanceCriteria: mustUnmarshalStringSlice(acStr),
			OutputFiles:       mustUnmarshalStringSlice(ofStr),
			WorkerAgentID:     waID,
			ReviewCycle:       reviewCycle,
			CreatedAt:         createdAt,
			UpdatedAt:         updatedAt,
			Metadata:          mustUnmarshalStringMap(metaStr),
		}
		if out[nsID] == nil {
			out[nsID] = make(map[string]*Task)
		}
		out[nsID][id] = task
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Event persistence
// ---------------------------------------------------------------------------

func insertEvent(db *sql.DB, nsID, taskID string, ev Event) error {
	meta := mustMarshalJSON(ev.Metadata)
	_, err := db.Exec(
		`INSERT INTO events (namespace_id, task_id, transition, from_state, to_state, timestamp, actor, reason, metadata) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		nsID, taskID, ev.Transition, string(ev.FromState), string(ev.ToState),
		ev.Timestamp.Format(time.RFC3339Nano), ev.Actor, ev.Reason, meta,
	)
	return err
}

func loadHistory(db *sql.DB) (map[string]map[string][]Event, error) {
	rows, err := db.Query(`SELECT namespace_id, task_id, transition, from_state, to_state, timestamp, actor, reason, metadata FROM events ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]map[string][]Event)
	for rows.Next() {
		var (
			nsID, taskID, trans, fromStr, toStr, tsStr, actor, reason, metaStr string
		)
		if err := rows.Scan(&nsID, &taskID, &trans, &fromStr, &toStr, &tsStr, &actor, &reason, &metaStr); err != nil {
			return nil, err
		}
		ts, _ := time.Parse(time.RFC3339Nano, tsStr)
		ev := Event{
			TaskID:     taskID,
			Transition: trans,
			FromState:  TaskState(fromStr),
			ToState:    TaskState(toStr),
			Timestamp:  ts,
			Actor:      actor,
			Reason:     reason,
			Metadata:   mustUnmarshalStringMap(metaStr),
		}
		if out[nsID] == nil {
			out[nsID] = make(map[string][]Event)
		}
		out[nsID][taskID] = append(out[nsID][taskID], ev)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// JSON helpers
// ---------------------------------------------------------------------------

func mustMarshalJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func mustUnmarshalStringMap(s string) map[string]string {
	if s == "" || s == "null" {
		return nil
	}
	var m map[string]string
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return nil
	}
	return m
}

func mustUnmarshalStringSlice(s string) []string {
	if s == "" || s == "null" {
		return nil
	}
	var sl []string
	if err := json.Unmarshal([]byte(s), &sl); err != nil {
		return nil
	}
	return sl
}
