package server

import (
	"context"
	"time"

	"github.com/toustifer/agentflow/pkg/engine"
)

// ---------------------------------------------------------------------------
// MCP handlers for project_docs
// ---------------------------------------------------------------------------

// doc_write - Create or update a project doc
func (s *Server) handleDocWrite(ctx context.Context, input map[string]any) (map[string]any, error) {
	nsID, err := requiredString(input, "namespace_id")
	if err != nil {
		return nil, err
	}

	section, _ := optionalString(input, "section")
	path, _ := optionalString(input, "path")
	title, _ := optionalString(input, "title")
	content, _ := optionalString(input, "content")
	tags, _ := optionalStringSlice(input, "tags")
	docID, _ := optionalInt64(input, "id")

	doc := engine.ProjectDoc{
		ID:      docID,
		Section: section,
		Path:    path,
		Title:   title,
		Content: content,
		Tags:    tags,
	}

	result, err := s.engine.WriteProjectDoc(ctx, nsID, doc)
	if err != nil {
		return nil, err
	}
	return projectDocToMap(result), nil
}

// doc_get - Get a single doc by ID
func (s *Server) handleDocGet(ctx context.Context, input map[string]any) (map[string]any, error) {
	nsID, err := requiredString(input, "namespace_id")
	if err != nil {
		return nil, err
	}
	docID, err := requiredInt64(input, "id")
	if err != nil {
		return nil, err
	}

	doc, err := s.engine.GetProjectDoc(ctx, nsID, docID)
	if err != nil {
		return nil, err
	}
	return projectDocToMap(doc), nil
}

// doc_list - List all docs in a namespace
func (s *Server) handleDocList(ctx context.Context, input map[string]any) (map[string]any, error) {
	nsID, err := requiredString(input, "namespace_id")
	if err != nil {
		return nil, err
	}

	docs, err := s.engine.ListProjectDocs(ctx, nsID)
	if err != nil {
		return nil, err
	}

	items := make([]any, 0, len(docs))
	for i := range docs {
		items = append(items, projectDocToMap(&docs[i]))
	}
	return map[string]any{"docs": items}, nil
}

// doc_search - Full-text search across docs
func (s *Server) handleDocSearch(ctx context.Context, input map[string]any) (map[string]any, error) {
	nsID, err := requiredString(input, "namespace_id")
	if err != nil {
		return nil, err
	}
	query, err := requiredString(input, "query")
	if err != nil {
		return nil, err
	}

	results, err := s.engine.SearchProjectDocs(ctx, nsID, query)
	if err != nil {
		return nil, err
	}

	items := make([]any, 0, len(results))
	for _, r := range results {
		items = append(items, map[string]any{
			"id":           r.ID,
			"section":      r.Section,
			"path":         r.Path,
			"title":        r.Title,
			"tags":         r.Tags,
			"version":      r.Version,
			"updated_at":   r.UpdatedAt.Format(time.RFC3339Nano),
		})
	}
	return map[string]any{"results": items}, nil
}

// doc_delete - Delete a doc by ID
func (s *Server) handleDocDelete(ctx context.Context, input map[string]any) (map[string]any, error) {
	nsID, err := requiredString(input, "namespace_id")
	if err != nil {
		return nil, err
	}
	docID, err := requiredInt64(input, "id")
	if err != nil {
		return nil, err
	}

	if err := s.engine.DeleteProjectDoc(ctx, nsID, docID); err != nil {
		return nil, err
	}
	return map[string]any{"deleted": docID}, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func projectDocToMap(doc *engine.ProjectDoc) map[string]any {
	return map[string]any{
		"id":           doc.ID,
		"namespace_id": doc.NamespaceID,
		"section":      doc.Section,
		"path":         doc.Path,
		"title":        doc.Title,
		"content":      doc.Content,
		"tags":         doc.Tags,
		"version":      doc.Version,
		"created_at":   doc.CreatedAt.Format(time.RFC3339Nano),
		"updated_at":   doc.UpdatedAt.Format(time.RFC3339Nano),
	}
}
