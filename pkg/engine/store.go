package engine

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
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
	dag_id              TEXT NOT NULL DEFAULT '',
	depends_on          TEXT NOT NULL DEFAULT '[]',
	tags                TEXT NOT NULL DEFAULT '[]',
	priority            INTEGER NOT NULL DEFAULT 0,
	estimated_hours     REAL NOT NULL DEFAULT 0,
	actual_hours        REAL NOT NULL DEFAULT 0,
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
);

CREATE TABLE IF NOT EXISTS dags (
	id                    TEXT NOT NULL,
	namespace_id          TEXT NOT NULL,
	title                 TEXT NOT NULL,
	branch                TEXT NOT NULL DEFAULT '',
	execution_branch      TEXT NOT NULL DEFAULT '',
	base_branch           TEXT NOT NULL DEFAULT '',
	metadata              TEXT NOT NULL DEFAULT '{}',
	worktree_path         TEXT NOT NULL DEFAULT '',
	worktree_status       TEXT NOT NULL DEFAULT '',
	head_sha              TEXT NOT NULL DEFAULT '',
	active_task_id        TEXT NOT NULL DEFAULT '',
	lease_holder_task_id  TEXT NOT NULL DEFAULT '',
	lease_holder_worker_id TEXT NOT NULL DEFAULT '',
	lease_holder_agent_id TEXT NOT NULL DEFAULT '',
	lease_acquired_at     TEXT NOT NULL DEFAULT '',
	runtime_updated_at    TEXT NOT NULL DEFAULT '',
	status                TEXT NOT NULL DEFAULT 'planning',
	created_at            TEXT NOT NULL,
	updated_at            TEXT NOT NULL,
	PRIMARY KEY (namespace_id, id),
	FOREIGN KEY (namespace_id) REFERENCES namespaces(id)
);

CREATE TABLE IF NOT EXISTS workers (
	id           TEXT NOT NULL,
	namespace_id TEXT NOT NULL,
	name         TEXT NOT NULL,
	scope        TEXT NOT NULL DEFAULT '',
	skills       TEXT NOT NULL DEFAULT '[]',
	metadata     TEXT NOT NULL DEFAULT '{}',
	created_at   TEXT NOT NULL,
	updated_at   TEXT NOT NULL,
	PRIMARY KEY (namespace_id, id),
	FOREIGN KEY (namespace_id) REFERENCES namespaces(id)
);

CREATE TABLE IF NOT EXISTS worker_handbooks (
	namespace_id TEXT NOT NULL,
	worker_id    TEXT NOT NULL,
	scope        TEXT NOT NULL DEFAULT '',
	tech_stack   TEXT NOT NULL DEFAULT '[]',
	knowledge    TEXT NOT NULL DEFAULT '[]',
	pitfalls     TEXT NOT NULL DEFAULT '[]',
	created_at   TEXT NOT NULL,
	updated_at   TEXT NOT NULL,
	PRIMARY KEY (namespace_id, worker_id),
	FOREIGN KEY (namespace_id) REFERENCES namespaces(id)
);

CREATE TABLE IF NOT EXISTS worker_diaries (
	namespace_id TEXT NOT NULL,
	worker_id    TEXT NOT NULL,
	date         TEXT NOT NULL,
	task_id      TEXT NOT NULL DEFAULT '',
	content      TEXT NOT NULL,
	tags         TEXT NOT NULL DEFAULT '[]',
	created_at   TEXT NOT NULL,
	PRIMARY KEY (namespace_id, worker_id, date),
	FOREIGN KEY (namespace_id) REFERENCES namespaces(id)
);

CREATE TABLE IF NOT EXISTS leader_diaries (
	namespace_id TEXT NOT NULL,
	date         TEXT NOT NULL,
	entries      TEXT NOT NULL DEFAULT '[]',
	created_at   TEXT NOT NULL,
	updated_at   TEXT NOT NULL,
	PRIMARY KEY (namespace_id, date),
	FOREIGN KEY (namespace_id) REFERENCES namespaces(id)
);

CREATE TABLE IF NOT EXISTS project_docs (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	namespace_id TEXT NOT NULL,
	section      TEXT NOT NULL DEFAULT '',
	path         TEXT NOT NULL DEFAULT '',
	title        TEXT NOT NULL DEFAULT '',
	content      TEXT NOT NULL DEFAULT '',
	tags         TEXT NOT NULL DEFAULT '[]',
	version      INTEGER NOT NULL DEFAULT 1,
	created_at   TEXT NOT NULL,
	updated_at   TEXT NOT NULL,
	FOREIGN KEY (namespace_id) REFERENCES namespaces(id)
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

	// Migration: add new columns to existing tasks tables
	if err := migrateTasksTable(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate tasks: %w", err)
	}
	if err := migrateWorkersTable(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate workers: %w", err)
	}
	if err := migrateDAGsTable(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate dags: %w", err)
	}

	return db, nil
}

func migrateTasksTable(db *sql.DB) error {
	cols := []string{
		"dag_id TEXT NOT NULL DEFAULT ''",
		"depends_on TEXT NOT NULL DEFAULT '[]'",
		"tags TEXT NOT NULL DEFAULT '[]'",
		"priority INTEGER NOT NULL DEFAULT 0",
		"estimated_hours REAL NOT NULL DEFAULT 0",
		"actual_hours REAL NOT NULL DEFAULT 0",
	}
	for _, c := range cols {
		// SQLite ALTER TABLE ADD COLUMN is not idempotent; check PRAGMA first.
		colName := strings.SplitN(c, " ", 2)[0]
		var count int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM pragma_table_info('tasks') WHERE name = ?`, colName,
		).Scan(&count); err != nil {
			return fmt.Errorf("check column %s: %w", colName, err)
		}
		if count == 0 {
			if _, err := db.Exec("ALTER TABLE tasks ADD COLUMN " + c); err != nil {
				return fmt.Errorf("add column %s: %w", colName, err)
			}
		}
	}
	return nil
}

func migrateWorkersTable(db *sql.DB) error {
	cols := []string{
		"prompt_template TEXT NOT NULL DEFAULT ''",
		"kind TEXT NOT NULL DEFAULT ''",
		"task_tags TEXT NOT NULL DEFAULT '[]'",
		"required_reads TEXT NOT NULL DEFAULT '[]'",
		"recommended_mcp TEXT NOT NULL DEFAULT '[]'",
		"launch_mode TEXT NOT NULL DEFAULT ''",
		"handoff_targets TEXT NOT NULL DEFAULT '[]'",
		"recovery_policy TEXT NOT NULL DEFAULT '[]'",
		"fallback_mcp TEXT NOT NULL DEFAULT '[]'",
		"stuck_playbook TEXT NOT NULL DEFAULT ''",
		"escalation_mode TEXT NOT NULL DEFAULT ''",
	}
	for _, c := range cols {
		colName := strings.SplitN(c, " ", 2)[0]
		var count int
		qr := "SELECT COUNT(*) FROM pragma_table_info('workers') WHERE name = ?"
		if err := db.QueryRow(qr, colName).Scan(&count); err != nil {
			return fmt.Errorf("check column %s: %w", colName, err)
		}
		if count == 0 {
			if _, err := db.Exec("ALTER TABLE workers ADD COLUMN " + c); err != nil {
				return fmt.Errorf("add column %s: %w", colName, err)
			}
		}
	}
	return nil
}
func migrateDAGsTable(db *sql.DB) error {
	cols := []string{
		"execution_branch TEXT NOT NULL DEFAULT ''",
		"base_branch TEXT NOT NULL DEFAULT ''",
		"metadata TEXT NOT NULL DEFAULT '{}'",
		"worktree_path TEXT NOT NULL DEFAULT ''",
		"worktree_status TEXT NOT NULL DEFAULT ''",
		"head_sha TEXT NOT NULL DEFAULT ''",
		"active_task_id TEXT NOT NULL DEFAULT ''",
		"lease_holder_task_id TEXT NOT NULL DEFAULT ''",
		"lease_holder_worker_id TEXT NOT NULL DEFAULT ''",
		"lease_holder_agent_id TEXT NOT NULL DEFAULT ''",
		"lease_acquired_at TEXT NOT NULL DEFAULT ''",
		"runtime_updated_at TEXT NOT NULL DEFAULT ''",
	}
	for _, c := range cols {
		colName := strings.SplitN(c, " ", 2)[0]
		var count int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM pragma_table_info('dags') WHERE name = ?`, colName,
		).Scan(&count); err != nil {
			return fmt.Errorf("check column %s: %w", colName, err)
		}
		if count == 0 {
			if _, err := db.Exec("ALTER TABLE dags ADD COLUMN " + c); err != nil {
				return fmt.Errorf("add column %s: %w", colName, err)
			}
		}
	}
	if _, err := db.Exec(`UPDATE dags SET execution_branch = branch WHERE execution_branch = '' AND branch != ''`); err != nil {
		return fmt.Errorf("backfill execution_branch: %w", err)
	}
	return nil
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
	deps := mustMarshalJSON(task.DependsOn)
	tags := mustMarshalJSON(task.Tags)
	meta := mustMarshalJSON(task.Metadata)
	_, err := db.Exec(
		`INSERT INTO tasks (id, namespace_id, title, description, state, assigned_worker,
		 acceptance_criteria, output_files, dag_id, depends_on, tags, priority,
		 estimated_hours, actual_hours, worker_agent_id, review_cycle,
		 created_at, updated_at, metadata) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		task.ID, task.NamespaceID, task.Title, task.Description, string(task.State),
		task.AssignedWorker, ac, of, task.DAGID, deps, tags, task.Priority,
		task.EstimatedHours, task.ActualHours, task.WorkerAgentID, task.ReviewCycle,
		task.CreatedAt.Format(time.RFC3339Nano), task.UpdatedAt.Format(time.RFC3339Nano), meta,
	)
	return err
}

func updateTask(db *sql.DB, task *Task) error {
	ac := mustMarshalJSON(task.AcceptanceCriteria)
	of := mustMarshalJSON(task.OutputFiles)
	deps := mustMarshalJSON(task.DependsOn)
	tags := mustMarshalJSON(task.Tags)
	meta := mustMarshalJSON(task.Metadata)
	_, err := db.Exec(
		`UPDATE tasks SET title=?, description=?, state=?, assigned_worker=?,
		 acceptance_criteria=?, output_files=?, dag_id=?, depends_on=?, tags=?,
		 priority=?, estimated_hours=?, actual_hours=?, worker_agent_id=?, review_cycle=?,
		 updated_at=?, metadata=? WHERE namespace_id=? AND id=?`,
		task.Title, task.Description, string(task.State),
		task.AssignedWorker, ac, of, task.DAGID, deps, tags,
		task.Priority, task.EstimatedHours, task.ActualHours,
		task.WorkerAgentID, task.ReviewCycle,
		task.UpdatedAt.Format(time.RFC3339Nano), meta,
		task.NamespaceID, task.ID,
	)
	return err
}

func loadTasks(db *sql.DB) (map[string]map[string]*Task, error) {
	rows, err := db.Query(`SELECT id, namespace_id, title, description, state, assigned_worker,
		acceptance_criteria, output_files, dag_id, depends_on, tags, priority,
		estimated_hours, actual_hours, worker_agent_id, review_cycle,
		created_at, updated_at, metadata FROM tasks`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]map[string]*Task)
	for rows.Next() {
		var (
			id, nsID, title, desc, stateStr, worker string
			acStr, ofStr, dagID, depsStr, tagsStr, waID, createdAtStr, updatedAtStr, metaStr string
			reviewCycle int
			priority int
			estimatedHours, actualHours float64
		)
		if err := rows.Scan(&id, &nsID, &title, &desc, &stateStr, &worker,
			&acStr, &ofStr, &dagID, &depsStr, &tagsStr, &priority,
			&estimatedHours, &actualHours, &waID, &reviewCycle,
			&createdAtStr, &updatedAtStr, &metaStr); err != nil {
			return nil, err
		}
		createdAt, _ := time.Parse(time.RFC3339Nano, createdAtStr)
		updatedAt, _ := time.Parse(time.RFC3339Nano, updatedAtStr)
		task := &Task{
			ID:                 id,
			NamespaceID:        nsID,
			Title:              title,
			Description:        desc,
			State:              TaskState(stateStr),
			AssignedWorker:     worker,
			AcceptanceCriteria: mustUnmarshalStringSlice(acStr),
			OutputFiles:        mustUnmarshalStringSlice(ofStr),
			DAGID:              dagID,
			DependsOn:          mustUnmarshalStringSlice(depsStr),
			Tags:               mustUnmarshalStringSlice(tagsStr),
			Priority:           priority,
			EstimatedHours:     estimatedHours,
			ActualHours:        actualHours,
			WorkerAgentID:      waID,
			ReviewCycle:        reviewCycle,
			CreatedAt:          createdAt,
			UpdatedAt:          updatedAt,
			Metadata:           mustUnmarshalStringMap(metaStr),
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

// ---------------------------------------------------------------------------
// Deletion
// ---------------------------------------------------------------------------

func deleteAllForNamespace(db *sql.DB, nsID string) error {
	tables := []string{"leader_diaries", "worker_diaries", "worker_handbooks", "workers", "events", "tasks", "dags"}
	for _, table := range tables {
		if _, err := db.Exec("DELETE FROM "+table+" WHERE namespace_id = ?", nsID); err != nil {
			return fmt.Errorf("delete %s: %w", table, err)
		}
	}
	if _, err := db.Exec("DELETE FROM namespaces WHERE id = ?", nsID); err != nil {
		return fmt.Errorf("delete namespace: %w", err)
	}
	return nil
}
