package server

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/toustifer/agentflow/pkg/engine"
)

func TestDocsSyncExportAndAutoMirror(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()
	nsID := "ns-1"

	ns, err := srv.engine.GetNamespace(ctx, nsID)
	require.NoError(t, err)
	workdir := getWorkdir(ns)
	require.NotEmpty(t, workdir)

	_, err = srv.engine.RegisterWorker(ctx, engine.RegisterWorkerRequest{
		NamespaceID:    nsID,
		ID:             "worker-docs",
		Name:           "Docs Worker",
		PromptTemplate: "do {task_id}",
	})
	require.NoError(t, err)

	// Auto-mirror path: doc_write should export without explicit docs_sync
	docRes, err := srv.Handle(ctx, "doc_write", map[string]any{
		"namespace_id": nsID,
		"section":      "architecture",
		"path":         "overview.md",
		"title":        "Overview",
		"content":      "System overview for sync test.",
		"tags":         []any{"sync", "docs"},
	})
	require.NoError(t, err)
	docID := docRes["id"]
	require.NotNil(t, docID)

	root := filepath.Join(workdir, docsSyncRootRel)
	manifestPath := filepath.Join(root, "MANIFEST.json")
	require.FileExists(t, manifestPath)

	docFile := filepath.Join(root, "docs", "architecture", "overview.md")
	require.FileExists(t, docFile)
	raw, err := os.ReadFile(docFile)
	require.NoError(t, err)
	require.Contains(t, string(raw), "System overview for sync test.")
	require.Contains(t, string(raw), "section: \"architecture\"")

	// Explicit export after handbook + diaries
	_, err = srv.Handle(ctx, "worker_handbook_write", map[string]any{
		"namespace_id": nsID,
		"worker_id":    "worker-docs",
		"scope":        "docs domain",
		"tech_stack":   []any{"go", "sqlite"},
	})
	require.NoError(t, err)

	_, err = srv.Handle(ctx, "worker_diary_write", map[string]any{
		"namespace_id": nsID,
		"worker_id":    "worker-docs",
		"date":         "2026-07-20",
		"content":      "Mirrored diary entry.",
		"tags":         []any{"diary"},
	})
	require.NoError(t, err)

	_, err = srv.Handle(ctx, "leader_diary_write", map[string]any{
		"namespace_id": nsID,
		"date":         "2026-07-20",
		"type":         "decision",
		"title":        "Sync layout",
		"content":      "Use .mycompany/agentflow for same-repo docs.",
	})
	require.NoError(t, err)

	exp, err := srv.Handle(ctx, "docs_sync", map[string]any{
		"namespace_id": nsID,
		"direction":    "export",
	})
	require.NoError(t, err)
	require.Equal(t, "export", exp["direction"])
	exportBlock := exp["export"].(map[string]any)
	require.Equal(t, 1, mapCount(exportBlock["counts"], "docs"))
	require.Equal(t, 1, mapCount(exportBlock["counts"], "handbooks"))
	require.Equal(t, 1, mapCount(exportBlock["counts"], "worker_diaries"))
	require.Equal(t, 1, mapCount(exportBlock["counts"], "leader_diaries"))

	require.FileExists(t, filepath.Join(root, "workers", "worker-docs", "handbook.json"))
	require.FileExists(t, filepath.Join(root, "workers", "worker-docs", "diary", "2026-07-20.md"))
	require.FileExists(t, filepath.Join(root, "leader", "diary", "2026-07-20.md"))

	manifestRaw, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	var manifest map[string]any
	require.NoError(t, json.Unmarshal(manifestRaw, &manifest))
	require.Equal(t, "v1", manifest["layout"])
	require.Equal(t, nsID, manifest["namespace_id"])
	hash, ok := manifest["content_tree_hash"].(string)
	require.True(t, ok && len(hash) == 64, "content_tree_hash sha256 hex expected, got %v", manifest["content_tree_hash"])
	require.Equal(t, hash, exportBlock["content_tree_hash"])
}

func TestDocsSyncImportCreatesMissingDoc(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()
	nsID := "ns-1"

	ns, err := srv.engine.GetNamespace(ctx, nsID)
	require.NoError(t, err)
	workdir := getWorkdir(ns)

	// Seed export tree by writing then deleting the SQLite doc
	_, err = srv.Handle(ctx, "doc_write", map[string]any{
		"namespace_id": nsID,
		"section":      "api",
		"path":         "routes.md",
		"title":        "Routes",
		"content":      "Import me back.",
	})
	require.NoError(t, err)

	list, err := srv.Handle(ctx, "doc_list", map[string]any{"namespace_id": nsID})
	require.NoError(t, err)
	docs := list["docs"].([]any)
	require.Len(t, docs, 1)
	id := asInt64(docs[0].(map[string]any)["id"])

	_, err = srv.Handle(ctx, "doc_delete", map[string]any{
		"namespace_id": nsID,
		"id":           id,
	})
	require.NoError(t, err)

	// After delete, auto-mirror empties docs tree — rewrite a file with stale id for import fallback
	docsDir := filepath.Join(workdir, docsSyncRootRel, "docs", "api")
	require.NoError(t, os.MkdirAll(docsDir, 0o755))
	body := "---\nid: 99999\nsection: \"api\"\npath: \"routes.md\"\ntitle: \"Routes\"\n---\n\nImport me back.\n"
	require.NoError(t, os.WriteFile(filepath.Join(docsDir, "routes.md"), []byte(body), 0o644))

	imp, err := srv.Handle(ctx, "docs_sync", map[string]any{
		"namespace_id": nsID,
		"direction":    "import",
	})
	require.NoError(t, err)
	importBlock := imp["import"].(map[string]any)
	require.Equal(t, 1, mapCount(importBlock["counts"], "docs"))

	list, err = srv.Handle(ctx, "doc_list", map[string]any{"namespace_id": nsID})
	require.NoError(t, err)
	docs = list["docs"].([]any)
	require.Len(t, docs, 1)
	item := docs[0].(map[string]any)
	require.Equal(t, "Routes", item["title"])
	require.Contains(t, item["content"], "Import me back.")
}

func TestDocsSyncRejectsMissingWorkdir(t *testing.T) {
	t.Parallel()

	eng, err := engine.NewEngine(engine.NewEngineConfig{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, eng.Close()) })

	_, err = eng.CreateNamespace(context.Background(), engine.CreateNamespaceRequest{
		ID:   "ns-nowd",
		Name: "no-workdir",
	})
	require.NoError(t, err)

	srv, err := New(eng, Config{})
	require.NoError(t, err)

	_, err = srv.Handle(context.Background(), "docs_sync", map[string]any{
		"namespace_id": "ns-nowd",
		"direction":    "export",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "no workdir")
}

func mapCount(v any, key string) int {
	switch m := v.(type) {
	case map[string]int:
		return m[key]
	case map[string]any:
		return asInt(m[key])
	default:
		return -1
	}
}

func asInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return -1
	}
}

func asInt64(v any) int64 {
	switch n := v.(type) {
	case int:
		return int64(n)
	case int64:
		return n
	case float64:
		return int64(n)
	default:
		return 0
	}
}
