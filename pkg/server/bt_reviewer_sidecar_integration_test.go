package server

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/toustifer/agentflow/pkg/engine"
)

func TestReviewerFetchWorkDiffViaPython(t *testing.T) {
	useBTTestEnv(t)

	repoPath := initTestGitRepo(t)
	eng, err := engine.NewEngine(engine.NewEngineConfig{})
	require.NoError(t, err)
	defer eng.Close()

	srv, err := New(eng, Config{})
	require.NoError(t, err)

	_, err = eng.CreateNamespace(context.Background(), engine.CreateNamespaceRequest{ID: "ns-review", Name: "review-ns", Metadata: map[string]string{"workdir": repoPath}})
	require.NoError(t, err)
	_, err = eng.RegisterWorker(context.Background(), engine.RegisterWorkerRequest{NamespaceID: "ns-review", ID: "worker-a", Name: "Worker A", PromptTemplate: "Task {task_id} in {worktree_path} on {branch}"})
	require.NoError(t, err)
	_, err = eng.RegisterWorker(context.Background(), engine.RegisterWorkerRequest{NamespaceID: "ns-review", ID: "reviewer-a", Name: "Reviewer", PromptTemplate: "rev_commit={review_commit} rev_diff={review.diff} task={task_id}"})
	require.NoError(t, err)
	_, err = eng.CreateDAG(context.Background(), engine.CreateDAGRequest{NamespaceID: "ns-review", ID: "dag-1", Title: "DAG 1", ExecutionBranch: "feat/test"})
	require.NoError(t, err)
	_, err = eng.CreateTask(context.Background(), engine.CreateTaskRequest{NamespaceID: "ns-review", ID: "T1", Title: "task 1", AssignedWorker: "worker-a", DAGID: "dag-1"})
	require.NoError(t, err)
	_, err = eng.TransitionTask(context.Background(), "ns-review", "T1", engine.TransStart, map[string]string{"actor_role": "leader"})
	require.NoError(t, err)
	_, err = srv.enterWorktreeOnce(context.Background(), "ns-review", "T1", "worker-a")
	require.NoError(t, err)
	_, err = srv.gitCommitChangesOnce(context.Background(), "ns-review", "T1", "worker-a")
	require.NoError(t, err)
	_, err = eng.WriteWorkerDiary(context.Background(), engine.WriteDiaryRequest{NamespaceID: "ns-review", WorkerID: "worker-a", Date: time.Now().UTC().Format("2006-01-02"), Content: "done"})
	require.NoError(t, err)
	_, err = srv.taskSubmitForReviewOnce(context.Background(), "ns-review", "T1", "worker-a")
	require.NoError(t, err)

	bridge := btBridgeForRequest(srv)
	if bridge == nil {
		t.Skip("bt_service Python sidecar not available (import/start failed)")
	}
	miniTree := map[string]any{
		"name":       "review-fetch-diff-mini",
		"blackboard": map[string]any{},
		"tree": map[string]any{
			"type": "Sequence",
			"children": []any{
				map[string]any{"type": "Action", "properties": map[string]any{"fn": "fetch_work_diff"}},
				map[string]any{"type": "Action", "properties": map[string]any{"fn": "review_decide"}},
				map[string]any{"type": "Action", "properties": map[string]any{"fn": "task_review_pass"}},
			},
		},
	}
	payload, err := bridge.RPC("tick", map[string]any{
		"tree_json":  miniTree,
		"blackboard": map[string]any{"namespace_id": "ns-review", "task_id": "T1", "worker_id": "reviewer-a", "review_decision_input": "approve"},
		"options":    map[string]any{"return_blackboard": true},
	})
	require.NoError(t, err)
	require.Equal(t, "success", payload["status"])
	bb, ok := payload["blackboard"].(map[string]any)
	if !ok {
		var raw map[string]any
		bytes, err := json.Marshal(payload["blackboard"])
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(bytes, &raw))
		bb = raw
	}
	require.Equal(t, true, bb["review_context_ready"])
	require.Equal(t, "review_pending", bb["review_fetch_state"])
	require.NotEmpty(t, bb["review_fetch_commit"])
	require.NotEmpty(t, bb["review_prompt"])
	require.Equal(t, "pass", bb["review_decision"])
	require.Equal(t, "done", bb["review_decision_state"])

	task, err := eng.GetTask(context.Background(), "ns-review", "T1")
	require.NoError(t, err)
	require.Equal(t, engine.TaskDone, task.State)

	if globalBTBridge != nil {
		globalBTBridge.Stop()
		globalBTBridge = nil
	}
}

func TestReviewerDefaultTreeRoutesReworkDecision(t *testing.T) {
	useBTTestEnv(t)

	repoPath := initTestGitRepo(t)
	eng, err := engine.NewEngine(engine.NewEngineConfig{})
	require.NoError(t, err)
	defer eng.Close()

	srv, err := New(eng, Config{})
	require.NoError(t, err)

	_, err = eng.CreateNamespace(context.Background(), engine.CreateNamespaceRequest{ID: "ns-review", Name: "review-ns", Metadata: map[string]string{"workdir": repoPath}})
	require.NoError(t, err)
	_, err = eng.RegisterWorker(context.Background(), engine.RegisterWorkerRequest{NamespaceID: "ns-review", ID: "worker-a", Name: "Worker A", PromptTemplate: "Task {task_id} in {worktree_path} on {branch}"})
	require.NoError(t, err)
	_, err = eng.RegisterWorker(context.Background(), engine.RegisterWorkerRequest{NamespaceID: "ns-review", ID: "reviewer-a", Name: "Reviewer", PromptTemplate: "rev_commit={review_commit} rev_diff={review.diff} task={task_id}"})
	require.NoError(t, err)
	_, err = eng.CreateDAG(context.Background(), engine.CreateDAGRequest{NamespaceID: "ns-review", ID: "dag-1", Title: "DAG 1", ExecutionBranch: "feat/test"})
	require.NoError(t, err)
	_, err = eng.CreateTask(context.Background(), engine.CreateTaskRequest{NamespaceID: "ns-review", ID: "T1", Title: "task 1", AssignedWorker: "worker-a", DAGID: "dag-1"})
	require.NoError(t, err)
	_, err = eng.TransitionTask(context.Background(), "ns-review", "T1", engine.TransStart, map[string]string{"actor_role": "leader"})
	require.NoError(t, err)
	_, err = srv.enterWorktreeOnce(context.Background(), "ns-review", "T1", "worker-a")
	require.NoError(t, err)
	_, err = srv.gitCommitChangesOnce(context.Background(), "ns-review", "T1", "worker-a")
	require.NoError(t, err)
	_, err = eng.WriteWorkerDiary(context.Background(), engine.WriteDiaryRequest{NamespaceID: "ns-review", WorkerID: "worker-a", Date: time.Now().UTC().Format("2006-01-02"), Content: "done"})
	require.NoError(t, err)
	_, err = srv.taskSubmitForReviewOnce(context.Background(), "ns-review", "T1", "worker-a")
	require.NoError(t, err)

	bridge := btBridgeForRequest(srv)
	if bridge == nil {
		t.Skip("bt_service Python sidecar not available (import/start failed)")
	}
	payload, err := bridge.RPC("tick", map[string]any{
		"tree_name":  "reviewer-default",
		"blackboard": map[string]any{"namespace_id": "ns-review", "task_id": "T1", "worker_id": "reviewer-a", "review_decision_input": "rework"},
		"options":    map[string]any{"return_blackboard": true},
	})
	require.NoError(t, err)
	require.Equal(t, "success", payload["status"])
	bb, ok := payload["blackboard"].(map[string]any)
	if !ok {
		var raw map[string]any
		bytes, err := json.Marshal(payload["blackboard"])
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(bytes, &raw))
		bb = raw
	}
	require.Equal(t, false, bb["review_approved"])
	require.Equal(t, "rework", bb["review_decision"])
	require.Equal(t, "rework_needed", bb["review_decision_state"])

	task, err := eng.GetTask(context.Background(), "ns-review", "T1")
	require.NoError(t, err)
	require.Equal(t, engine.TaskReworkNeeded, task.State)

	if globalBTBridge != nil {
		globalBTBridge.Stop()
		globalBTBridge = nil
	}
}
func TestReviewerDefaultTreeDoesNotAutoApproveWithoutDecision(t *testing.T) {
	useBTTestEnv(t)

	repoPath := initTestGitRepo(t)
	eng, err := engine.NewEngine(engine.NewEngineConfig{})
	require.NoError(t, err)
	defer eng.Close()

	srv, err := New(eng, Config{})
	require.NoError(t, err)

	_, err = eng.CreateNamespace(context.Background(), engine.CreateNamespaceRequest{ID: "ns-review", Name: "review-ns", Metadata: map[string]string{"workdir": repoPath}})
	require.NoError(t, err)
	_, err = eng.RegisterWorker(context.Background(), engine.RegisterWorkerRequest{NamespaceID: "ns-review", ID: "worker-a", Name: "Worker A", PromptTemplate: "Task {task_id} in {worktree_path} on {branch}"})
	require.NoError(t, err)
	_, err = eng.RegisterWorker(context.Background(), engine.RegisterWorkerRequest{NamespaceID: "ns-review", ID: "reviewer-a", Name: "Reviewer", PromptTemplate: "rev_commit={review_commit} rev_diff={review.diff} task={task_id}"})
	require.NoError(t, err)
	_, err = eng.CreateDAG(context.Background(), engine.CreateDAGRequest{NamespaceID: "ns-review", ID: "dag-1", Title: "DAG 1", ExecutionBranch: "feat/test"})
	require.NoError(t, err)
	_, err = eng.CreateTask(context.Background(), engine.CreateTaskRequest{NamespaceID: "ns-review", ID: "T1", Title: "task 1", AssignedWorker: "worker-a", DAGID: "dag-1"})
	require.NoError(t, err)
	_, err = eng.TransitionTask(context.Background(), "ns-review", "T1", engine.TransStart, map[string]string{"actor_role": "leader"})
	require.NoError(t, err)
	_, err = srv.enterWorktreeOnce(context.Background(), "ns-review", "T1", "worker-a")
	require.NoError(t, err)
	_, err = srv.gitCommitChangesOnce(context.Background(), "ns-review", "T1", "worker-a")
	require.NoError(t, err)
	_, err = eng.WriteWorkerDiary(context.Background(), engine.WriteDiaryRequest{NamespaceID: "ns-review", WorkerID: "worker-a", Date: time.Now().UTC().Format("2006-01-02"), Content: "done"})
	require.NoError(t, err)
	_, err = srv.taskSubmitForReviewOnce(context.Background(), "ns-review", "T1", "worker-a")
	require.NoError(t, err)

	bridge := btBridgeForRequest(srv)
	if bridge == nil {
		t.Skip("bt_service Python sidecar not available (import/start failed)")
	}
	payload, err := bridge.RPC("tick", map[string]any{
		"tree_name":  "reviewer-default",
		"blackboard": map[string]any{"namespace_id": "ns-review", "task_id": "T1", "worker_id": "reviewer-a"},
		"options":    map[string]any{"return_blackboard": true},
	})
	require.NoError(t, err)
	require.Equal(t, "failure", payload["status"])
	bb, ok := payload["blackboard"].(map[string]any)
	if !ok {
		var raw map[string]any
		bytes, err := json.Marshal(payload["blackboard"])
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(bytes, &raw))
		bb = raw
	}
	require.Equal(t, true, bb["review_context_ready"])
	_, hasApproved := bb["review_approved"]
	require.False(t, hasApproved)
	_, hasDecision := bb["review_decision"]
	require.False(t, hasDecision)

	task, err := eng.GetTask(context.Background(), "ns-review", "T1")
	require.NoError(t, err)
	require.Equal(t, engine.TaskReviewPending, task.State)

	if globalBTBridge != nil {
		globalBTBridge.Stop()
		globalBTBridge = nil
	}
}
