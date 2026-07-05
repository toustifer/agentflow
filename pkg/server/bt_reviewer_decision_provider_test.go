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

func TestTaskReviewPassOnceTransitionsToDone(t *testing.T) {
	t.Parallel()

	eng, err := engine.NewEngine(engine.NewEngineConfig{})
	require.NoError(t, err)
	defer eng.Close()
	srv, err := New(eng, Config{})
	require.NoError(t, err)
	_, err = eng.CreateNamespace(context.Background(), engine.CreateNamespaceRequest{ID: "ns-1", Name: "test"})
	require.NoError(t, err)
	_, err = eng.RegisterWorker(context.Background(), engine.RegisterWorkerRequest{NamespaceID: "ns-1", ID: "reviewer-a", Name: "Reviewer A"})
	require.NoError(t, err)
	_, err = eng.CreateTask(context.Background(), engine.CreateTaskRequest{NamespaceID: "ns-1", ID: "T1", Title: "task 1"})
	require.NoError(t, err)
	_, err = eng.UpdateTask(context.Background(), "ns-1", "T1", engine.UpdateTaskRequest{State: engine.TaskReviewPending})
	require.NoError(t, err)
	resp, err := srv.taskReviewPassOnce(context.Background(), "ns-1", "T1", "reviewer-a")
	require.NoError(t, err)
	require.Equal(t, "done", resp.State)
}

func TestTaskReviewReworkOnceTransitionsToReworkNeeded(t *testing.T) {
	t.Parallel()

	eng, err := engine.NewEngine(engine.NewEngineConfig{})
	require.NoError(t, err)
	defer eng.Close()
	srv, err := New(eng, Config{})
	require.NoError(t, err)
	_, err = eng.CreateNamespace(context.Background(), engine.CreateNamespaceRequest{ID: "ns-1", Name: "test"})
	require.NoError(t, err)
	_, err = eng.RegisterWorker(context.Background(), engine.RegisterWorkerRequest{NamespaceID: "ns-1", ID: "reviewer-a", Name: "Reviewer A"})
	require.NoError(t, err)
	_, err = eng.CreateTask(context.Background(), engine.CreateTaskRequest{NamespaceID: "ns-1", ID: "T1", Title: "task 1"})
	require.NoError(t, err)
	_, err = eng.UpdateTask(context.Background(), "ns-1", "T1", engine.UpdateTaskRequest{State: engine.TaskReviewPending})
	require.NoError(t, err)
	resp, err := srv.taskReviewReworkOnce(context.Background(), "ns-1", "T1", "reviewer-a")
	require.NoError(t, err)
	require.Equal(t, "rework_needed", resp.State)
}

func TestTaskReviewPassOnceRejectsUnknownReviewer(t *testing.T) {
	t.Parallel()

	eng, err := engine.NewEngine(engine.NewEngineConfig{})
	require.NoError(t, err)
	defer eng.Close()
	srv, err := New(eng, Config{})
	require.NoError(t, err)
	_, err = eng.CreateNamespace(context.Background(), engine.CreateNamespaceRequest{ID: "ns-1", Name: "test"})
	require.NoError(t, err)
	_, err = eng.CreateTask(context.Background(), engine.CreateTaskRequest{NamespaceID: "ns-1", ID: "T1", Title: "task 1"})
	require.NoError(t, err)
	_, err = eng.UpdateTask(context.Background(), "ns-1", "T1", engine.UpdateTaskRequest{State: engine.TaskReviewPending})
	require.NoError(t, err)

	_, err = srv.taskReviewPassOnce(context.Background(), "ns-1", "T1", "missing-reviewer")
	require.Error(t, err)
}

func TestTaskReviewPassProviderHandlesValidation(t *testing.T) {
	t.Parallel()
	eng, err := engine.NewEngine(engine.NewEngineConfig{})
	require.NoError(t, err)
	defer eng.Close()
	srv, err := New(eng, Config{})
	require.NoError(t, err)
	provider, err := newBTReviewPassProvider(srv)
	require.NoError(t, err)
	defer provider.close()
	req := httptest.NewRequest(http.MethodPost, "/task-review-pass", nil)
	req.Header.Set("X-Agentflow-BT-Token", provider.token)
	rec := httptest.NewRecorder()
	provider.handleTaskReviewPass(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	req = httptest.NewRequest(http.MethodPost, "/task-review-pass", nil)
	req.Header.Set("X-Agentflow-BT-Token", "wrong")
	rec = httptest.NewRecorder()
	provider.handleTaskReviewPass(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
	_, err = eng.CreateNamespace(context.Background(), engine.CreateNamespaceRequest{ID: "ns-1", Name: "test"})
	require.NoError(t, err)
	req = httptest.NewRequest(http.MethodPost, "/task-review-pass", strings.NewReader(`{"namespace_id":"ns-1","task_id":"missing","worker_id":"reviewer-a"}`))
	req.Header.Set("X-Agentflow-BT-Token", provider.token)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	provider.handleTaskReviewPass(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code)
}
