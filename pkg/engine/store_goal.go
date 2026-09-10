package engine

import (
	"database/sql"
	"time"
)

func insertGoal(db *sql.DB, g *Goal) error {
	tags := mustMarshalJSON(g.Tags)
	meta := mustMarshalJSON(g.Metadata)
	_, err := db.Exec(
		`INSERT INTO goals (id, namespace_id, title, description, status, priority, tags, context, dag_id, metadata, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		g.ID, g.NamespaceID, g.Title, g.Description, string(g.Status), g.Priority, tags, g.Context, g.DAGID, meta,
		g.CreatedAt.Format(time.RFC3339Nano), g.UpdatedAt.Format(time.RFC3339Nano),
	)
	return err
}

func updateGoalRecord(db *sql.DB, g *Goal) error {
	tags := mustMarshalJSON(g.Tags)
	meta := mustMarshalJSON(g.Metadata)
	_, err := db.Exec(
		`UPDATE goals SET title=?, description=?, status=?, priority=?, tags=?, context=?, dag_id=?, metadata=?, updated_at=? WHERE namespace_id=? AND id=?`,
		g.Title, g.Description, string(g.Status), g.Priority, tags, g.Context, g.DAGID, meta,
		g.UpdatedAt.Format(time.RFC3339Nano), g.NamespaceID, g.ID,
	)
	return err
}

func loadGoals(db *sql.DB) (map[string]map[string]*Goal, error) {
	rows, err := db.Query(`SELECT id, namespace_id, title, description, status, priority, tags, context, dag_id, metadata, created_at, updated_at FROM goals`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]map[string]*Goal)
	for rows.Next() {
		var (
			id, nsID, title, desc, statusStr, context, dagID, metaStr, tagsStr, createdAtStr, updatedAtStr string
			priority                                                                                        int
		)
		if err := rows.Scan(&id, &nsID, &title, &desc, &statusStr, &priority, &tagsStr, &context, &dagID, &metaStr, &createdAtStr, &updatedAtStr); err != nil {
			return nil, err
		}
		createdAt, _ := time.Parse(time.RFC3339Nano, createdAtStr)
		updatedAt, _ := time.Parse(time.RFC3339Nano, updatedAtStr)
		goal := &Goal{
			ID:          id,
			NamespaceID: nsID,
			Title:       title,
			Description: desc,
			Status:      GoalStatus(statusStr),
			Priority:    priority,
			Tags:        mustUnmarshalStringSlice(tagsStr),
			Context:     context,
			DAGID:       dagID,
			Metadata:    mustUnmarshalStringMap(metaStr),
			CreatedAt:   createdAt,
			UpdatedAt:   updatedAt,
		}
		if out[nsID] == nil {
			out[nsID] = make(map[string]*Goal)
		}
		out[nsID][id] = goal
	}
	return out, rows.Err()
}
