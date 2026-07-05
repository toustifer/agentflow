package server

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/toustifer/agentflow/pkg/engine"
)

func TestWorkerTaskGetConfirmViaPython(t *testing.T) {
	useBTTestEnv(t)

	repoPath := initTestGitRepo(t)
	eng, err := engine.NewEngine(engine.NewEngineConfig{})
	require.NoError(t, err)
	defer eng.Close()

	srv, err := New(eng, Config{})
	require.NoError(t, err)

	_, err = eng.CreateNamespace(context.Background(), engine.CreateNamespaceRequest{ID: "ns-worker", Name: "worker-ns", Metadata: map[string]string{"workdir": repoPath}})
	require.NoError(t, err)
	_, err = eng.RegisterWorker(context.Background(), engine.RegisterWorkerRequest{NamespaceID: "ns-worker", ID: "worker-a", Name: "Worker A", PromptTemplate: "Task {task_id} in {worktree_path} on {branch}"})
	require.NoError(t, err)
	_, err = eng.CreateDAG(context.Background(), engine.CreateDAGRequest{NamespaceID: "ns-worker", ID: "dag-1", Title: "DAG 1", Branch: "feat/test"})
	require.NoError(t, err)
	_, err = eng.CreateTask(context.Background(), engine.CreateTaskRequest{NamespaceID: "ns-worker", ID: "T1", Title: "task 1", AssignedWorker: "worker-a", DAGID: "dag-1"})
	require.NoError(t, err)
	_, err = eng.TransitionTask(context.Background(), "ns-worker", "T1", engine.TransStart, map[string]string{"actor_role": "leader"})
	require.NoError(t, err)

	bridge := btBridgeForRequest(srv)
	require.NotNil(t, bridge)
	miniTree := map[string]any{
		"name":       "worker-task-confirm-mini",
		"blackboard": map[string]any{},
		"tree": map[string]any{
			"type": "Sequence",
			"children": []any{
				map[string]any{"type": "Action", "properties": map[string]any{"fn": "task_get_confirm"}},
				map[string]any{"type": "Action", "properties": map[string]any{"fn": "enter_worktree"}},
				map[string]any{"type": "Action", "properties": map[string]any{"fn": "implement_code"}},
				map[string]any{"type": "Action", "properties": map[string]any{"fn": "git_commit_changes"}},
				map[string]any{"type": "Action", "properties": map[string]any{"fn": "doc_write_record"}},
				map[string]any{"type": "Action", "properties": map[string]any{"fn": "diary_write_entry"}},
				map[string]any{"type": "Action", "properties": map[string]any{"fn": "task_submit_for_review"}},
			},
		},
	}
	payload, err := bridge.RPC("tick", map[string]any{
		"tree_json": miniTree,
		"blackboard": map[string]any{
			"namespace_id":        "ns-worker",
			"task_id":             "T1",
			"worker_id":           "worker-a",
			"doc_record_content":  "implemented task 1",
			"doc_record_title":    "task 1",
			"diary_entry_content": "done",
		},
		"options": map[string]any{"return_blackboard": true},
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
	require.Equal(t, true, bb["task_confirmed"])
	require.Equal(t, true, bb["worktree_ready"])
	require.Equal(t, true, bb["implementation_ready"])
	require.Equal(t, true, bb["review_ready"])
	require.Equal(t, true, bb["doc_recorded"])
	require.Equal(t, true, bb["diary_written"])
	require.Equal(t, true, bb["submitted_for_review"])
	require.Equal(t, "review_pending", bb["submitted_state"])
	require.NotEmpty(t, bb["submitted_review_commit"])
	require.NotEmpty(t, bb["recorded_doc_id"])
	require.Equal(t, "worker-a", bb["diary_worker_id"])

	task, err := eng.GetTask(context.Background(), "ns-worker", "T1")
	require.NoError(t, err)
	require.Equal(t, engine.TaskReviewPending, task.State)
	require.NotEmpty(t, task.Metadata["review.commit"])

	docs, err := eng.ListProjectDocs(context.Background(), "ns-worker")
	require.NoError(t, err)
	require.Len(t, docs, 1)
	require.Equal(t, "implemented task 1", docs[0].Content)

	diary, err := eng.GetWorkerDiary(context.Background(), "ns-worker", "worker-a", time.Now().UTC().Format("2006-01-02"))
	require.NoError(t, err)
	require.Contains(t, diary.Content, "done")

	if globalBTBridge != nil {
		globalBTBridge.Stop()
		globalBTBridge = nil
	}
}
