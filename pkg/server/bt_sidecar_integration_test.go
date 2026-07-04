package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/toustifer/agentflow/pkg/engine"
)

func TestLeaderTickViaPythonShape(t *testing.T) {
	t.Parallel()

	// Use repo root so bt_service and trees/ are discoverable.
	require.NoError(t, os.Setenv("AGENTFLOW_BT_DIR", "D:/myprogram/agentflow"))
	defer os.Unsetenv("AGENTFLOW_BT_DIR")
	globalBTBridge = nil

	dbPath := filepath.Join(t.TempDir(), "shape.db")
	eng, err := engine.NewEngine(engine.NewEngineConfig{DBPath: dbPath})
	require.NoError(t, err)
	defer eng.Close()

	srv, err := New(eng, Config{})
	require.NoError(t, err)

	_, err = eng.CreateNamespace(context.Background(), engine.CreateNamespaceRequest{
		ID:   "ns-shape",
		Name: "shape-ns",
	})
	require.NoError(t, err)

	result, err := srv.handleLeaderTick(context.Background(), map[string]any{
		"namespace_id": "ns-shape",
	})
	require.NoError(t, err)
	require.Equal(t, "success", result["tree_status"])
	require.Equal(t, "shape", result["phase"])
	require.NotEmpty(t, result["actions"])

	if globalBTBridge != nil {
		globalBTBridge.Stop()
		globalBTBridge = nil
	}
}

func TestLeaderTickDispatchesTaskViaPython(t *testing.T) {
	t.Parallel()

	require.NoError(t, os.Setenv("AGENTFLOW_BT_DIR", "D:/myprogram/agentflow"))
	defer os.Unsetenv("AGENTFLOW_BT_DIR")
	globalBTBridge = nil

	dbPath := filepath.Join(t.TempDir(), "dispatch.db")
	repoPath := initTestGitRepo(t)
	eng, err := engine.NewEngine(engine.NewEngineConfig{DBPath: dbPath})
	require.NoError(t, err)
	defer eng.Close()

	srv, err := New(eng, Config{})
	require.NoError(t, err)

	_, err = eng.CreateNamespace(context.Background(), engine.CreateNamespaceRequest{
		ID:   "ns-dispatch",
		Name: "dispatch-ns",
		Metadata: map[string]string{
			"workdir": repoPath,
		},
	})
	require.NoError(t, err)
	_, err = eng.RegisterWorker(context.Background(), engine.RegisterWorkerRequest{
		NamespaceID: "ns-dispatch",
		ID:          "worker-a",
		Name:        "Worker A",
	})
	require.NoError(t, err)
	_, err = eng.CreateDAG(context.Background(), engine.CreateDAGRequest{
		NamespaceID: "ns-dispatch",
		ID:          "dag-1",
		Title:       "DAG 1",
		Branch:      "feat/test",
	})
	require.NoError(t, err)
	_, err = eng.CreateTask(context.Background(), engine.CreateTaskRequest{
		NamespaceID:    "ns-dispatch",
		ID:             "T1",
		Title:          "task 1",
		AssignedWorker: "worker-a",
		DAGID:          "dag-1",
	})
	require.NoError(t, err)

	result, err := srv.handleLeaderTick(context.Background(), map[string]any{
		"namespace_id": "ns-dispatch",
	})
	require.NoError(t, err)
	require.Equal(t, "success", result["tree_status"])
	require.Equal(t, "execute", result["phase"])

	task, err := eng.GetTask(context.Background(), "ns-dispatch", "T1")
	require.NoError(t, err)
	require.Equal(t, engine.TaskExecuting, task.State)
	require.NotEmpty(t, task.Metadata["git.worktree_path"])

	if globalBTBridge != nil {
		globalBTBridge.Stop()
		globalBTBridge = nil
	}
}

func TestLeaderTickDispatchTaskIsIdempotent(t *testing.T) {
	t.Parallel()

	require.NoError(t, os.Setenv("AGENTFLOW_BT_DIR", "D:/myprogram/agentflow"))
	defer os.Unsetenv("AGENTFLOW_BT_DIR")
	globalBTBridge = nil

	dbPath := filepath.Join(t.TempDir(), "dispatch-idempotent.db")
	repoPath := initTestGitRepo(t)
	eng, err := engine.NewEngine(engine.NewEngineConfig{DBPath: dbPath})
	require.NoError(t, err)
	defer eng.Close()

	srv, err := New(eng, Config{})
	require.NoError(t, err)

	_, err = eng.CreateNamespace(context.Background(), engine.CreateNamespaceRequest{
		ID:   "ns-dispatch",
		Name: "dispatch-ns",
		Metadata: map[string]string{
			"workdir": repoPath,
		},
	})
	require.NoError(t, err)
	_, err = eng.RegisterWorker(context.Background(), engine.RegisterWorkerRequest{
		NamespaceID: "ns-dispatch",
		ID:          "worker-a",
		Name:        "Worker A",
	})
	require.NoError(t, err)
	_, err = eng.CreateDAG(context.Background(), engine.CreateDAGRequest{
		NamespaceID: "ns-dispatch",
		ID:          "dag-1",
		Title:       "DAG 1",
		Branch:      "feat/test",
	})
	require.NoError(t, err)
	_, err = eng.CreateTask(context.Background(), engine.CreateTaskRequest{
		NamespaceID:    "ns-dispatch",
		ID:             "T1",
		Title:          "task 1",
		AssignedWorker: "worker-a",
		DAGID:          "dag-1",
	})
	require.NoError(t, err)

	_, err = srv.handleLeaderTick(context.Background(), map[string]any{"namespace_id": "ns-dispatch"})
	require.NoError(t, err)
	firstTask, err := eng.GetTask(context.Background(), "ns-dispatch", "T1")
	require.NoError(t, err)
	firstWorktree := firstTask.Metadata["git.worktree_path"]
	require.NotEmpty(t, firstWorktree)

	_, err = srv.handleLeaderTick(context.Background(), map[string]any{"namespace_id": "ns-dispatch"})
	require.NoError(t, err)
	secondTask, err := eng.GetTask(context.Background(), "ns-dispatch", "T1")
	require.NoError(t, err)
	require.Equal(t, engine.TaskExecuting, secondTask.State)
	require.Equal(t, firstWorktree, secondTask.Metadata["git.worktree_path"])

	if globalBTBridge != nil {
		globalBTBridge.Stop()
		globalBTBridge = nil
	}
}
