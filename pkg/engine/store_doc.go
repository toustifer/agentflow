package engine

import (
	"database/sql"
	"time"
)

// ---------------------------------------------------------------------------
// Project Doc persistence
// ---------------------------------------------------------------------------

func insertProjectDoc(db *sql.DB, doc *ProjectDoc) error {
	tags := mustMarshalJSON(doc.Tags)
	result, err := db.Exec(
		`INSERT INTO project_docs (namespace_id, section, path, title, content, tags, version, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		doc.NamespaceID, doc.Section, doc.Path, doc.Title, doc.Content, tags, doc.Version,
		doc.CreatedAt.Format(time.RFC3339Nano), doc.UpdatedAt.Format(time.RFC3339Nano),
	)
	if err != nil {
		return err
	}
	id, _ := result.LastInsertId()
	doc.ID = id
	return nil
}

func updateProjectDoc(db *sql.DB, doc *ProjectDoc) error {
	tags := mustMarshalJSON(doc.Tags)
	_, err := db.Exec(
		`UPDATE project_docs SET section=?, path=?, title=?, content=?, tags=?, version=?, updated_at=? WHERE id=? AND namespace_id=?`,
		doc.Section, doc.Path, doc.Title, doc.Content, tags, doc.Version, doc.UpdatedAt.Format(time.RFC3339Nano), doc.ID, doc.NamespaceID,
	)
	return err
}

func deleteProjectDoc(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM project_docs WHERE id=?`, id)
	return err
}

func loadProjectDocs(db *sql.DB) (map[string][]ProjectDoc, error) {
	rows, err := db.Query(`SELECT id, namespace_id, section, path, title, content, tags, version, created_at, updated_at FROM project_docs`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string][]ProjectDoc)
	for rows.Next() {
		var (
			id int64
			nsID, section, path, title, content, tagsStr, createdAtStr, updatedAtStr string
			version int
		)
		if err := rows.Scan(&id, &nsID, &section, &path, &title, &content, &tagsStr, &version, &createdAtStr, &updatedAtStr); err != nil {
			return nil, err
		}
		createdAt, _ := time.Parse(time.RFC3339Nano, createdAtStr)
		updatedAt, _ := time.Parse(time.RFC3339Nano, updatedAtStr)
		doc := ProjectDoc{
			ID:          id,
			NamespaceID: nsID,
			Section:     section,
			Path:        path,
			Title:       title,
			Content:     content,
			Tags:        mustUnmarshalStringSlice(tagsStr),
			Version:     version,
			CreatedAt:   createdAt,
			UpdatedAt:   updatedAt,
		}
		out[nsID] = append(out[nsID], doc)
	}
	return out, rows.Err()
}
