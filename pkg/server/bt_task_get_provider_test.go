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

func TestTaskGetConfirmOnceReturnsMissingWorktreeSnapshot(t *testing.T) {
	t.Parallel()

	repoPath := initTestGitRepo(t)
	eng, err := engine.NewEngine(engine.NewEngineConfig{})
	require.NoError(t, err)
	defer eng.Close()

	srv, err := New(eng, Config{})
	require.NoError(t, err)

	_, err = eng.CreateNamespace(context.Background(), engine.CreateNamespaceRequest{ID: "ns-1", Name: "test", Metadata: map[string]string{"workdir": repoPath}})
	require.NoError(t, err)
	_, err = eng.RegisterWorker(context.Background(), engine.RegisterWorkerRequest{NamespaceID: "ns-1", ID: "worker-a", Name: "Worker A", PromptTemplate: "Task {task_id} in {worktree_path} on {branch}"})
	require.NoError(t, err)
	_, err = eng.CreateDAG(context.Background(), engine.CreateDAGRequest{NamespaceID: "ns-1", ID: "dag-1", Title: "DAG 1", ExecutionBranch: "feat/test"})
	require.NoError(t, err)
	_, err = eng.CreateTask(context.Background(), engine.CreateTaskRequest{NamespaceID: "ns-1", ID: "T1", Title: "task 1", AssignedWorker: "worker-a", DAGID: "dag-1"})
	require.NoError(t, err)

	resp, err := srv.taskGetConfirmOnce(context.Background(), "ns-1", "T1", "worker-a")
	require.NoError(t, err)
	require.Equal(t, "T1", resp.TaskID)
	require.Equal(t, "assigned", resp.State)
	require.Equal(t, "worker-a", resp.AssignedWorker)
	require.Equal(t, "feat/test", resp.Git["branch"])
	require.Equal(t, "missing", resp.Git["status"])
	require.Equal(t, []string{"worktree_get", "worker_prompt_get"}, resp.SuggestedActions)
}

func TestTaskGetConfirmOnceReturnsExistingWorktreeSnapshot(t *testing.T) {
	t.Parallel()

	repoPath := initTestGitRepo(t)
	eng, err := engine.NewEngine(engine.NewEngineConfig{})
	require.NoError(t, err)
	defer eng.Close()

	srv, err := New(eng, Config{})
	require.NoError(t, err)

	_, err = eng.CreateNamespace(context.Background(), engine.CreateNamespaceRequest{ID: "ns-1", Name: "test", Metadata: map[string]string{"workdir": repoPath}})
	require.NoError(t, err)
	_, err = eng.RegisterWorker(context.Background(), engine.RegisterWorkerRequest{NamespaceID: "ns-1", ID: "worker-a", Name: "Worker A", PromptTemplate: "Task {task_id} in {worktree_path} on {branch}"})
	require.NoError(t, err)
	_, err = eng.CreateDAG(context.Background(), engine.CreateDAGRequest{NamespaceID: "ns-1", ID: "dag-1", Title: "DAG 1", ExecutionBranch: "feat/test"})
	require.NoError(t, err)
	_, err = eng.CreateTask(context.Background(), engine.CreateTaskRequest{NamespaceID: "ns-1", ID: "T1", Title: "task 1", AssignedWorker: "worker-a", DAGID: "dag-1"})
	require.NoError(t, err)
	_, err = srv.dispatchTaskOnce(context.Background(), "ns-1", "T1")
	require.NoError(t, err)

	resp, err := srv.taskGetConfirmOnce(context.Background(), "ns-1", "T1", "worker-a")
	require.NoError(t, err)
	require.Equal(t, "executing", resp.State)
	require.Equal(t, "feat/test", resp.Git["branch"])
	require.NotEmpty(t, resp.Git["head_sha"])
	require.Contains(t, []any{"clean", "dirty"}, resp.Git["status"])
}

func TestTaskGetConfirmOnceRejectsWorkerMismatch(t *testing.T) {
	t.Parallel()

	repoPath := initTestGitRepo(t)
	eng, err := engine.NewEngine(engine.NewEngineConfig{})
	require.NoError(t, err)
	defer eng.Close()

	srv, err := New(eng, Config{})
	require.NoError(t, err)

	_, err = eng.CreateNamespace(context.Background(), engine.CreateNamespaceRequest{ID: "ns-1", Name: "test", Metadata: map[string]string{"workdir": repoPath}})
	require.NoError(t, err)
	_, err = eng.RegisterWorker(context.Background(), engine.RegisterWorkerRequest{NamespaceID: "ns-1", ID: "worker-a", Name: "Worker A", PromptTemplate: "Task {task_id} in {worktree_path} on {branch}"})
	require.NoError(t, err)
	_, err = eng.CreateDAG(context.Background(), engine.CreateDAGRequest{NamespaceID: "ns-1", ID: "dag-1", Title: "DAG 1", ExecutionBranch: "feat/test"})
	require.NoError(t, err)
	_, err = eng.CreateTask(context.Background(), engine.CreateTaskRequest{NamespaceID: "ns-1", ID: "T1", Title: "task 1", AssignedWorker: "worker-a", DAGID: "dag-1"})
	require.NoError(t, err)

	_, err = srv.taskGetConfirmOnce(context.Background(), "ns-1", "T1", "worker-b")
	require.Error(t, err)
	require.Contains(t, err.Error(), "assigned to worker")
}

func TestTaskGetConfirmProviderHandlesValidation(t *testing.T) {
	t.Parallel()

	eng, err := engine.NewEngine(engine.NewEngineConfig{})
	require.NoError(t, err)
	defer eng.Close()

	srv, err := New(eng, Config{})
	require.NoError(t, err)
	provider, err := newBTTaskGetProvider(srv)
	require.NoError(t, err)
	defer provider.close()

	req := httptest.NewRequest(http.MethodPost, "/task-get-confirm", nil)
	req.Header.Set("X-Agentflow-BT-Token", provider.token)
	rec := httptest.NewRecorder()
	provider.handleTaskGetConfirm(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)

	req = httptest.NewRequest(http.MethodPost, "/task-get-confirm", nil)
	req.Header.Set("X-Agentflow-BT-Token", "wrong")
	rec = httptest.NewRecorder()
	provider.handleTaskGetConfirm(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)

	_, err = eng.CreateNamespace(context.Background(), engine.CreateNamespaceRequest{ID: "ns-1", Name: "test"})
	require.NoError(t, err)
	_, err = eng.RegisterWorker(context.Background(), engine.RegisterWorkerRequest{NamespaceID: "ns-1", ID: "worker-a", Name: "Worker A", PromptTemplate: "Task {task_id} in {worktree_path} on {branch}"})
	require.NoError(t, err)
	req = httptest.NewRequest(http.MethodPost, "/task-get-confirm", strings.NewReader(`{"namespace_id":"ns-1","task_id":"missing","worker_id":"worker-a"}`))
	req.Header.Set("X-Agentflow-BT-Token", provider.token)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	provider.handleTaskGetConfirm(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code)
}
