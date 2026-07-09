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

func TestReportDoneOnceReturnsDoneSnapshot(t *testing.T) {
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
	_, err = eng.CreateDAG(context.Background(), engine.CreateDAGRequest{NamespaceID: "ns-1", ID: "dag-1", Title: "DAG 1", ExecutionBranch: "feat/test"})
	require.NoError(t, err)
	_, err = eng.CreateTask(context.Background(), engine.CreateTaskRequest{NamespaceID: "ns-1", ID: "T1", Title: "task 1", AssignedWorker: "worker-a", DAGID: "dag-1"})
	require.NoError(t, err)
	_, err = eng.TransitionTask(context.Background(), "ns-1", "T1", engine.TransStart, map[string]string{"actor_role": "leader"})
	require.NoError(t, err)
	_, err = eng.TransitionTask(context.Background(), "ns-1", "T1", engine.TransSubmit, map[string]string{"actor_role": "worker"})
	require.NoError(t, err)
	_, err = eng.TransitionTask(context.Background(), "ns-1", "T1", engine.TransPass, map[string]string{"actor_role": "reviewer"})
	require.NoError(t, err)

	resp, err := srv.reportDoneOnce(context.Background(), "ns-1")
	require.NoError(t, err)
	require.Equal(t, "done", resp.Phase)
	require.Equal(t, 1, resp.CompletedTasks)
	require.Equal(t, 1, resp.TotalTasks)
	require.Equal(t, 100, resp.CompletionPct)
	require.Equal(t, []string{"goal", "doc_list"}, resp.SuggestedActions)
	require.NotNil(t, resp.DAG)
}

func TestReportDoneOnceRejectsNonDonePhase(t *testing.T) {
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
	_, err = eng.CreateDAG(context.Background(), engine.CreateDAGRequest{NamespaceID: "ns-1", ID: "dag-1", Title: "DAG 1", ExecutionBranch: "feat/test"})
	require.NoError(t, err)
	_, err = eng.CreateTask(context.Background(), engine.CreateTaskRequest{NamespaceID: "ns-1", ID: "T1", Title: "task 1", AssignedWorker: "worker-a", DAGID: "dag-1"})
	require.NoError(t, err)

	_, err = srv.reportDoneOnce(context.Background(), "ns-1")
	require.Error(t, err)
	require.Contains(t, err.Error(), "phase")
}

func TestReportDoneProviderHandlesValidation(t *testing.T) {
	t.Parallel()

	eng, err := engine.NewEngine(engine.NewEngineConfig{})
	require.NoError(t, err)
	defer eng.Close()

	srv, err := New(eng, Config{})
	require.NoError(t, err)
	provider, err := newBTDoneProvider(srv)
	require.NoError(t, err)
	defer provider.close()

	req := httptest.NewRequest(http.MethodPost, "/report-done", nil)
	req.Header.Set("X-Agentflow-BT-Token", provider.token)
	rec := httptest.NewRecorder()
	provider.handleReportDone(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)

	req = httptest.NewRequest(http.MethodPost, "/report-done", nil)
	req.Header.Set("X-Agentflow-BT-Token", "wrong")
	rec = httptest.NewRecorder()
	provider.handleReportDone(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)

	_, err = eng.CreateNamespace(context.Background(), engine.CreateNamespaceRequest{ID: "ns-1", Name: "test"})
	require.NoError(t, err)
	req = httptest.NewRequest(http.MethodPost, "/report-done", strings.NewReader(`{"namespace_id":"ns-1"}`))
	req.Header.Set("X-Agentflow-BT-Token", provider.token)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	provider.handleReportDone(rec, req)
	require.Equal(t, http.StatusConflict, rec.Code)
}
