package server

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/toustifer/agentflow/pkg/engine"
)

func useBTTestEnv(t *testing.T) {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	require.NoError(t, os.Setenv("AGENTFLOW_BT_DIR", repoRoot))
	t.Cleanup(func() {
		os.Unsetenv("AGENTFLOW_BT_DIR")
		if globalBTBridge != nil {
			globalBTBridge.Stop()
			globalBTBridge = nil
		}
	})
	globalBTBridge = nil
}

func decodeBlackboard(t *testing.T, payload map[string]any) map[string]any {
	t.Helper()
	bb, ok := payload["blackboard"].(map[string]any)
	if ok {
		return bb
	}
	var raw map[string]any
	bytes, err := json.Marshal(payload["blackboard"])
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(bytes, &raw))
	return raw
}

func TestLeaderTickViaPythonShape(t *testing.T) {
	// Use repo root so bt_service and trees/ are discoverable.
	useBTTestEnv(t)

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

}

func TestLeaderTickDispatchesTaskViaPython(t *testing.T) {
	useBTTestEnv(t)

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
		PromptTemplate: "Task {task_id} in {worktree_path} on {branch}",
	})
	require.NoError(t, err)
	_, err = eng.CreateDAG(context.Background(), engine.CreateDAGRequest{
		NamespaceID: "ns-dispatch",
		ID:          "dag-1",
		Title:       "DAG 1",
		ExecutionBranch:      "feat/test",
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

}

func TestLeaderTickMonitorTasksViaPython(t *testing.T) {
	useBTTestEnv(t)

	dbPath := filepath.Join(t.TempDir(), "monitor.db")
	repoPath := initTestGitRepo(t)
	eng, err := engine.NewEngine(engine.NewEngineConfig{DBPath: dbPath})
	require.NoError(t, err)
	defer eng.Close()

	srv, err := New(eng, Config{})
	require.NoError(t, err)

	_, err = eng.CreateNamespace(context.Background(), engine.CreateNamespaceRequest{
		ID:   "ns-monitor",
		Name: "monitor-ns",
		Metadata: map[string]string{
			"workdir": repoPath,
		},
	})
	require.NoError(t, err)
	_, err = eng.RegisterWorker(context.Background(), engine.RegisterWorkerRequest{
		NamespaceID: "ns-monitor",
		ID:          "worker-a",
		Name:        "Worker A",
		PromptTemplate: "Task {task_id} in {worktree_path} on {branch}",
	})
	require.NoError(t, err)
	_, err = eng.CreateDAG(context.Background(), engine.CreateDAGRequest{
		NamespaceID: "ns-monitor",
		ID:          "dag-1",
		Title:       "DAG 1",
		ExecutionBranch:      "feat/test",
	})
	require.NoError(t, err)
	_, err = eng.CreateTask(context.Background(), engine.CreateTaskRequest{
		NamespaceID:    "ns-monitor",
		ID:             "T1",
		Title:          "task 1",
		AssignedWorker: "worker-a",
		DAGID:          "dag-1",
	})
	require.NoError(t, err)

	_, err = srv.handleLeaderTick(context.Background(), map[string]any{"namespace_id": "ns-monitor"})
	require.NoError(t, err)

	result, err := srv.handleLeaderTick(context.Background(), map[string]any{"namespace_id": "ns-monitor"})
	require.NoError(t, err)
	require.Equal(t, "success", result["tree_status"])
	require.Equal(t, "execute", result["phase"])

	task, err := eng.GetTask(context.Background(), "ns-monitor", "T1")
	require.NoError(t, err)
	require.Equal(t, engine.TaskExecuting, task.State)

	bridge := btBridgeForRequest(srv)
	require.NotNil(t, bridge)
	payload, err := bridge.RPC("tick", map[string]any{
		"tree_name": "leader-default",
		"blackboard": map[string]any{
			"namespace_id": "ns-monitor",
		},
		"options": map[string]any{"return_blackboard": true},
	})
	require.NoError(t, err)
	bb := decodeBlackboard(t, payload)
	require.Equal(t, "T1", bb["last_monitored_task_id"])
	require.Equal(t, "executing", bb["last_monitored_state"])
}

func TestLeaderTickReportStuckViaPython(t *testing.T) {
	useBTTestEnv(t)

	dbPath := filepath.Join(t.TempDir(), "stuck.db")
	eng, err := engine.NewEngine(engine.NewEngineConfig{DBPath: dbPath})
	require.NoError(t, err)
	defer eng.Close()

	srv, err := New(eng, Config{})
	require.NoError(t, err)

	_, err = eng.CreateNamespace(context.Background(), engine.CreateNamespaceRequest{
		ID:   "ns-stuck",
		Name: "stuck-ns",
	})
	require.NoError(t, err)
	_, err = eng.RegisterWorker(context.Background(), engine.RegisterWorkerRequest{
		NamespaceID: "ns-stuck",
		ID:          "worker-a",
		Name:        "Worker A",
		PromptTemplate: "Task {task_id} in {worktree_path} on {branch}",
	})
	require.NoError(t, err)
	_, err = eng.CreateDAG(context.Background(), engine.CreateDAGRequest{
		NamespaceID: "ns-stuck",
		ID:          "dag-1",
		Title:       "DAG 1",
		ExecutionBranch:      "feat/test",
	})
	require.NoError(t, err)
	_, err = eng.CreateTask(context.Background(), engine.CreateTaskRequest{
		NamespaceID:    "ns-stuck",
		ID:             "T1",
		Title:          "task 1",
		AssignedWorker: "worker-a",
		DAGID:          "dag-1",
	})
	require.NoError(t, err)
	_, err = srv.dispatchTaskOnce(context.Background(), "ns-stuck", "T1")
	require.NoError(t, err)
	_, err = eng.TransitionTask(context.Background(), "ns-stuck", "T1", engine.TransSubmit, map[string]string{"actor_role": "worker"})
	require.NoError(t, err)
	_, err = eng.TransitionTask(context.Background(), "ns-stuck", "T1", engine.TransRework, map[string]string{"actor_role": "reviewer"})
	require.NoError(t, err)

	result, err := srv.handleLeaderTick(context.Background(), map[string]any{"namespace_id": "ns-stuck"})
	require.NoError(t, err)
	require.Equal(t, "failure", result["tree_status"])
	require.Equal(t, "execute", result["phase"])
	require.Equal(t, []map[string]any{{"assigned_worker": "worker-a", "task_id": "T1", "title": "task 1"}}, result["next_tasks"])

	bridge := btBridgeForRequest(srv)
	require.NotNil(t, bridge)
	payload, err := bridge.RPC("tick", map[string]any{
		"tree_name": "leader-default",
		"blackboard": map[string]any{
			"namespace_id": "ns-stuck",
		},
		"options": map[string]any{"return_blackboard": true},
	})
	require.NoError(t, err)
	bb := decodeBlackboard(t, payload)
	require.Equal(t, "execute", bb["phase"])
	require.Equal(t, true, bb["has_next_tasks"])
	require.Equal(t, false, bb["has_stuck_tasks"])
}

func TestLifecycleEndToEndViaPython(t *testing.T) {
	useBTTestEnv(t)

	dbPath := filepath.Join(t.TempDir(), "lifecycle.db")
	repoPath := initTestGitRepo(t)
	eng, err := engine.NewEngine(engine.NewEngineConfig{DBPath: dbPath})
	require.NoError(t, err)
	defer eng.Close()

	srv, err := New(eng, Config{})
	require.NoError(t, err)

	_, err = eng.CreateNamespace(context.Background(), engine.CreateNamespaceRequest{
		ID:   "ns-lifecycle",
		Name: "lifecycle-ns",
		Metadata: map[string]string{
			"workdir": repoPath,
		},
	})
	require.NoError(t, err)
	_, err = eng.RegisterWorker(context.Background(), engine.RegisterWorkerRequest{
		NamespaceID: "ns-lifecycle",
		ID:          "worker-a",
		Name:        "Worker A",
		PromptTemplate: "Task {task_id} in {worktree_path} on {branch}",
	})
	require.NoError(t, err)
	_, err = eng.RegisterWorker(context.Background(), engine.RegisterWorkerRequest{
		NamespaceID: "ns-lifecycle",
		ID:          "reviewer-a",
		Name:        "Reviewer A",
		PromptTemplate: "rev_commit={review_commit} rev_diff={review.diff} task={task_id}",
	})
	require.NoError(t, err)
	_, err = eng.CreateDAG(context.Background(), engine.CreateDAGRequest{
		NamespaceID: "ns-lifecycle",
		ID:          "dag-1",
		Title:       "DAG 1",
		ExecutionBranch:      "feat/test",
	})
	require.NoError(t, err)
	_, err = eng.CreateTask(context.Background(), engine.CreateTaskRequest{
		NamespaceID:    "ns-lifecycle",
		ID:             "T1",
		Title:          "task 1",
		AssignedWorker: "worker-a",
		DAGID:          "dag-1",
	})
	require.NoError(t, err)

	leaderResult, err := srv.handleLeaderTick(context.Background(), map[string]any{"namespace_id": "ns-lifecycle"})
	require.NoError(t, err)
	require.Equal(t, "execute", leaderResult["phase"])

	task, err := eng.GetTask(context.Background(), "ns-lifecycle", "T1")
	require.NoError(t, err)
	require.Equal(t, engine.TaskExecuting, task.State)

	bridge := btBridgeForRequest(srv)
	require.NotNil(t, bridge)
	workerPayload, err := bridge.RPC("tick", map[string]any{
		"tree_name": "worker-default",
		"blackboard": map[string]any{
			"namespace_id":        "ns-lifecycle",
			"task_id":             "T1",
			"worker_id":           "worker-a",
			"doc_record_content":  "implemented task 1",
			"doc_record_title":    "task 1",
			"diary_entry_content": "done",
		},
		"options": map[string]any{"return_blackboard": true},
	})
	require.NoError(t, err)
	require.Equal(t, "success", workerPayload["status"])
	workerBB := decodeBlackboard(t, workerPayload)
	require.Equal(t, true, workerBB["submitted_for_review"])
	require.Equal(t, "review_pending", workerBB["submitted_state"])

	task, err = eng.GetTask(context.Background(), "ns-lifecycle", "T1")
	require.NoError(t, err)
	require.Equal(t, engine.TaskReviewPending, task.State)

	reviewerPayload, err := bridge.RPC("tick", map[string]any{
		"tree_name": "reviewer-default",
		"blackboard": map[string]any{
			"namespace_id":          "ns-lifecycle",
			"task_id":               "T1",
			"worker_id":             "reviewer-a",
			"review_decision_input": "approve",
		},
		"options": map[string]any{"return_blackboard": true},
	})
	require.NoError(t, err)
	require.Equal(t, "success", reviewerPayload["status"])
	reviewerBB := decodeBlackboard(t, reviewerPayload)
	require.Equal(t, "pass", reviewerBB["review_decision"])
	require.Equal(t, "done", reviewerBB["review_decision_state"])

	task, err = eng.GetTask(context.Background(), "ns-lifecycle", "T1")
	require.NoError(t, err)
	require.Equal(t, engine.TaskDone, task.State)

	docs, err := eng.ListProjectDocs(context.Background(), "ns-lifecycle")
	require.NoError(t, err)
	require.Len(t, docs, 1)
	require.Equal(t, "implemented task 1", docs[0].Content)

	diary, err := eng.GetWorkerDiary(context.Background(), "ns-lifecycle", "worker-a", time.Now().UTC().Format("2006-01-02"))
	require.NoError(t, err)
	require.Contains(t, diary.Content, "done")

	finalLeaderResult, err := srv.handleLeaderTick(context.Background(), map[string]any{"namespace_id": "ns-lifecycle"})
	require.NoError(t, err)
	require.Equal(t, "done", finalLeaderResult["phase"])
	require.Equal(t, "success", finalLeaderResult["tree_status"])

}

func TestLeaderTickReportDoneViaPython(t *testing.T) {
	useBTTestEnv(t)

	dbPath := filepath.Join(t.TempDir(), "done.db")
	eng, err := engine.NewEngine(engine.NewEngineConfig{DBPath: dbPath})
	require.NoError(t, err)
	defer eng.Close()

	srv, err := New(eng, Config{})
	require.NoError(t, err)

	_, err = eng.CreateNamespace(context.Background(), engine.CreateNamespaceRequest{ID: "ns-done", Name: "done-ns"})
	require.NoError(t, err)
	_, err = eng.RegisterWorker(context.Background(), engine.RegisterWorkerRequest{NamespaceID: "ns-done", ID: "worker-a", Name: "Worker A"})
	require.NoError(t, err)
	_, err = eng.CreateDAG(context.Background(), engine.CreateDAGRequest{NamespaceID: "ns-done", ID: "dag-1", Title: "DAG 1", ExecutionBranch: "feat/test"})
	require.NoError(t, err)
	_, err = eng.CreateTask(context.Background(), engine.CreateTaskRequest{NamespaceID: "ns-done", ID: "T1", Title: "task 1", AssignedWorker: "worker-a", DAGID: "dag-1"})
	require.NoError(t, err)
	_, err = eng.TransitionTask(context.Background(), "ns-done", "T1", engine.TransStart, map[string]string{"actor_role": "leader"})
	require.NoError(t, err)
	_, err = eng.TransitionTask(context.Background(), "ns-done", "T1", engine.TransSubmit, map[string]string{"actor_role": "worker"})
	require.NoError(t, err)
	_, err = eng.TransitionTask(context.Background(), "ns-done", "T1", engine.TransPass, map[string]string{"actor_role": "reviewer"})
	require.NoError(t, err)

	result, err := srv.handleLeaderTick(context.Background(), map[string]any{"namespace_id": "ns-done"})
	require.NoError(t, err)
	require.Equal(t, "success", result["tree_status"])
	require.Equal(t, "done", result["phase"])

	bridge := btBridgeForRequest(srv)
	require.NotNil(t, bridge)
	payload, err := bridge.RPC("tick", map[string]any{
		"tree_name":  "leader-default",
		"blackboard": map[string]any{"namespace_id": "ns-done"},
		"options":    map[string]any{"return_blackboard": true},
	})
	require.NoError(t, err)
	bb := decodeBlackboard(t, payload)
	require.Equal(t, "done", bb["last_done_phase"])
	require.Equal(t, 1.0, bb["last_done_completed_tasks"])
	require.Equal(t, 1.0, bb["last_done_total_tasks"])
	require.Equal(t, 100.0, bb["last_done_completion_pct"])
	require.Equal(t, []any{"goal", "doc_list"}, bb["last_done_suggested_actions"])

}
