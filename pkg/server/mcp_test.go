package server

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/toustifer/agentflow/pkg/engine"
)

func TestToolRegistryIncludesCoreTools(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	tools := srv.Tools()
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}

	require.Contains(t, names, "namespace_create")
	require.Contains(t, names, "namespace_list")
	require.Contains(t, names, "task_create")
	require.Contains(t, names, "task_transition")
	require.Contains(t, names, "task_get")
	require.Contains(t, names, "task_list")
	require.Contains(t, names, "task_history")
	require.Contains(t, names, "flow_ping")
}

func TestTaskCreateHandlerCallsEngine(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)

	result, err := srv.Handle(context.Background(), "task_create", map[string]any{
		"namespace_id":    "ns-1",
		"task_id":         "T1",
		"title":           "bootstrap",
		"assigned_worker": "worker-ops",
	})
	require.NoError(t, err)
	require.Equal(t, "T1", result["id"])

	task, err := srv.engine.GetTask(context.Background(), "ns-1", "T1")
	require.NoError(t, err)
	require.Equal(t, "ns-1", task.NamespaceID)
	require.Equal(t, "bootstrap", task.Title)
	require.Equal(t, "worker-ops", task.AssignedWorker)
}

func TestTaskGetReturnsEngineTask(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	_, err := srv.engine.CreateTask(context.Background(), engine.CreateTaskRequest{
		NamespaceID:    "ns-1",
		ID:             "T-get",
		Title:          "inspect",
		AssignedWorker: "worker-a",
	})
	require.NoError(t, err)

	result, err := srv.Handle(context.Background(), "task_get", map[string]any{
		"namespace_id": "ns-1",
		"task_id":      "T-get",
	})
	require.NoError(t, err)
	require.Equal(t, "T-get", result["id"])
	require.Equal(t, "ns-1", result["namespace_id"])
	require.Equal(t, "inspect", result["title"])
	require.Equal(t, "assigned", result["state"])
}

func TestTaskTransitionAdvancesState(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	createDagTaskForStart(t, srv, "T-transition")

	result, err := srv.Handle(context.Background(), "task_transition", map[string]any{
		"namespace_id": "ns-1",
		"task_id":      "T-transition",
		"transition":   "start",
		"metadata": map[string]any{
			"actor": "leader",
		},
	})
	require.NoError(t, err)
	require.Equal(t, "executing", result["state"])
	workerLaunch, ok := result["worker_launch"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, true, workerLaunch["required"])
	require.Equal(t, false, workerLaunch["started"])
	require.Equal(t, "launch_worker_manually", workerLaunch["leader_next_action"])
	require.Equal(t, "worker-b", workerLaunch["worker_id"])
	require.Equal(t, "T-transition", workerLaunch["task_id"])
	require.Equal(t, "feat/test", workerLaunch["branch"])
	require.NotEmpty(t, workerLaunch["worktree_path"])
	require.NotEmpty(t, workerLaunch["prompt_template"])

	task, err := srv.engine.GetTask(context.Background(), "ns-1", "T-transition")
	require.NoError(t, err)
	require.Equal(t, engine.TaskExecuting, task.State)
	require.Equal(t, "feat/test", task.Metadata["git.branch"])
	require.NotEmpty(t, task.Metadata["git.worktree_path"])
}

func TestWorkerPromptGetIncludesGitContext(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	_, err := srv.engine.RegisterWorker(context.Background(), engine.RegisterWorkerRequest{
		NamespaceID: "ns-1",
		ID:          "worker-b",
		Name:        "Builder",
		PromptTemplate: "task={task_id} branch={branch} repo={repo_path} worktree={worktree_path} dag={dag_title}",
	})
	require.NoError(t, err)
	createDagTaskForStart(t, srv, "T-prompt")
	_, err = srv.Handle(context.Background(), "task_transition", map[string]any{
		"namespace_id": "ns-1",
		"task_id":      "T-prompt",
		"transition":   "start",
	})
	require.NoError(t, err)

	result, err := srv.Handle(context.Background(), "worker_prompt_get", map[string]any{
		"namespace_id": "ns-1",
		"worker_id":    "worker-b",
		"task_id":      "T-prompt",
	})
	require.NoError(t, err)
	prompt := result["prompt"].(string)
	require.Contains(t, prompt, "branch=feat/test")
	require.Contains(t, prompt, "repo=")
	require.Contains(t, prompt, "worktree=")
	require.Contains(t, prompt, "dag=Test DAG")
}

func TestGitStatusAndWorktreeGet(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	createDagTaskForStart(t, srv, "T-status")
	_, err := srv.Handle(context.Background(), "task_transition", map[string]any{
		"namespace_id": "ns-1",
		"task_id":      "T-status",
		"transition":   "start",
	})
	require.NoError(t, err)

	gitResult, err := srv.Handle(context.Background(), "git_status", map[string]any{
		"namespace_id": "ns-1",
		"task_id":      "T-status",
	})
	require.NoError(t, err)
	require.Equal(t, "main", gitResult["branch"])
	require.NotEmpty(t, gitResult["repo_path"])
	require.NotEmpty(t, gitResult["task_metadata"])
	require.Equal(t, "clean", gitResult["status"])

	worktreeResult, err := srv.Handle(context.Background(), "worktree_get", map[string]any{
		"namespace_id": "ns-1",
		"task_id":      "T-status",
	})
	require.NoError(t, err)
	require.Equal(t, "feat/test", worktreeResult["branch"])
	require.NotEmpty(t, worktreeResult["worktree_path"])
	require.Equal(t, "clean", worktreeResult["status"])
}

func TestFlowPingReturnsSuccess(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)

	result, err := srv.Handle(context.Background(), "flow_ping", map[string]any{})
	require.NoError(t, err)
	require.Equal(t, map[string]any{"ok": true}, result)
}

func TestTaskGetListHistoryReturnCorrectResults(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	_, err := srv.engine.CreateTask(context.Background(), engine.CreateTaskRequest{
		NamespaceID:    "ns-1",
		ID:             "T-list-1",
		Title:          "first",
		AssignedWorker: "w1",
	})
	require.NoError(t, err)
	_, err = srv.engine.CreateTask(context.Background(), engine.CreateTaskRequest{
		NamespaceID:    "ns-1",
		ID:             "T-list-2",
		Title:          "second",
		AssignedWorker: "w2",
	})
	require.NoError(t, err)

	// task_list should return all tasks in the namespace
	listResult, err := srv.Handle(context.Background(), "task_list", map[string]any{
		"namespace_id": "ns-1",
	})
	require.NoError(t, err)
	items, ok := listResult["tasks"].([]any)
	require.True(t, ok)
	require.Len(t, items, 2)

	// task_history should return non-empty history for a created task
	historyResult, err := srv.Handle(context.Background(), "task_history", map[string]any{
		"namespace_id": "ns-1",
		"task_id":      "T-list-1",
	})
	require.NoError(t, err)
	historyItems, ok := historyResult["history"].([]any)
	require.True(t, ok)
	require.GreaterOrEqual(t, len(historyItems), 1)
}

func TestLifecycleTickRunsEndToEnd(t *testing.T) {
	useBTTestEnv(t)

	srv := newTestServer(t)
	_, err := srv.engine.RegisterWorker(context.Background(), engine.RegisterWorkerRequest{
		NamespaceID: "ns-1",
		ID:          "worker-a",
		Name:        "Worker A",
		PromptTemplate: "Task {task_id} in {worktree_path} on {branch}",
	})
	require.NoError(t, err)
	_, err = srv.engine.RegisterWorker(context.Background(), engine.RegisterWorkerRequest{
		NamespaceID: "ns-1",
		ID:          "reviewer-a",
		Name:        "Reviewer A",
		PromptTemplate: "rev_commit={review_commit} rev_diff={review.diff} task={task_id}",
	})
	require.NoError(t, err)
	_, err = srv.engine.CreateDAG(context.Background(), engine.CreateDAGRequest{
		NamespaceID: "ns-1",
		ID:          "dag-1",
		Title:       "Test DAG",
		Branch:      "feat/test",
	})
	require.NoError(t, err)
	_, err = srv.engine.CreateTask(context.Background(), engine.CreateTaskRequest{
		NamespaceID:    "ns-1",
		ID:             "T-life",
		Title:          "lifecycle",
		AssignedWorker: "worker-a",
		DAGID:          "dag-1",
	})
	require.NoError(t, err)

	result, err := srv.Handle(context.Background(), "lifecycle_tick", map[string]any{
		"namespace_id":          "ns-1",
		"task_id":               "T-life",
		"worker_id":             "worker-a",
		"reviewer_id":           "reviewer-a",
		"review_decision_input": "approve",
		"doc_record_content":    "implemented task 1",
		"doc_record_title":      "task 1",
		"diary_entry_content":   "done",
	})
	require.NoError(t, err)

	task := result["task"].(map[string]any)
	require.Equal(t, "done", task["state"])

	leader := result["leader"].(map[string]any)
	finalLeader := leader["final"].(map[string]any)
	require.Equal(t, "done", finalLeader["phase"])

	worker := result["worker"].(map[string]any)
	workerBB := worker["blackboard"].(map[string]any)
	require.Equal(t, true, workerBB["submitted_for_review"])

	reviewer := result["reviewer"].(map[string]any)
	reviewerBB := reviewer["blackboard"].(map[string]any)
	require.Equal(t, "pass", reviewerBB["review_decision"])
	require.Equal(t, "done", reviewerBB["review_decision_state"])

	if globalBTBridge != nil {
		globalBTBridge.Stop()
		globalBTBridge = nil
	}
}

func TestLifecycleTickRejectsUndispatchedTask(t *testing.T) {
	useBTTestEnv(t)

	srv := newTestServer(t)
	_, err := srv.engine.RegisterWorker(context.Background(), engine.RegisterWorkerRequest{
		NamespaceID: "ns-1",
		ID:          "worker-a",
		Name:        "Worker A",
		PromptTemplate: "Task {task_id} in {worktree_path} on {branch}",
	})
	require.NoError(t, err)
	_, err = srv.engine.RegisterWorker(context.Background(), engine.RegisterWorkerRequest{
		NamespaceID: "ns-1",
		ID:          "reviewer-a",
		Name:        "Reviewer A",
		PromptTemplate: "rev_commit={review_commit} rev_diff={review.diff} task={task_id}",
	})
	require.NoError(t, err)
	_, err = srv.engine.CreateDAG(context.Background(), engine.CreateDAGRequest{
		NamespaceID: "ns-1",
		ID:          "dag-1",
		Title:       "Test DAG",
		Branch:      "feat/test",
	})
	require.NoError(t, err)
	_, err = srv.engine.CreateTask(context.Background(), engine.CreateTaskRequest{
		NamespaceID:    "ns-1",
		ID:             "T-other",
		Title:          "other task",
		AssignedWorker: "worker-a",
		DAGID:          "dag-1",
	})
	require.NoError(t, err)
	_, err = srv.engine.CreateTask(context.Background(), engine.CreateTaskRequest{
		NamespaceID:    "ns-1",
		ID:             "T-life",
		Title:          "lifecycle",
		AssignedWorker: "worker-a",
		DAGID:          "dag-1",
		DependsOn:      []string{"T-other"},
	})
	require.NoError(t, err)

	_, err = srv.Handle(context.Background(), "lifecycle_tick", map[string]any{
		"namespace_id":          "ns-1",
		"task_id":               "T-life",
		"worker_id":             "worker-a",
		"reviewer_id":           "reviewer-a",
		"review_decision_input": "approve",
		"doc_record_content":    "implemented task 1",
		"doc_record_title":      "task 1",
		"diary_entry_content":   "done",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "was not dispatched")

	if globalBTBridge != nil {
		globalBTBridge.Stop()
		globalBTBridge = nil
	}
}

func TestLifecycleTickRejectsMissingPromptTemplate(t *testing.T) {
	useBTTestEnv(t)

	srv := newTestServer(t)
	_, err := srv.engine.RegisterWorker(context.Background(), engine.RegisterWorkerRequest{
		NamespaceID:     "ns-1",
		ID:              "worker-a",
		Name:            "Worker A",
		PromptTemplate:  "Task {task_id} in {worktree_path} on {branch}",
	})
	require.NoError(t, err)
	_, err = srv.engine.RegisterWorker(context.Background(), engine.RegisterWorkerRequest{
		NamespaceID: "ns-1",
		ID:          "reviewer-a",
		Name:        "Reviewer A",
		PromptTemplate: "rev_commit={review_commit} rev_diff={review.diff} task={task_id}",
	})
	require.NoError(t, err)
	_, err = srv.engine.CreateDAG(context.Background(), engine.CreateDAGRequest{
		NamespaceID: "ns-1",
		ID:          "dag-1",
		Title:       "Test DAG",
		Branch:      "feat/test",
	})
	require.NoError(t, err)
	_, err = srv.engine.CreateTask(context.Background(), engine.CreateTaskRequest{
		NamespaceID:    "ns-1",
		ID:             "T-life-missing-prompt",
		Title:          "lifecycle",
		AssignedWorker: "worker-a",
		DAGID:          "dag-1",
	})
	require.NoError(t, err)

	_, err = srv.Handle(context.Background(), "lifecycle_tick", map[string]any{
		"namespace_id":          "ns-1",
		"task_id":               "T-life-missing-prompt",
		"worker_id":             "worker-a",
		"reviewer_id":           "reviewer-a",
		"review_decision_input": "approve",
		"doc_record_content":    "implemented task 1",
		"doc_record_title":      "task 1",
		"diary_entry_content":   "done",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "worker \"worker-a\" prompt preflight failed")
	require.Contains(t, err.Error(), "worker has no prompt template configured")

	if globalBTBridge != nil {
		globalBTBridge.Stop()
		globalBTBridge = nil
	}
}

func TestHubSyncFailureDoesNotFailFlowPing(t *testing.T) {
	t.Parallel()

	srv := newTestServerWithConfig(t, Config{HubEnabled: true})
	srv.hub = failingHubSyncer{err: errors.New("hub unavailable")}

	result, err := srv.Handle(context.Background(), "flow_ping", map[string]any{})
	require.NoError(t, err)
	require.Equal(t, map[string]any{"ok": true}, result)
}

func TestHubSyncFailureDoesNotFailNamespaceCreate(t *testing.T) {
	t.Parallel()

	srv := newTestServerWithConfig(t, Config{HubEnabled: true})
	srv.hub = failingHubSyncer{err: errors.New("hub unavailable")}

	result, err := srv.Handle(context.Background(), "namespace_create", map[string]any{
		"id":   "ns-2",
		"name": "namespace-sync-fallback",
	})
	require.NoError(t, err)
	require.Equal(t, "ns-2", result["id"])

	ns, err := srv.engine.GetNamespace(context.Background(), "ns-2")
	require.NoError(t, err)
	require.Equal(t, "namespace-sync-fallback", ns.Name)
}

func TestHubSyncFailureDoesNotFailTaskCreateAndTransition(t *testing.T) {
	t.Parallel()

	srv := newTestServerWithConfig(t, Config{HubEnabled: true})
	srv.hub = failingHubSyncer{err: errors.New("hub unavailable")}
	createDagTaskForStart(t, srv, "T-sync")

	created, err := srv.Handle(context.Background(), "task_get", map[string]any{
		"namespace_id": "ns-1",
		"task_id":      "T-sync",
	})
	require.NoError(t, err)
	require.Equal(t, "T-sync", created["id"])

	transitioned, err := srv.Handle(context.Background(), "task_transition", map[string]any{
		"namespace_id": "ns-1",
		"task_id":      "T-sync",
		"transition":   "start",
	})
	require.NoError(t, err)
	require.Equal(t, "executing", transitioned["state"])
}

func TestHubSyncFailureDoesNotFailTaskGet(t *testing.T) {
	t.Parallel()

	srv := newTestServerWithConfig(t, Config{HubEnabled: true})
	srv.hub = failingHubSyncer{err: errors.New("hub unavailable")}

	_, err := srv.engine.CreateTask(context.Background(), engine.CreateTaskRequest{
		NamespaceID:    "ns-1",
		ID:             "T-get-fail",
		Title:          "get with sync failure",
		AssignedWorker: "w",
	})
	require.NoError(t, err)

	result, err := srv.Handle(context.Background(), "task_get", map[string]any{
		"namespace_id": "ns-1",
		"task_id":      "T-get-fail",
	})
	require.NoError(t, err)
	require.Equal(t, "T-get-fail", result["id"])
	require.Equal(t, "get with sync failure", result["title"])
}

func TestHubSyncFailureDoesNotFailTaskList(t *testing.T) {
	t.Parallel()

	srv := newTestServerWithConfig(t, Config{HubEnabled: true})
	srv.hub = failingHubSyncer{err: errors.New("hub unavailable")}

	_, err := srv.engine.CreateTask(context.Background(), engine.CreateTaskRequest{
		NamespaceID:    "ns-1",
		ID:             "T-list-fail-1",
		Title:          "list sync fallback",
		AssignedWorker: "w",
	})
	require.NoError(t, err)

	result, err := srv.Handle(context.Background(), "task_list", map[string]any{
		"namespace_id": "ns-1",
	})
	require.NoError(t, err)
	items, ok := result["tasks"].([]any)
	require.True(t, ok)
	require.GreaterOrEqual(t, len(items), 1)
}

func TestHubSyncFailureDoesNotFailTaskHistory(t *testing.T) {
	t.Parallel()

	srv := newTestServerWithConfig(t, Config{HubEnabled: true})
	srv.hub = failingHubSyncer{err: errors.New("hub unavailable")}

	_, err := srv.engine.CreateTask(context.Background(), engine.CreateTaskRequest{
		NamespaceID:    "ns-1",
		ID:             "T-hist-fail",
		Title:          "history with sync failure",
		AssignedWorker: "w",
	})
	require.NoError(t, err)

	result, err := srv.Handle(context.Background(), "task_history", map[string]any{
		"namespace_id": "ns-1",
		"task_id":      "T-hist-fail",
	})
	require.NoError(t, err)
	items, ok := result["history"].([]any)
	require.True(t, ok)
	require.GreaterOrEqual(t, len(items), 1)
}

func TestTaskGetCallsSyncWithCorrectTask(t *testing.T) {
	t.Parallel()

	srv := newTestServerWithConfig(t, Config{HubEnabled: true})
	tracker := &trackingHubSyncer{}
	srv.hub = tracker

	_, err := srv.engine.CreateTask(context.Background(), engine.CreateTaskRequest{
		NamespaceID:    "ns-1",
		ID:             "T-sync-check",
		Title:          "sync verification",
		AssignedWorker: "w",
	})
	require.NoError(t, err)

	result, err := srv.Handle(context.Background(), "task_get", map[string]any{
		"namespace_id": "ns-1",
		"task_id":      "T-sync-check",
	})
	require.NoError(t, err)
	require.Equal(t, "T-sync-check", result["id"])

	// verify SyncTask was called with the correct task ID
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	require.Contains(t, tracker.syncedTasks, "T-sync-check")
}

func TestTaskListCallsSyncForEachTask(t *testing.T) {
	t.Parallel()

	srv := newTestServerWithConfig(t, Config{HubEnabled: true})
	tracker := &trackingHubSyncer{}
	srv.hub = tracker

	_, err := srv.engine.CreateTask(context.Background(), engine.CreateTaskRequest{
		NamespaceID:    "ns-1",
		ID:             "T-list-sync-1",
		Title:          "list sync A",
		AssignedWorker: "w",
	})
	require.NoError(t, err)
	_, err = srv.engine.CreateTask(context.Background(), engine.CreateTaskRequest{
		NamespaceID:    "ns-1",
		ID:             "T-list-sync-2",
		Title:          "list sync B",
		AssignedWorker: "w",
	})
	require.NoError(t, err)

	result, err := srv.Handle(context.Background(), "task_list", map[string]any{
		"namespace_id": "ns-1",
	})
	require.NoError(t, err)
	items, ok := result["tasks"].([]any)
	require.True(t, ok)
	require.Len(t, items, 2)

	// verify SyncTask was called for each task
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	require.Contains(t, tracker.syncedTasks, "T-list-sync-1")
	require.Contains(t, tracker.syncedTasks, "T-list-sync-2")
}

func TestEnsureReadyGitRepoFresh(t *testing.T) {
	t.Parallel()
	workdir := filepath.Join(t.TempDir(), "fresh")
	require.NoError(t, os.MkdirAll(workdir, 0o755))
	now := time.Now()
	info, branch, err := ensureReadyGitRepo(context.Background(), workdir, "main", true, "Tester", "tester@example.com")
	require.NoError(t, err)
	evalWorkdir, _ := filepath.EvalSymlinks(workdir)
		require.NoError(t, err)
		require.Equal(t, info, evalWorkdir)
	require.Equal(t, "main", branch)
	_, err = os.Stat(filepath.Join(workdir, ".git"))
	require.NoError(t, err)
	if now.After(now.Add(time.Hour)) {
		t.Fatal("time must not stand still")
	}
}

func TestEnsureReadyGitRepoRejects(t *testing.T) {
	t.Parallel()
	workdir := filepath.Join(t.TempDir(), "empty")
	require.NoError(t, os.MkdirAll(workdir, 0o755))
	_, _, err := ensureReadyGitRepo(context.Background(), workdir, "main", false, "", "")
	require.Error(t, err)
}

func TestWriteRulesFile(t *testing.T) {
	t.Parallel()
	target := filepath.Join(t.TempDir(), ".claude")
	_, err := writeRulesFile(target)
	require.NoError(t, err)
	got, err := os.ReadFile(filepath.Join(target, "agentflow-git.md"))
	require.NoError(t, err)
	text := string(got)
	require.Contains(t, text, "# agentflow-git.md")
	require.Contains(t, text, "## Workspace rule")
	require.Contains(t, text, "## Branch binding")
	require.Contains(t, text, "## Standard task flow")
	require.Contains(t, text, "## Forbidden actions")
	require.Contains(t, text, "## Review handoff contract")
	require.Contains(t, text, "## Failure handling")
}

func TestProjectInitFreshProject(t *testing.T) {
	t.Parallel()
	workdir := filepath.Join(t.TempDir(), "trip-fable")
	require.NoError(t, os.MkdirAll(workdir, 0o755))

	srv := buildIsolatedServer(t)
	result, err := srv.Handle(context.Background(), "project_init", map[string]any{
		"project_id": "trip-fable",
		"workdir":    workdir,
	})
	require.NoError(t, err)
	require.Equal(t, "trip-fable", result["namespace_id"])
	evalWorkdir, err := filepath.EvalSymlinks(workdir)
	require.NoError(t, err)
	require.Equal(t, evalWorkdir, result["repo_path"])
	_, err = os.Stat(filepath.Join(workdir, ".git"))
	require.NoError(t, err)
	rulesPath := result["rules_file_path"].(string)
	require.FileExists(t, rulesPath)
	require.Equal(t, false, result["has_head_commit"])
}

func TestProjectNextStepsFreshProjectRequiresInitialCommit(t *testing.T) {
	t.Parallel()
	workdir := filepath.Join(t.TempDir(), "fresh-next-steps")
	require.NoError(t, os.MkdirAll(workdir, 0o755))

	srv := buildIsolatedServer(t)
	_, err := srv.Handle(context.Background(), "project_init", map[string]any{
		"project_id": "fresh-next-steps",
		"workdir":    workdir,
	})
	require.NoError(t, err)

	result, err := srv.Handle(context.Background(), "project_next_steps", map[string]any{
		"namespace_id": "fresh-next-steps",
		"workdir":      workdir,
	})
	require.NoError(t, err)
	require.Equal(t, "setup", result["phase"])
	require.Equal(t, "等待首个 commit", result["phase_name"])
	nextSteps, ok := result["next_steps"].([]string)
	require.True(t, ok)
	require.NotEmpty(t, nextSteps)
	require.Contains(t, nextSteps[1], "首个 git commit")
	actions, ok := result["actions"].([]string)
	require.True(t, ok)
	require.Equal(t, []string{"project_init", "git_status"}, actions)
}

func TestTaskTransitionStartRejectsRepoWithoutInitialCommit(t *testing.T) {
	t.Parallel()

	eng, err := engine.NewEngine(engine.NewEngineConfig{})
	require.NoError(t, err)
	defer eng.Close()

	workdir := filepath.Join(t.TempDir(), "fresh-no-head")
	require.NoError(t, os.MkdirAll(workdir, 0o755))
	runGitTest(t, workdir, "init", "-b", "main")
	runGitTest(t, workdir, "config", "user.name", "Agentflow Test")
	runGitTest(t, workdir, "config", "user.email", "agentflow@example.com")

	_, err = eng.CreateNamespace(context.Background(), engine.CreateNamespaceRequest{
		ID:   "ns-no-head",
		Name: "no-head",
		Metadata: map[string]string{
			"workdir":         workdir,
			"git_main_branch": "main",
		},
	})
	require.NoError(t, err)
	_, err = eng.CreateDAG(context.Background(), engine.CreateDAGRequest{
		NamespaceID: "ns-no-head",
		ID:          "dag-1",
		Title:       "DAG",
		Branch:      "feat/test",
	})
	require.NoError(t, err)
	_, err = eng.CreateTask(context.Background(), engine.CreateTaskRequest{
		NamespaceID:    "ns-no-head",
		ID:             "T1",
		Title:          "task",
		AssignedWorker: "worker-a",
		DAGID:          "dag-1",
	})
	require.NoError(t, err)

	srv, err := New(eng, Config{})
	require.NoError(t, err)
	_, err = srv.Handle(context.Background(), "task_transition", map[string]any{
		"namespace_id": "ns-no-head",
		"task_id":      "T1",
		"transition":   "start",
		"actor_role":   "leader",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "还没有首个 commit")
}

func TestSubmitCapturesReviewCommitAndDiff(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	createDagTaskForStart(t, srv, "T-review")
	_, err := srv.Handle(context.Background(), "task_transition", map[string]any{
		"namespace_id": "ns-1", "task_id": "T-review",
		"transition": "start", "actor_role": "leader",
	})
	require.NoError(t, err)

	wtPath := srv.findLatestWorktreePath(t, "ns-1", "T-review")
	require.NoError(t, os.WriteFile(filepath.Join(wtPath, "note.txt"), []byte("hello\n"), 0o644))
	runGitTest(t, wtPath, "add", "note.txt")
	runGitTest(t, wtPath, "commit", "-m", "task=T-review: add note")
	// Register worker-b so diary write and submit succeed
	_, err = srv.engine.RegisterWorker(context.Background(), engine.RegisterWorkerRequest{
		NamespaceID: "ns-1", ID: "worker-b", Name: "Builder",
	})
	require.NoError(t, err)
	_, err = srv.Handle(context.Background(), "worker_diary_write", map[string]any{
		"namespace_id": "ns-1", "worker_id": "worker-b",
		"date": time.Now().UTC().Format("2006-01-02"),
		"content": "drafted", "task_id": "T-review",
	})
	require.NoError(t, err)

	result, err := srv.Handle(context.Background(), "task_transition", map[string]any{
		"namespace_id": "ns-1", "task_id": "T-review",
		"transition": "submit", "actor_role": "worker",
	})
	if err != nil {
		cur, _ := srv.engine.GetTask(context.Background(), "ns-1", "T-review")
		t.Logf("submit failed; current state=%q history=%+v", cur.State, cur.Metadata)
	}
	require.NoError(t, err)
	require.Equal(t, "review_pending", result["state"])

	task, err := srv.engine.GetTask(context.Background(), "ns-1", "T-review")
	require.NoError(t, err)
	require.NotEmpty(t, task.Metadata["review.commit"])
	require.Contains(t, task.Metadata["review.diff"], "note.txt")
}


func TestTaskTransitionSubmitRejectsDirtyWorktree(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	createDagTaskForStart(t, srv, "T-dirty-submit")
	_, err := srv.engine.RegisterWorker(context.Background(), engine.RegisterWorkerRequest{
		NamespaceID: "ns-1", ID: "worker-b", Name: "Builder",
	})
	require.NoError(t, err)
	_, err = srv.Handle(context.Background(), "task_transition", map[string]any{
		"namespace_id": "ns-1", "task_id": "T-dirty-submit",
		"transition": "start", "actor_role": "leader",
	})
	require.NoError(t, err)

	wtPath := srv.findLatestWorktreePath(t, "ns-1", "T-dirty-submit")
	require.NoError(t, os.WriteFile(filepath.Join(wtPath, "dirty.txt"), []byte("hello\n"), 0o644))
	_, err = srv.Handle(context.Background(), "worker_diary_write", map[string]any{
		"namespace_id": "ns-1", "worker_id": "worker-b",
		"date": time.Now().UTC().Format("2006-01-02"),
		"content": "drafted", "task_id": "T-dirty-submit",
	})
	require.NoError(t, err)

	_, err = srv.Handle(context.Background(), "task_transition", map[string]any{
		"namespace_id": "ns-1", "task_id": "T-dirty-submit",
		"transition": "submit", "actor_role": "worker",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "未提交改动")

	task, err := srv.engine.GetTask(context.Background(), "ns-1", "T-dirty-submit")
	require.NoError(t, err)
	require.Equal(t, engine.TaskExecuting, task.State)
}

func TestTaskTransitionSubmitMissingDiaryKeepsExecutingState(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	createDagTaskForStart(t, srv, "T-missing-diary")
	_, err := srv.engine.RegisterWorker(context.Background(), engine.RegisterWorkerRequest{
		NamespaceID: "ns-1", ID: "worker-b", Name: "Builder",
	})
	require.NoError(t, err)
	_, err = srv.Handle(context.Background(), "task_transition", map[string]any{
		"namespace_id": "ns-1", "task_id": "T-missing-diary",
		"transition": "start", "actor_role": "leader",
	})
	require.NoError(t, err)

	_, err = srv.Handle(context.Background(), "task_transition", map[string]any{
		"namespace_id": "ns-1", "task_id": "T-missing-diary",
		"transition": "submit", "actor_role": "worker",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "未写")

	task, err := srv.engine.GetTask(context.Background(), "ns-1", "T-missing-diary")
	require.NoError(t, err)
	require.Equal(t, engine.TaskExecuting, task.State)
}


func TestWorkerPromptGetReviewerMode(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	_, err := srv.engine.RegisterWorker(context.Background(), engine.RegisterWorkerRequest{
		NamespaceID: "ns-1", ID: "reviewer-a",
		Name: "Reviewer",
		PromptTemplate: "rev_commit={review_commit} rev_diff={review.diff} worker={assigned_worker} task={task_id}",
	})
	require.NoError(t, err)
	createDagTaskForStart(t, srv, "T-prompt-rev")
	_, err = srv.Handle(context.Background(), "task_transition", map[string]any{
		"namespace_id": "ns-1", "task_id": "T-prompt-rev",
		"transition": "start",
	})
	require.NoError(t, err)
	_, err = srv.engine.UpdateTask(context.Background(), "ns-1", "T-prompt-rev", engine.UpdateTaskRequest{
		State: engine.TaskReviewPending,
		ReviewMetadata: map[string]string{
			"review.commit": "abcdef",
			"review.diff":   "diff --git a/x b/x",
		},
	})
	require.NoError(t, err)

	result, err := srv.Handle(context.Background(), "worker_prompt_get", map[string]any{
		"namespace_id": "ns-1", "worker_id": "reviewer-a",
		"task_id": "T-prompt-rev", "as_reviewer": true,
	})
	require.NoError(t, err)
	prompt := result["prompt"].(string)
	require.Contains(t, prompt, "rev_commit=abcdef")
	require.Contains(t, prompt, "rev_diff=diff --git")
	require.Contains(t, prompt, "task=T-prompt-rev")
}


func TestLeaderTickReturnsPhase(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	result, err := srv.Handle(context.Background(), "leader_tick", map[string]any{
		"namespace_id": "ns-1",
	})
	require.NoError(t, err)
	require.Contains(t, []string{"setup", "shape", "plan", "execute", "stuck", "done"}, result["phase"])
	require.Contains(t, []string{"running", "success", "failure"}, result["tree_status"])
	require.NotEmpty(t, result["progress"])
}



func (s *Server) findLatestWorktreePath(t *testing.T, nsID, taskID string) string {
	t.Helper()
	task, err := s.engine.GetTask(context.Background(), nsID, taskID)
	require.NoError(t, err)
	return task.Metadata["git.worktree_path"]
}

func buildIsolatedServer(t *testing.T) *Server {
	t.Helper()
	eng, err := engine.NewEngine(engine.NewEngineConfig{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, eng.Close()) })
	srv, err := New(eng, Config{})
	require.NoError(t, err)
	return srv
}

func newTestServer(t *testing.T) *Server {
	t.Helper()
	return newTestServerWithConfig(t, Config{})
}

func newTestServerWithConfig(t *testing.T, cfg Config) *Server {
	t.Helper()

	eng, err := engine.NewEngine(engine.NewEngineConfig{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, eng.Close()) })

	repoPath := initTestGitRepo(t)
	_, err = eng.CreateNamespace(context.Background(), engine.CreateNamespaceRequest{
		ID:   "ns-1",
		Name: "agent-company",
		Metadata: map[string]string{
			"workdir": repoPath,
		},
	})
	require.NoError(t, err)

	srv, err := New(eng, cfg)
	require.NoError(t, err)
	return srv
}

func initTestGitRepo(t *testing.T) string {
	t.Helper()
	repoPath := filepath.Join(t.TempDir(), "repo")
	require.NoError(t, os.MkdirAll(repoPath, 0o755))
	runGitTest(t, repoPath, "init", "-b", "main")
	runGitTest(t, repoPath, "config", "user.name", "Agentflow Test")
	runGitTest(t, repoPath, "config", "user.email", "agentflow@example.com")
	readmePath := filepath.Join(repoPath, "README.md")
	require.NoError(t, os.WriteFile(readmePath, []byte("# test\n"), 0o644))
	runGitTest(t, repoPath, "add", "README.md")
	runGitTest(t, repoPath, "commit", "-m", "init")
	return repoPath
}

func createDagTaskForStart(t *testing.T, srv *Server, taskID string) {
	t.Helper()
	var err error
	if _, err = srv.engine.GetWorker(context.Background(), "ns-1", "worker-b"); err != nil {
		_, err = srv.engine.RegisterWorker(context.Background(), engine.RegisterWorkerRequest{
			NamespaceID:    "ns-1",
			ID:             "worker-b",
			Name:           "Worker B",
			PromptTemplate: "Task {task_id} in {worktree_path} on {branch}",
		})
		require.NoError(t, err)
	}
	_, err = srv.engine.CreateDAG(context.Background(), engine.CreateDAGRequest{
		NamespaceID: "ns-1",
		ID:          "dag-1",
		Title:       "Test DAG",
		Branch:      "feat/test",
	})
	require.NoError(t, err)
	_, err = srv.engine.CreateTask(context.Background(), engine.CreateTaskRequest{
		NamespaceID:    "ns-1",
		ID:             taskID,
		Title:          "execute",
		AssignedWorker: "worker-b",
		DAGID:          "dag-1",
	})
	require.NoError(t, err)
}

func runGitTest(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
}

type failingHubSyncer struct {
	err error
}

func (f failingHubSyncer) SyncTask(context.Context, *engine.Task) error {
	return f.err
}

func (f failingHubSyncer) SyncNamespace(context.Context, *engine.Namespace) error {
	return f.err
}

func (f failingHubSyncer) Ping(context.Context) error {
	return f.err
}

// trackingHubSyncer records synced task IDs for verification in tests.
type trackingHubSyncer struct {
	mu           sync.Mutex
	syncedTasks  []string
}

func (t *trackingHubSyncer) SyncTask(_ context.Context, task *engine.Task) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.syncedTasks = append(t.syncedTasks, task.ID)
	return nil
}

func (t *trackingHubSyncer) SyncNamespace(_ context.Context, ns *engine.Namespace) error {
	return nil
}

func (t *trackingHubSyncer) Ping(_ context.Context) error {
	return nil
}
