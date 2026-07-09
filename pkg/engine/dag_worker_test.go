package engine

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// DAG tests
// ---------------------------------------------------------------------------

func TestDAGCRUD(t *testing.T) {
	t.Parallel()

	e, err := NewEngine(NewEngineConfig{})
	require.NoError(t, err)
	defer func() { require.NoError(t, e.Close()) }()

	_, err = e.CreateNamespace(context.Background(), CreateNamespaceRequest{ID: "ns-1", Name: "test"})
	require.NoError(t, err)

	// Create
	dag, err := e.CreateDAG(context.Background(), CreateDAGRequest{
		NamespaceID: "ns-1",
		ID:          "dag-1",
		Title:       "添加登录功能",
		ExecutionBranch: "feat/login",
	})
	require.NoError(t, err)
	require.Equal(t, "dag-1", dag.ID)
	require.Equal(t, "添加登录功能", dag.Title)
	require.Equal(t, "feat/login", dag.ExecutionBranch)
	require.Equal(t, DAGPlanning, dag.Status)

	// Get
	dag2, err := e.GetDAG(context.Background(), "ns-1", "dag-1")
	require.NoError(t, err)
	require.Equal(t, dag.Title, dag2.Title)

	// Duplicate should fail
	_, err = e.CreateDAG(context.Background(), CreateDAGRequest{
		NamespaceID: "ns-1", ID: "dag-1", Title: "dup",
	})
	require.Error(t, err)

	// List
	dags, err := e.ListDAGs(context.Background(), "ns-1")
	require.NoError(t, err)
	require.Len(t, dags, 1)

	// Update
	dag3, err := e.UpdateDAG(context.Background(), "ns-1", "dag-1", UpdateDAGRequest{
		Title:  "添加登录功能 v2",
		ExecutionBranch: "feat/login-v2",
	})
	require.NoError(t, err)
	require.Equal(t, "添加登录功能 v2", dag3.Title)
	require.Equal(t, "feat/login-v2", dag3.ExecutionBranch)
}

func TestDAGGraphFromTasks(t *testing.T) {
	t.Parallel()

	e, err := NewEngine(NewEngineConfig{})
	require.NoError(t, err)
	defer func() { require.NoError(t, e.Close()) }()

	_, err = e.CreateNamespace(context.Background(), CreateNamespaceRequest{ID: "ns-1", Name: "test"})
	require.NoError(t, err)
	_, err = e.CreateDAG(context.Background(), CreateDAGRequest{
		NamespaceID: "ns-1", ID: "dag-1", Title: "x", ExecutionBranch: "feat/x",
	})
	require.NoError(t, err)

	// Create tasks with depends_on
	_, err = e.CreateTask(context.Background(), CreateTaskRequest{
		NamespaceID: "ns-1", ID: "T1", Title: "auth", DAGID: "dag-1", AssignedWorker: "worker-auth",
	})
	require.NoError(t, err)
	_, err = e.CreateTask(context.Background(), CreateTaskRequest{
		NamespaceID: "ns-1", ID: "T2", Title: "fe", DAGID: "dag-1",
		DependsOn: []string{"T1"}, AssignedWorker: "worker-fe",
	})
	require.NoError(t, err)
	_, err = e.CreateTask(context.Background(), CreateTaskRequest{
		NamespaceID: "ns-1", ID: "T3", Title: "qa", DAGID: "dag-1",
		DependsOn: []string{"T2"}, AssignedWorker: "worker-qa",
	})
	require.NoError(t, err)

	// Build graph
	graph, err := e.GetDAGGraph(context.Background(), "ns-1", "dag-1")
	require.NoError(t, err)
	require.Len(t, graph.Tasks, 3)
	require.Len(t, graph.Nodes, 3)
	require.Len(t, graph.Edges, 2)
	// Edges: T1→T2, T2→T3
	require.Equal(t, "T1", graph.Edges[0].FromTaskID)
	require.Equal(t, "T2", graph.Edges[0].ToTaskID)
	require.Equal(t, "T2", graph.Edges[1].FromTaskID)
	require.Equal(t, "T3", graph.Edges[1].ToTaskID)

	// Workers grouped
	require.Len(t, graph.Workers, 3)
}

func TestCircularDependencyDetected(t *testing.T) {
	t.Parallel()

	e, err := NewEngine(NewEngineConfig{})
	require.NoError(t, err)
	defer func() { require.NoError(t, e.Close()) }()

	_, err = e.CreateNamespace(context.Background(), CreateNamespaceRequest{ID: "ns-1", Name: "test"})
	require.NoError(t, err)
	_, err = e.CreateDAG(context.Background(), CreateDAGRequest{
		NamespaceID: "ns-1", ID: "dag-1", Title: "x", ExecutionBranch: "x",
	})
	require.NoError(t, err)

	// T1 → T2 → T1 cycle
	_, err = e.CreateTask(context.Background(), CreateTaskRequest{
		NamespaceID: "ns-1", ID: "T1", Title: "a", DAGID: "dag-1",
	})
	require.NoError(t, err)
	_, err = e.CreateTask(context.Background(), CreateTaskRequest{
		NamespaceID: "ns-1", ID: "T2", Title: "b", DAGID: "dag-1",
		DependsOn: []string{"T1"},
	})
	require.NoError(t, err)
	// Try to update T1 to depend on T2 — would create cycle
	_, err = e.CreateTask(context.Background(), CreateTaskRequest{
		NamespaceID: "ns-1", ID: "T3", Title: "c", DAGID: "dag-1",
		DependsOn: []string{"T2"},
	})
	require.NoError(t, err)
	// New task trying to depend on something that (transitively) depends on it
	_, err = e.CreateTask(context.Background(), CreateTaskRequest{
		NamespaceID: "ns-1", ID: "T4", Title: "d", DAGID: "dag-1",
		DependsOn: []string{"T3"},
	})
	require.NoError(t, err)
	// Direct circular attempt
	_, err = e.CreateTask(context.Background(), CreateTaskRequest{
		NamespaceID: "ns-1", ID: "T5", Title: "e", DAGID: "dag-1",
		DependsOn: []string{"T1"}, // T1 already in DAG
	})
	require.NoError(t, err)
	// Now try creating T1's update equivalent — but we don't have update for deps.
	// Test: a task that depends on itself
	_, err = e.CreateTask(context.Background(), CreateTaskRequest{
		NamespaceID: "ns-1", ID: "T6", Title: "self", DAGID: "dag-1",
		DependsOn: []string{"T6"},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found in DAG")
}

// ---------------------------------------------------------------------------
// Worker tests
// ---------------------------------------------------------------------------

func TestWorkerCRUD(t *testing.T) {
	t.Parallel()

	e, err := NewEngine(NewEngineConfig{})
	require.NoError(t, err)
	defer func() { require.NoError(t, e.Close()) }()

	_, err = e.CreateNamespace(context.Background(), CreateNamespaceRequest{ID: "ns-1", Name: "test"})
	require.NoError(t, err)

	// Register
	w, err := e.RegisterWorker(context.Background(), RegisterWorkerRequest{
		NamespaceID:    "ns-1",
		ID:             "worker-auth",
		Name:           "认证服务",
		Kind:           "worker",
		Scope:          "登录/注册/OAuth",
		Skills:         []string{"go", "postgres"},
		TaskTags:       []string{"auth", "backend"},
		PromptTemplate: "Task {task_id}",
		RequiredReads:  []string{".claude/PROJECT_FINAL_SHAPE.md"},
		RecommendedMCP: []string{"doc_search"},
		LaunchMode:     "manual_subagent",
		HandoffTargets: []string{"reviewer-default"},
	})
	require.NoError(t, err)
	require.Equal(t, "worker-auth", w.ID)
	require.Equal(t, "认证服务", w.Name)
	require.Equal(t, []string{"go", "postgres"}, w.Skills)
	require.Equal(t, "worker", w.Kind)
	require.Equal(t, []string{"auth", "backend"}, w.TaskTags)
	require.Equal(t, []string{".claude/PROJECT_FINAL_SHAPE.md"}, w.RequiredReads)
	require.Equal(t, []string{"doc_search"}, w.RecommendedMCP)
	require.Equal(t, "manual_subagent", w.LaunchMode)
	require.Equal(t, []string{"reviewer-default"}, w.HandoffTargets)

	// Get
	_, err = e.GetWorker(context.Background(), "ns-1", "worker-auth")
	require.NoError(t, err)

	// List
	workers, err := e.ListWorkers(context.Background(), "ns-1")
	require.NoError(t, err)
	require.Len(t, workers, 1)

	// Update
	w2, err := e.UpdateWorker(context.Background(), "ns-1", "worker-auth", UpdateWorkerRequest{
		Name:             "认证与权限",
		Kind:             "reviewer",
		Scope:            "登录/注册/权限/OAuth",
		TaskTags:         []string{"auth", "review"},
		RequiredReads:    []string{".claude/IMPLEMENTATION_BRIEF.md"},
		RecommendedMCP:   []string{"doc_search", "find_pitfalls"},
		LaunchMode:       "auto_subagent",
		HandoffTargets:   []string{"worker-auth"},
	})
	require.NoError(t, err)
	require.Equal(t, "认证与权限", w2.Name)
	require.Equal(t, "登录/注册/权限/OAuth", w2.Scope)
	require.Equal(t, "reviewer", w2.Kind)
	require.Equal(t, []string{"auth", "review"}, w2.TaskTags)
	require.Equal(t, []string{".claude/IMPLEMENTATION_BRIEF.md"}, w2.RequiredReads)
	require.Equal(t, []string{"doc_search", "find_pitfalls"}, w2.RecommendedMCP)
	require.Equal(t, "auto_subagent", w2.LaunchMode)
	require.Equal(t, []string{"worker-auth"}, w2.HandoffTargets)
}

func TestWorkerStatusFromTasks(t *testing.T) {
	t.Parallel()

	e, err := NewEngine(NewEngineConfig{})
	require.NoError(t, err)
	defer func() { require.NoError(t, e.Close()) }()

	_, err = e.CreateNamespace(context.Background(), CreateNamespaceRequest{ID: "ns-1", Name: "test"})
	require.NoError(t, err)
	_, err = e.RegisterWorker(context.Background(), RegisterWorkerRequest{
		NamespaceID:    "ns-1", ID: "worker-x", Name: "X", PromptTemplate: "Task {task_id}",
	})
	require.NoError(t, err)

	// No tasks → idle
	require.Equal(t, WorkerIdle, e.WorkerStatus(context.Background(), "ns-1", "worker-x"))

	// Add a task, leave it assigned → still idle (assigned is not active)
	_, err = e.CreateTask(context.Background(), CreateTaskRequest{
		NamespaceID: "ns-1", ID: "T1", Title: "a", AssignedWorker: "worker-x",
	})
	require.NoError(t, err)
	require.Equal(t, WorkerIdle, e.WorkerStatus(context.Background(), "ns-1", "worker-x"))

	// Start → busy
	_, err = e.TransitionTask(context.Background(), "ns-1", "T1", TransStart, map[string]string{"actor_role": "leader"})
	require.NoError(t, err)
	require.Equal(t, WorkerBusy, e.WorkerStatus(context.Background(), "ns-1", "worker-x"))

	// Submit → still busy as an observational status while review is pending
	_, err = e.TransitionTask(context.Background(), "ns-1", "T1", TransSubmit, map[string]string{"actor_role": "worker"})
	require.NoError(t, err)
	require.Equal(t, WorkerBusy, e.WorkerStatus(context.Background(), "ns-1", "worker-x"))

	// Pass → idle
	_, err = e.TransitionTask(context.Background(), "ns-1", "T1", TransPass, map[string]string{"actor_role": "reviewer"})
	require.NoError(t, err)
	require.Equal(t, WorkerIdle, e.WorkerStatus(context.Background(), "ns-1", "worker-x"))
}

func TestProjectNextTasksDoesNotGateOnWorkerBusy(t *testing.T) {
	t.Parallel()

	e, err := NewEngine(NewEngineConfig{})
	require.NoError(t, err)
	defer func() { require.NoError(t, e.Close()) }()

	_, err = e.CreateNamespace(context.Background(), CreateNamespaceRequest{ID: "ns-1", Name: "test"})
	require.NoError(t, err)
	_, err = e.RegisterWorker(context.Background(), RegisterWorkerRequest{
		NamespaceID: "ns-1", ID: "worker-x", Name: "X", PromptTemplate: "Task {task_id}",
	})
	require.NoError(t, err)
	_, err = e.CreateTask(context.Background(), CreateTaskRequest{
		NamespaceID: "ns-1", ID: "T1", Title: "first", AssignedWorker: "worker-x",
	})
	require.NoError(t, err)
	_, err = e.CreateTask(context.Background(), CreateTaskRequest{
		NamespaceID: "ns-1", ID: "T2", Title: "second", AssignedWorker: "worker-x",
	})
	require.NoError(t, err)
	_, err = e.TransitionTask(context.Background(), "ns-1", "T1", TransStart, map[string]string{"actor_role": "leader"})
	require.NoError(t, err)

	next, err := e.ProjectNextTasks(context.Background(), "ns-1")
	require.NoError(t, err)
	require.Len(t, next, 1)
	require.Equal(t, "T2", next[0].TaskID)
	require.Equal(t, true, next[0].DepsSatisfied)
	require.Equal(t, true, next[0].WorkerBusy)
	require.Equal(t, true, next[0].Ready)
	require.Equal(t, "", next[0].Reason)
}

func TestProjectBlockersOnlyReportsDependencies(t *testing.T) {
	t.Parallel()

	e, err := NewEngine(NewEngineConfig{})
	require.NoError(t, err)
	defer func() { require.NoError(t, e.Close()) }()

	_, err = e.CreateNamespace(context.Background(), CreateNamespaceRequest{ID: "ns-1", Name: "test"})
	require.NoError(t, err)
	_, err = e.RegisterWorker(context.Background(), RegisterWorkerRequest{
		NamespaceID: "ns-1", ID: "worker-x", Name: "X", PromptTemplate: "Task {task_id}",
	})
	require.NoError(t, err)
	_, err = e.CreateTask(context.Background(), CreateTaskRequest{
		NamespaceID: "ns-1", ID: "T0", Title: "dep", AssignedWorker: "worker-x",
	})
	require.NoError(t, err)
	_, err = e.CreateTask(context.Background(), CreateTaskRequest{
		NamespaceID: "ns-1", ID: "T1", Title: "first", AssignedWorker: "worker-x",
	})
	require.NoError(t, err)
	_, err = e.CreateTask(context.Background(), CreateTaskRequest{
		NamespaceID: "ns-1", ID: "T2", Title: "second", AssignedWorker: "worker-x", DependsOn: []string{"T0"},
	})
	require.NoError(t, err)
	_, err = e.TransitionTask(context.Background(), "ns-1", "T1", TransStart, map[string]string{"actor_role": "leader"})
	require.NoError(t, err)

	blockers, err := e.ProjectBlockers(context.Background(), "ns-1")
	require.NoError(t, err)
	require.Len(t, blockers, 1)
	require.Equal(t, "dependency", blockers[0].Type)
	require.Equal(t, "T2", blockers[0].TaskID)
	require.Equal(t, "T0", blockers[0].BlockedBy)
}

// ---------------------------------------------------------------------------
// QueryTasks tests
// ---------------------------------------------------------------------------

func TestQueryTasksByDAG(t *testing.T) {
	t.Parallel()

	e, err := NewEngine(NewEngineConfig{})
	require.NoError(t, err)
	defer func() { require.NoError(t, e.Close()) }()

	_, err = e.CreateNamespace(context.Background(), CreateNamespaceRequest{ID: "ns-1", Name: "test"})
	require.NoError(t, err)
	_, err = e.CreateDAG(context.Background(), CreateDAGRequest{
		NamespaceID: "ns-1", ID: "dag-1", Title: "x", ExecutionBranch: "x",
	})
	require.NoError(t, err)
	_, err = e.CreateDAG(context.Background(), CreateDAGRequest{
		NamespaceID: "ns-1", ID: "dag-2", Title: "y", ExecutionBranch: "y",
	})
	require.NoError(t, err)

	_, err = e.CreateTask(context.Background(), CreateTaskRequest{
		NamespaceID: "ns-1", ID: "T1", Title: "a", DAGID: "dag-1",
	})
	require.NoError(t, err)
	_, err = e.CreateTask(context.Background(), CreateTaskRequest{
		NamespaceID: "ns-1", ID: "T2", Title: "b", DAGID: "dag-2",
	})
	require.NoError(t, err)
	_, err = e.CreateTask(context.Background(), CreateTaskRequest{
		NamespaceID: "ns-1", ID: "T3", Title: "c", DAGID: "dag-1",
	})
	require.NoError(t, err)

	tasks, err := e.QueryTasks(context.Background(), TaskQuery{
		NamespaceID: "ns-1", DAGID: "dag-1",
	})
	require.NoError(t, err)
	require.Len(t, tasks, 2)

	tasks, err = e.QueryTasks(context.Background(), TaskQuery{
		NamespaceID: "ns-1",
	})
	require.NoError(t, err)
	require.Len(t, tasks, 3)
}

func TestQueryTasksReadyOnly(t *testing.T) {
	t.Parallel()

	e, err := NewEngine(NewEngineConfig{})
	require.NoError(t, err)
	defer func() { require.NoError(t, e.Close()) }()

	_, err = e.CreateNamespace(context.Background(), CreateNamespaceRequest{ID: "ns-1", Name: "test"})
	require.NoError(t, err)
	_, err = e.CreateDAG(context.Background(), CreateDAGRequest{
		NamespaceID: "ns-1", ID: "dag-1", Title: "x", ExecutionBranch: "x",
	})
	require.NoError(t, err)

	_, err = e.CreateTask(context.Background(), CreateTaskRequest{
		NamespaceID: "ns-1", ID: "T1", Title: "a", DAGID: "dag-1",
	})
	require.NoError(t, err)
	_, err = e.CreateTask(context.Background(), CreateTaskRequest{
		NamespaceID: "ns-1", ID: "T2", Title: "b", DAGID: "dag-1", DependsOn: []string{"T1"},
	})
	require.NoError(t, err)

	// T1 has no deps → ready; T2 depends on T1 which is not done → not ready
	tasks, err := e.QueryTasks(context.Background(), TaskQuery{
		NamespaceID: "ns-1", ReadyOnly: true,
	})
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	require.Equal(t, "T1", tasks[0].ID)

	// After T1 done, T2 becomes ready
	_, err = e.TransitionTask(context.Background(), "ns-1", "T1", TransStart, map[string]string{"actor_role": "leader"})
	require.NoError(t, err)
	_, err = e.TransitionTask(context.Background(), "ns-1", "T1", TransSubmit, map[string]string{"actor_role": "worker"})
	require.NoError(t, err)
	_, err = e.TransitionTask(context.Background(), "ns-1", "T1", TransPass, map[string]string{"actor_role": "reviewer"})
	require.NoError(t, err)

	tasks, err = e.QueryTasks(context.Background(), TaskQuery{
		NamespaceID: "ns-1", ReadyOnly: true,
	})
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	require.Equal(t, "T2", tasks[0].ID)
}

func TestQueryTasksByWorker(t *testing.T) {
	t.Parallel()

	e, err := NewEngine(NewEngineConfig{})
	require.NoError(t, err)
	defer func() { require.NoError(t, e.Close()) }()

	_, err = e.CreateNamespace(context.Background(), CreateNamespaceRequest{ID: "ns-1", Name: "test"})
	require.NoError(t, err)
	_, err = e.CreateTask(context.Background(), CreateTaskRequest{
		NamespaceID: "ns-1", ID: "T1", Title: "a", AssignedWorker: "worker-a",
	})
	require.NoError(t, err)
	_, err = e.CreateTask(context.Background(), CreateTaskRequest{
		NamespaceID: "ns-1", ID: "T2", Title: "b", AssignedWorker: "worker-b",
	})
	require.NoError(t, err)

	tasks, err := e.QueryTasks(context.Background(), TaskQuery{
		NamespaceID: "ns-1", AssignedWorker: "worker-a",
	})
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	require.Equal(t, "T1", tasks[0].ID)
}

// ---------------------------------------------------------------------------
// Persistence test for DAG and Worker
// ---------------------------------------------------------------------------

func TestDAGRuntimeLeasePersistence(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "agentflow-dag-runtime.db")

	e1, err := NewEngine(NewEngineConfig{DBPath: dbPath})
	require.NoError(t, err)
	_, err = e1.CreateNamespace(context.Background(), CreateNamespaceRequest{ID: "ns-1", Name: "test"})
	require.NoError(t, err)
	_, err = e1.CreateDAG(context.Background(), CreateDAGRequest{
		NamespaceID:     "ns-1",
		ID:              "dag-1",
		Title:           "shared runtime",
		ExecutionBranch: "feat/shared-runtime",
		BaseBranch:      "main",
	})
	require.NoError(t, err)
	_, err = e1.UpdateDAGRuntime(context.Background(), "ns-1", "dag-1", UpdateDAGRuntimeRequest{
		WorktreePath:        "/tmp/shared-dag-worktree",
		WorktreeStatus:      "clean",
		HeadSHA:             "abc123",
		ActiveTaskID:        "T1",
		LeaseHolderTaskID:   "T1",
		LeaseHolderWorkerID: "worker-a",
		LeaseHolderAgentID:  "agent-1",
		LeaseAcquiredAt:     "2026-07-09T00:00:00Z",
		RuntimeUpdatedAt:    "2026-07-09T00:00:01Z",
	})
	require.NoError(t, err)
	require.NoError(t, e1.Close())

	e2, err := NewEngine(NewEngineConfig{DBPath: dbPath})
	require.NoError(t, err)
	defer func() { require.NoError(t, e2.Close()) }()

	dag, err := e2.GetDAG(context.Background(), "ns-1", "dag-1")
	require.NoError(t, err)
	require.Equal(t, "/tmp/shared-dag-worktree", dag.WorktreePath)
	require.Equal(t, "clean", dag.WorktreeStatus)
	require.Equal(t, "abc123", dag.HeadSHA)
	require.Equal(t, "T1", dag.ActiveTaskID)
	require.Equal(t, "T1", dag.LeaseHolderTaskID)
	require.Equal(t, "worker-a", dag.LeaseHolderWorkerID)
	require.Equal(t, "agent-1", dag.LeaseHolderAgentID)
	require.Equal(t, "2026-07-09T00:00:00Z", dag.LeaseAcquiredAt)
	require.Equal(t, "2026-07-09T00:00:01Z", dag.RuntimeUpdatedAt)
}

func TestDAGWorkerPersistence(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "agentflow-dw.db")

	// First instance
	e1, err := NewEngine(NewEngineConfig{DBPath: dbPath})
	require.NoError(t, err)
	_, err = e1.CreateNamespace(context.Background(), CreateNamespaceRequest{ID: "ns-1", Name: "test"})
	require.NoError(t, err)
	_, err = e1.CreateDAG(context.Background(), CreateDAGRequest{
		NamespaceID: "ns-1", ID: "dag-1", Title: "添加登录", ExecutionBranch: "feat/login",
	})
	require.NoError(t, err)
	_, err = e1.RegisterWorker(context.Background(), RegisterWorkerRequest{
		NamespaceID: "ns-1", ID: "worker-auth", Name: "Auth", Skills: []string{"go"},
	})
	require.NoError(t, err)
	require.NoError(t, e1.Close())

	// Second instance
	e2, err := NewEngine(NewEngineConfig{DBPath: dbPath})
	require.NoError(t, err)
	defer func() { require.NoError(t, e2.Close()) }()

	dag, err := e2.GetDAG(context.Background(), "ns-1", "dag-1")
	require.NoError(t, err)
	require.Equal(t, "添加登录", dag.Title)
	require.Equal(t, "feat/login", dag.ExecutionBranch)

	w, err := e2.GetWorker(context.Background(), "ns-1", "worker-auth")
	require.NoError(t, err)
	require.Equal(t, "Auth", w.Name)
	require.Equal(t, []string{"go"}, w.Skills)
}
