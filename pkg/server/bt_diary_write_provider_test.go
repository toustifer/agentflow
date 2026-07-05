package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/toustifer/agentflow/pkg/engine"
)

func TestDiaryWriteEntryOnceCreatesDiaryEntry(t *testing.T) {
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

	resp, err := srv.diaryWriteEntryOnce(context.Background(), "ns-1", "worker-a", "T1", "", "did the work", []string{"task"})
	require.NoError(t, err)
	require.Equal(t, "worker-a", resp.WorkerID)
	require.Equal(t, "T1", resp.TaskID)
	require.Equal(t, time.Now().UTC().Format("2006-01-02"), resp.Date)

	diary, err := eng.GetWorkerDiary(context.Background(), "ns-1", "worker-a", resp.Date)
	require.NoError(t, err)
	require.Equal(t, "did the work", diary.Content)
	require.Equal(t, []string{"task"}, diary.Tags)
}

func TestDiaryWriteEntryOnceRejectsWorkerMismatch(t *testing.T) {
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

	_, err = srv.diaryWriteEntryOnce(context.Background(), "ns-1", "worker-b", "T1", "", "did the work", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "assigned to worker")
}

func TestDiaryWriteEntryProviderHandlesValidation(t *testing.T) {
	t.Parallel()

	eng, err := engine.NewEngine(engine.NewEngineConfig{})
	require.NoError(t, err)
	defer eng.Close()

	srv, err := New(eng, Config{})
	require.NoError(t, err)
	provider, err := newBTDiaryWriteProvider(srv)
	require.NoError(t, err)
	defer provider.close()

	req := httptest.NewRequest(http.MethodPost, "/diary-write-entry", nil)
	req.Header.Set("X-Agentflow-BT-Token", provider.token)
	rec := httptest.NewRecorder()
	provider.handleDiaryWriteEntry(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)

	req = httptest.NewRequest(http.MethodPost, "/diary-write-entry", nil)
	req.Header.Set("X-Agentflow-BT-Token", "wrong")
	rec = httptest.NewRecorder()
	provider.handleDiaryWriteEntry(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)

	_, err = eng.CreateNamespace(context.Background(), engine.CreateNamespaceRequest{ID: "ns-1", Name: "test"})
	require.NoError(t, err)
	req = httptest.NewRequest(http.MethodPost, "/diary-write-entry", strings.NewReader(`{"namespace_id":"ns-1","worker_id":"worker-a","content":"done"}`))
	req.Header.Set("X-Agentflow-BT-Token", provider.token)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	provider.handleDiaryWriteEntry(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code)
}
