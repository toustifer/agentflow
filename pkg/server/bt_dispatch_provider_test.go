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

func TestDispatchTaskOnceTransitionsAssignedTask(t *testing.T) {
	t.Parallel()

	repoPath := initTestGitRepo(t)
	eng, err := engine.NewEngine(engine.NewEngineConfig{})
	require.NoError(t, err)
	defer eng.Close()

	srv, err := New(eng, Config{})
	require.NoError(t, err)

	_, err = eng.CreateNamespace(context.Background(), engine.CreateNamespaceRequest{
		ID:   "ns-1",
		Name: "test",
		Metadata: map[string]string{
			"workdir": repoPath,
		},
	})
	require.NoError(t, err)
	_, err = eng.RegisterWorker(context.Background(), engine.RegisterWorkerRequest{
		NamespaceID:    "ns-1",
		ID:             "worker-a",
		Name:           "Worker A",
		PromptTemplate: "Task {task_id} in {worktree_path} on {branch}",
	})
	require.NoError(t, err)
	_, err = eng.CreateDAG(context.Background(), engine.CreateDAGRequest{
		NamespaceID: "ns-1",
		ID:          "dag-1",
		Title:       "DAG 1",
		Branch:      "feat/test",
	})
	require.NoError(t, err)
	_, err = eng.CreateTask(context.Background(), engine.CreateTaskRequest{
		NamespaceID:    "ns-1",
		ID:             "T1",
		Title:          "task 1",
		AssignedWorker: "worker-a",
		DAGID:          "dag-1",
	})
	require.NoError(t, err)

	resp, err := srv.dispatchTaskOnce(context.Background(), "ns-1", "T1")
	require.NoError(t, err)
	require.Equal(t, "T1", resp.TaskID)
	require.Equal(t, "executing", resp.State)
	require.Equal(t, "worker-a", resp.AssignedWorker)
	require.Equal(t, "feat/test", resp.Branch)
	require.NotEmpty(t, resp.WorktreePath)
	require.NotNil(t, resp.WorkerLaunch)
	require.Equal(t, true, resp.WorkerLaunch["required"])
	require.Equal(t, false, resp.WorkerLaunch["started"])
	require.Equal(t, "launch_worker_manually", resp.WorkerLaunch["leader_next_action"])
	require.Equal(t, "worker-a", resp.WorkerLaunch["worker_id"])
	require.Equal(t, "T1", resp.WorkerLaunch["task_id"])
	require.NotEmpty(t, resp.WorkerLaunch["prompt_template"])

	task, err := eng.GetTask(context.Background(), "ns-1", "T1")
	require.NoError(t, err)
	require.Equal(t, engine.TaskExecuting, task.State)
	require.Equal(t, resp.WorktreePath, task.Metadata["git.worktree_path"])
}

func TestDispatchTaskOnceIsIdempotentForExecutingTask(t *testing.T) {
	t.Parallel()

	repoPath := initTestGitRepo(t)
	eng, err := engine.NewEngine(engine.NewEngineConfig{})
	require.NoError(t, err)
	defer eng.Close()

	srv, err := New(eng, Config{})
	require.NoError(t, err)

	_, err = eng.CreateNamespace(context.Background(), engine.CreateNamespaceRequest{
		ID:   "ns-1",
		Name: "test",
		Metadata: map[string]string{
			"workdir": repoPath,
		},
	})
	require.NoError(t, err)
	_, err = eng.RegisterWorker(context.Background(), engine.RegisterWorkerRequest{
		NamespaceID:    "ns-1",
		ID:             "worker-a",
		Name:           "Worker A",
		PromptTemplate: "Task {task_id} in {worktree_path} on {branch}",
	})
	require.NoError(t, err)
	_, err = eng.CreateDAG(context.Background(), engine.CreateDAGRequest{
		NamespaceID: "ns-1",
		ID:          "dag-1",
		Title:       "DAG 1",
		Branch:      "feat/test",
	})
	require.NoError(t, err)
	_, err = eng.CreateTask(context.Background(), engine.CreateTaskRequest{
		NamespaceID:    "ns-1",
		ID:             "T1",
		Title:          "task 1",
		AssignedWorker: "worker-a",
		DAGID:          "dag-1",
	})
	require.NoError(t, err)

	first, err := srv.dispatchTaskOnce(context.Background(), "ns-1", "T1")
	require.NoError(t, err)
	second, err := srv.dispatchTaskOnce(context.Background(), "ns-1", "T1")
	require.NoError(t, err)
	require.Equal(t, first.TaskID, second.TaskID)
	require.Equal(t, first.State, second.State)
	require.Equal(t, first.WorktreePath, second.WorktreePath)
	require.Equal(t, first.Branch, second.Branch)
	require.Equal(t, first.WorkerLaunch["leader_next_action"], second.WorkerLaunch["leader_next_action"])
	require.Equal(t, first.WorkerLaunch["worker_id"], second.WorkerLaunch["worker_id"])
	require.Equal(t, first.WorkerLaunch["task_id"], second.WorkerLaunch["task_id"])
}

func TestDispatchTaskOnceRejectsCompletedTask(t *testing.T) {
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
	_, err = eng.TransitionTask(context.Background(), "ns-1", "T1", engine.TransCancel, map[string]string{"actor_role": "leader"})
	require.NoError(t, err)

	_, err = srv.dispatchTaskOnce(context.Background(), "ns-1", "T1")
	require.Error(t, err)
	require.Contains(t, err.Error(), "state")
}

func TestDispatchProviderHandlesValidationAndTaskNotFound(t *testing.T) {
	t.Parallel()

	eng, err := engine.NewEngine(engine.NewEngineConfig{})
	require.NoError(t, err)
	defer eng.Close()

	srv, err := New(eng, Config{})
	require.NoError(t, err)
	provider, err := newBTDispatchProvider(srv)
	require.NoError(t, err)
	defer provider.close()

	req := httptest.NewRequest(http.MethodPost, "/dispatch-task", nil)
	req.Header.Set("X-Agentflow-BT-Token", provider.token)
	rec := httptest.NewRecorder()
	provider.handleDispatchTask(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)

	req = httptest.NewRequest(http.MethodPost, "/dispatch-task", nil)
	req.Header.Set("X-Agentflow-BT-Token", "wrong")
	rec = httptest.NewRecorder()
	provider.handleDispatchTask(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)

	_, err = eng.CreateNamespace(context.Background(), engine.CreateNamespaceRequest{ID: "ns-1", Name: "test"})
	require.NoError(t, err)

	req = httptest.NewRequest(http.MethodPost, "/dispatch-task", strings.NewReader(`{"namespace_id":"ns-1","task_id":"missing"}`))
	req.Header.Set("X-Agentflow-BT-Token", provider.token)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	provider.handleDispatchTask(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code)
}
