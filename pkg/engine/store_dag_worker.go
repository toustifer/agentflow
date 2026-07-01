package engine

import (
	"database/sql"
	"time"
)

// ---------------------------------------------------------------------------
// DAG persistence
// ---------------------------------------------------------------------------

func insertDAG(db *sql.DB, dag *DAG) error {
	_, err := db.Exec(
		`INSERT INTO dags (id, namespace_id, title, branch, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		dag.ID, dag.NamespaceID, dag.Title, dag.Branch, string(dag.Status),
		dag.CreatedAt.Format(time.RFC3339Nano), dag.UpdatedAt.Format(time.RFC3339Nano),
	)
	return err
}

func updateDAG(db *sql.DB, dag *DAG) error {
	_, err := db.Exec(
		`UPDATE dags SET title=?, branch=?, status=?, updated_at=? WHERE namespace_id=? AND id=?`,
		dag.Title, dag.Branch, string(dag.Status),
		dag.UpdatedAt.Format(time.RFC3339Nano),
		dag.NamespaceID, dag.ID,
	)
	return err
}

func loadDAGs(db *sql.DB) (map[string]map[string]*DAG, error) {
	rows, err := db.Query(`SELECT id, namespace_id, title, branch, status, created_at, updated_at FROM dags`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]map[string]*DAG)
	for rows.Next() {
		var (
			id, nsID, title, branch, statusStr, createdAtStr, updatedAtStr string
		)
		if err := rows.Scan(&id, &nsID, &title, &branch, &statusStr, &createdAtStr, &updatedAtStr); err != nil {
			return nil, err
		}
		createdAt, _ := time.Parse(time.RFC3339Nano, createdAtStr)
		updatedAt, _ := time.Parse(time.RFC3339Nano, updatedAtStr)
		dag := &DAG{
			ID:          id,
			NamespaceID: nsID,
			Title:       title,
			Branch:      branch,
			Status:      DAGStatus(statusStr),
			CreatedAt:   createdAt,
			UpdatedAt:   updatedAt,
		}
		if out[nsID] == nil {
			out[nsID] = make(map[string]*DAG)
		}
		out[nsID][id] = dag
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Worker persistence
// ---------------------------------------------------------------------------

func insertWorker(db *sql.DB, w *Worker) error {
	skills := mustMarshalJSON(w.Skills)
	meta := mustMarshalJSON(w.Metadata)
	_, err := db.Exec(
		`INSERT INTO workers (id, namespace_id, name, scope, skills, prompt_template, metadata, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		w.ID, w.NamespaceID, w.Name, w.Scope, skills, w.PromptTemplate, meta,
		w.CreatedAt.Format(time.RFC3339Nano), w.UpdatedAt.Format(time.RFC3339Nano),
	)
	return err
}

func updateWorker(db *sql.DB, w *Worker) error {
	skills := mustMarshalJSON(w.Skills)
	meta := mustMarshalJSON(w.Metadata)
	_, err := db.Exec(
		`UPDATE workers SET name=?, scope=?, skills=?, prompt_template=?, metadata=?, updated_at=? WHERE namespace_id=? AND id=?`,
		w.Name, w.Scope, skills, w.PromptTemplate, meta,
		w.UpdatedAt.Format(time.RFC3339Nano),
		w.NamespaceID, w.ID,
	)
	return err
}

func loadWorkers(db *sql.DB) (map[string]map[string]*Worker, error) {
	rows, err := db.Query(`SELECT id, namespace_id, name, scope, skills, prompt_template, metadata, created_at, updated_at FROM workers`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]map[string]*Worker)
	for rows.Next() {
		var (
			id, nsID, name, scope, skillsStr, promptTpl, metaStr, createdAtStr, updatedAtStr string
		)
		if err := rows.Scan(&id, &nsID, &name, &scope, &skillsStr, &promptTpl, &metaStr, &createdAtStr, &updatedAtStr); err != nil {
			return nil, err
		}
		createdAt, _ := time.Parse(time.RFC3339Nano, createdAtStr)
		updatedAt, _ := time.Parse(time.RFC3339Nano, updatedAtStr)
		w := &Worker{
			ID:          id,
			NamespaceID: nsID,
			Name:        name,
			Scope:       scope,
			Skills:         mustUnmarshalStringSlice(skillsStr),
			PromptTemplate: promptTpl,
			Metadata:       mustUnmarshalStringMap(metaStr),
			CreatedAt:   createdAt,
			UpdatedAt:   updatedAt,
		}
		if out[nsID] == nil {
			out[nsID] = make(map[string]*Worker)
		}
		out[nsID][id] = w
	}
	return out, rows.Err()
}
