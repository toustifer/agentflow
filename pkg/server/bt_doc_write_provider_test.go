package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/toustifer/agentflow/pkg/engine"
)

func TestDocWriteRecordOnceCreatesProjectDoc(t *testing.T) {
	t.Parallel()

	eng, err := engine.NewEngine(engine.NewEngineConfig{})
	require.NoError(t, err)
	defer eng.Close()

	srv, err := New(eng, Config{})
	require.NoError(t, err)

	_, err = eng.CreateNamespace(context.Background(), engine.CreateNamespaceRequest{ID: "ns-1", Name: "test"})
	require.NoError(t, err)
	_, err = eng.RegisterWorker(context.Background(), engine.RegisterWorkerRequest{NamespaceID: "ns-1", ID: "worker-a", Name: "Worker A"})
	require.NoError(t, err)
	_, err = eng.CreateTask(context.Background(), engine.CreateTaskRequest{NamespaceID: "ns-1", ID: "T1", Title: "task 1", AssignedWorker: "worker-a"})
	require.NoError(t, err)

	resp, err := srv.docWriteRecordOnce(context.Background(), "ns-1", "T1", "worker-a", "Implementation Note", "doc body", "tasks/T1.md", "tasks", []string{"task", "worker"}, 0)
	require.NoError(t, err)
	require.Positive(t, resp.DocID)
	require.Equal(t, "Implementation Note", resp.Title)
	require.Equal(t, "tasks/T1.md", resp.Path)
	require.Equal(t, "tasks", resp.Section)

	doc, err := eng.GetProjectDoc(context.Background(), "ns-1", resp.DocID)
	require.NoError(t, err)
	require.Equal(t, "doc body", doc.Content)
	require.Equal(t, []string{"task", "worker"}, doc.Tags)
}

func TestDocWriteRecordOnceRejectsWorkerMismatch(t *testing.T) {
	t.Parallel()

	eng, err := engine.NewEngine(engine.NewEngineConfig{})
	require.NoError(t, err)
	defer eng.Close()

	srv, err := New(eng, Config{})
	require.NoError(t, err)

	_, err = eng.CreateNamespace(context.Background(), engine.CreateNamespaceRequest{ID: "ns-1", Name: "test"})
	require.NoError(t, err)
	_, err = eng.RegisterWorker(context.Background(), engine.RegisterWorkerRequest{NamespaceID: "ns-1", ID: "worker-a", Name: "Worker A"})
	require.NoError(t, err)
	_, err = eng.RegisterWorker(context.Background(), engine.RegisterWorkerRequest{NamespaceID: "ns-1", ID: "worker-b", Name: "Worker B"})
	require.NoError(t, err)
	_, err = eng.CreateTask(context.Background(), engine.CreateTaskRequest{NamespaceID: "ns-1", ID: "T1", Title: "task 1", AssignedWorker: "worker-a"})
	require.NoError(t, err)

	_, err = srv.docWriteRecordOnce(context.Background(), "ns-1", "T1", "worker-b", "Implementation Note", "doc body", "", "", nil, 0)
	require.Error(t, err)
	require.Contains(t, err.Error(), "assigned to worker")
}

func TestDocWriteRecordProviderHandlesValidation(t *testing.T) {
	t.Parallel()

	eng, err := engine.NewEngine(engine.NewEngineConfig{})
	require.NoError(t, err)
	defer eng.Close()

	srv, err := New(eng, Config{})
	require.NoError(t, err)
	provider, err := newBTDocWriteProvider(srv)
	require.NoError(t, err)
	defer provider.close()

	req := httptest.NewRequest(http.MethodPost, "/doc-write-record", nil)
	req.Header.Set("X-Agentflow-BT-Token", provider.token)
	rec := httptest.NewRecorder()
	provider.handleDocWriteRecord(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)

	req = httptest.NewRequest(http.MethodPost, "/doc-write-record", nil)
	req.Header.Set("X-Agentflow-BT-Token", "wrong")
	rec = httptest.NewRecorder()
	provider.handleDocWriteRecord(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)

	_, err = eng.CreateNamespace(context.Background(), engine.CreateNamespaceRequest{ID: "ns-1", Name: "test"})
	require.NoError(t, err)
	req = httptest.NewRequest(http.MethodPost, "/doc-write-record", strings.NewReader(`{"namespace_id":"ns-1","task_id":"missing","worker_id":"worker-a","content":"body"}`))
	req.Header.Set("X-Agentflow-BT-Token", provider.token)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	provider.handleDocWriteRecord(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code)
}
