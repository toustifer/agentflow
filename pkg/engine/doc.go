package engine

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// ProjectDoc — project-level documentation (architecture, UI/UX, API, decisions)
// ---------------------------------------------------------------------------

type ProjectDoc struct {
	ID          int64     `json:"id"`
	NamespaceID string    `json:"namespace_id"`
	Section     string    `json:"section"`
	Path        string    `json:"path"`
	Title       string    `json:"title"`
	Content     string    `json:"content"`
	Tags        []string  `json:"tags,omitempty"`
	Version     int       `json:"version"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type ProjectDocRef struct {
	ID          int64     `json:"id"`
	NamespaceID string    `json:"namespace_id"`
	Section     string    `json:"section"`
	Path        string    `json:"path"`
	Title       string    `json:"title"`
	Tags        []string  `json:"tags,omitempty"`
	Version     int       `json:"version"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ---------------------------------------------------------------------------
// Engine CRUD — in-memory backed by SQLite
// ---------------------------------------------------------------------------

func (e *Engine) WriteProjectDoc(ctx context.Context, nsID string, doc ProjectDoc) (*ProjectDoc, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, ok := e.namespaces[nsID]; !ok {
		return nil, ErrNamespaceNotFound
	}

	now := time.Now().UTC()
	if doc.ID == 0 {
		doc.NamespaceID = nsID
		doc.Version = 1
		doc.CreatedAt = now
		doc.UpdatedAt = now
		if e.db != nil {
			if err := insertProjectDoc(e.db, &doc); err != nil {
				return nil, fmt.Errorf("insert doc: %w", err)
			}
		}
	} else {
		// Update existing — bump version
		for i := range e.projectDocs[nsID] {
			if e.projectDocs[nsID][i].ID == doc.ID {
				doc.NamespaceID = nsID
				doc.Version = e.projectDocs[nsID][i].Version + 1
				doc.CreatedAt = e.projectDocs[nsID][i].CreatedAt
				doc.UpdatedAt = now
				e.projectDocs[nsID][i] = doc
				if e.db != nil {
					_ = updateProjectDoc(e.db, &doc)
				}
				return &doc, nil
			}
		}
		return nil, fmt.Errorf("doc id %d not found in namespace %s", doc.ID, nsID)
	}

	e.projectDocs[nsID] = append(e.projectDocs[nsID], doc)
	return &doc, nil
}

func (e *Engine) GetProjectDoc(ctx context.Context, nsID string, docID int64) (*ProjectDoc, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	for i := range e.projectDocs[nsID] {
		if e.projectDocs[nsID][i].ID == docID {
			doc := e.projectDocs[nsID][i]
			return &doc, nil
		}
	}
	return nil, fmt.Errorf("doc %d not found", docID)
}

func (e *Engine) ListProjectDocs(ctx context.Context, nsID string) ([]ProjectDoc, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	out := make([]ProjectDoc, len(e.projectDocs[nsID]))
	copy(out, e.projectDocs[nsID])
	return out, nil
}

func (e *Engine) SearchProjectDocs(ctx context.Context, nsID, query string) ([]ProjectDocRef, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	q := strings.ToLower(query)
	out := make([]ProjectDocRef, 0)
	for _, doc := range e.projectDocs[nsID] {
		if strings.Contains(strings.ToLower(doc.Title), q) ||
			strings.Contains(strings.ToLower(doc.Content), q) ||
			strings.Contains(strings.ToLower(doc.Path), q) ||
			strings.Contains(strings.ToLower(doc.Section), q) {
			out = append(out, ProjectDocRef{
				ID:          doc.ID,
				NamespaceID: doc.NamespaceID,
				Section:     doc.Section,
				Path:        doc.Path,
				Title:       doc.Title,
				Tags:        doc.Tags,
				Version:     doc.Version,
				UpdatedAt:   doc.UpdatedAt,
			})
		}
	}
	return out, nil
}

func (e *Engine) DeleteProjectDoc(ctx context.Context, nsID string, docID int64) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	docs := e.projectDocs[nsID]
	for i := range docs {
		if docs[i].ID == docID {
			e.projectDocs[nsID] = append(docs[:i], docs[i+1:]...)
			if e.db != nil {
				_ = deleteProjectDoc(e.db, docID)
			}
			return nil
		}
	}
	return fmt.Errorf("doc %d not found", docID)
}
