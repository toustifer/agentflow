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

// WriteProjectDoc creates or updates a project doc.
// On update, if ExpectedVersion is set (>0) and does not match the stored version,
// returns ErrDocVersionConflict so the client can re-read and retry (CAS).
// When ExpectedVersion is 0, the write is unconditional (legacy clients / import).
func (e *Engine) WriteProjectDoc(ctx context.Context, nsID string, doc ProjectDoc) (*ProjectDoc, error) {
	return e.WriteProjectDocCAS(ctx, nsID, doc, 0)
}

// WriteProjectDocCAS is like WriteProjectDoc with explicit expected_version CAS.
// expectedVersion==0 means "no CAS check" (force overwrite / create).
func (e *Engine) WriteProjectDocCAS(ctx context.Context, nsID string, doc ProjectDoc, expectedVersion int) (*ProjectDoc, error) {
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
		} else {
			if e.nextProjectDocID[nsID] == 0 {
				e.nextProjectDocID[nsID] = nextProjectDocID(e.projectDocs[nsID])
			} else {
				e.nextProjectDocID[nsID]++
			}
			doc.ID = e.nextProjectDocID[nsID]
		}
	} else {
		// Update existing — bump version with optional CAS
		for i := range e.projectDocs[nsID] {
			if e.projectDocs[nsID][i].ID == doc.ID {
				cur := e.projectDocs[nsID][i]
				if expectedVersion > 0 && cur.Version != expectedVersion {
					return nil, fmt.Errorf("%w: doc %d expected version %d, have %d — re-read and retry",
						ErrDocVersionConflict, doc.ID, expectedVersion, cur.Version)
				}
				// Preserve fields not supplied by caller (partial update safety)
				if doc.Section == "" {
					doc.Section = cur.Section
				}
				if doc.Path == "" {
					doc.Path = cur.Path
				}
				if doc.Title == "" {
					doc.Title = cur.Title
				}
				// Content empty is allowed only if caller intentionally clears; if both empty keep old
				if doc.Content == "" && cur.Content != "" && expectedVersion == 0 {
					// keep caller content (empty) for force; CAS clients usually send full content
				}
				doc.NamespaceID = nsID
				doc.Version = cur.Version + 1
				doc.CreatedAt = cur.CreatedAt
				doc.UpdatedAt = now
				if e.db != nil {
					// SQL CAS: when expectedVersion > 0, require matching version row
					n, err := updateProjectDocCAS(e.db, &doc, expectedVersion)
					if err != nil {
						return nil, err
					}
					if expectedVersion > 0 && n == 0 {
						return nil, fmt.Errorf("%w: doc %d expected version %d (sql)",
							ErrDocVersionConflict, doc.ID, expectedVersion)
					}
				}
				e.projectDocs[nsID][i] = doc
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

func nextProjectDocID(docs []ProjectDoc) int64 {
	var maxID int64
	for _, doc := range docs {
		if doc.ID > maxID {
			maxID = doc.ID
		}
	}
	return maxID + 1
}
