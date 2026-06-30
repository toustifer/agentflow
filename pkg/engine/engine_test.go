package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEngineTaskLifecycle(t *testing.T) {
	t.Parallel()

	e, err := NewEngine(NewEngineConfig{})
	require.NoError(t, err)
	defer func() { require.NoError(t, e.Close()) }()

	ns, err := e.CreateNamespace(context.Background(), CreateNamespaceRequest{
		ID:   "ns-1",
		Name: "agent-company",
	})
	require.NoError(t, err)
	require.Equal(t, "ns-1", ns.ID)

	task, err := e.CreateTask(context.Background(), CreateTaskRequest{
		NamespaceID:    "ns-1",
		ID:             "T1",
		Title:          "bootstrap",
		AssignedWorker: "worker-ops",
	})
	require.NoError(t, err)
	require.Equal(t, TaskAssigned, task.State)

	task, err = e.TransitionTask(context.Background(), "ns-1", "T1", TransStart, map[string]string{"actor": "leader"})
	require.NoError(t, err)
	require.Equal(t, TaskExecuting, task.State)

	task, err = e.TransitionTask(context.Background(), "ns-1", "T1", TransSubmit, map[string]string{"actor": "worker-ops", "reason": "self-check done"})
	require.NoError(t, err)
	require.Equal(t, TaskReviewPending, task.State)

	task, err = e.TransitionTask(context.Background(), "ns-1", "T1", TransRework, map[string]string{"actor": "reviewer", "reason": "missing test"})
	require.NoError(t, err)
	require.Equal(t, TaskReworkNeeded, task.State)
	require.Equal(t, 1, task.ReviewCycle)

	history, err := e.GetHistory(context.Background(), "ns-1", "T1")
	require.NoError(t, err)
	require.Len(t, history, 4)
	require.Equal(t, string(TransRework), history[3].Transition)
}

func TestEngineRejectsInvalidTransition(t *testing.T) {
	t.Parallel()

	e, err := NewEngine(NewEngineConfig{})
	require.NoError(t, err)
	defer func() { require.NoError(t, e.Close()) }()

	_, err = e.CreateNamespace(context.Background(), CreateNamespaceRequest{ID: "ns-1", Name: "agent-company"})
	require.NoError(t, err)
	_, err = e.CreateTask(context.Background(), CreateTaskRequest{NamespaceID: "ns-1", ID: "T1", Title: "bootstrap"})
	require.NoError(t, err)

	_, err = e.TransitionTask(context.Background(), "ns-1", "T1", TransPass, nil)
	require.ErrorIs(t, err, ErrInvalidTransition)
}

// ---------------------------------------------------------------------------
// SQLite persistence tests
// ---------------------------------------------------------------------------

func TestEngineInMemorySQLite(t *testing.T) {
	t.Parallel()

	e, err := NewEngine(NewEngineConfig{DBPath: ":memory:"})
	require.NoError(t, err)
	defer func() { require.NoError(t, e.Close()) }()

	_, err = e.CreateNamespace(context.Background(), CreateNamespaceRequest{ID: "ns-1", Name: "inmem-sqlite"})
	require.NoError(t, err)

	_, err = e.CreateTask(context.Background(), CreateTaskRequest{
		NamespaceID: "ns-1",
		ID:          "T1",
		Title:       "inmem-task",
	})
	require.NoError(t, err)

	task, err := e.TransitionTask(context.Background(), "ns-1", "T1", TransStart, nil)
	require.NoError(t, err)
	require.Equal(t, TaskExecuting, task.State)

	history, err := e.GetHistory(context.Background(), "ns-1", "T1")
	require.NoError(t, err)
	require.Len(t, history, 2)
}

func TestEngineFilePersistence_NamespacesAndTasks(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "agentflow.db")

	// --- first instance ---
	e1, err := NewEngine(NewEngineConfig{DBPath: dbPath})
	require.NoError(t, err)

	_, err = e1.CreateNamespace(context.Background(), CreateNamespaceRequest{
		ID:   "ns-prod",
		Name: "production",
		Metadata: map[string]string{"env": "prod"},
	})
	require.NoError(t, err)

	_, err = e1.CreateNamespace(context.Background(), CreateNamespaceRequest{
		ID:   "ns-dev",
		Name: "development",
	})
	require.NoError(t, err)

	_, err = e1.CreateTask(context.Background(), CreateTaskRequest{
		NamespaceID:    "ns-prod",
		ID:             "T1",
		Title:          "deploy",
		AssignedWorker: "bot",
	})
	require.NoError(t, err)

	require.NoError(t, e1.Close())

	// --- second instance, same file ---
	e2, err := NewEngine(NewEngineConfig{DBPath: dbPath})
	require.NoError(t, err)
	defer func() { require.NoError(t, e2.Close()) }()

	// Namespace survived
	ns, err := e2.GetNamespace(context.Background(), "ns-prod")
	require.NoError(t, err, "namespace should survive restart")
	require.Equal(t, "production", ns.Name)
	require.Equal(t, "prod", ns.Metadata["env"])

	// Second namespace
	ns2, err := e2.GetNamespace(context.Background(), "ns-dev")
	require.NoError(t, err)
	require.Equal(t, "development", ns2.Name)

	// Task survived
	task, err := e2.GetTask(context.Background(), "ns-prod", "T1")
	require.NoError(t, err)
	require.Equal(t, "deploy", task.Title)
	require.Equal(t, TaskAssigned, task.State)
	require.Equal(t, "bot", task.AssignedWorker)

	// ListNamespaces works
	list, err := e2.ListNamespaces(context.Background())
	require.NoError(t, err)
	require.Len(t, list, 2)

	// ListTasks works
	tasks, err := e2.ListTasks(context.Background(), "ns-prod", StateFilter{})
	require.NoError(t, err)
	require.Len(t, tasks, 1)
}

func TestEngineFilePersistence_TransitionsAndHistory(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "agentflow-trans.db")

	// --- first instance: create, transition, verify history ---
	e1, err := NewEngine(NewEngineConfig{DBPath: dbPath})
	require.NoError(t, err)

	_, err = e1.CreateNamespace(context.Background(), CreateNamespaceRequest{ID: "ns-1", Name: "test"})
	require.NoError(t, err)

	_, err = e1.CreateTask(context.Background(), CreateTaskRequest{
		NamespaceID: "ns-1",
		ID:          "T1",
		Title:       "cross-instance-task",
	})
	require.NoError(t, err)

	task, err := e1.TransitionTask(context.Background(), "ns-1", "T1", TransStart, map[string]string{"actor": "leader"})
	require.NoError(t, err)
	require.Equal(t, TaskExecuting, task.State)

	task, err = e1.TransitionTask(context.Background(), "ns-1", "T1", TransSubmit, map[string]string{"actor": "bot", "reason": "done"})
	require.NoError(t, err)
	require.Equal(t, TaskReviewPending, task.State)

	require.NoError(t, e1.Close())

	// --- second instance: state and history survived ---
	e2, err := NewEngine(NewEngineConfig{DBPath: dbPath})
	require.NoError(t, err)
	defer func() { require.NoError(t, e2.Close()) }()

	task, err = e2.GetTask(context.Background(), "ns-1", "T1")
	require.NoError(t, err)
	require.Equal(t, TaskReviewPending, task.State)

	history, err := e2.GetHistory(context.Background(), "ns-1", "T1")
	require.NoError(t, err)
	require.Len(t, history, 3, "should have create + start + submit events")
	require.Equal(t, "submit", history[2].Transition)

	// Transition again on reopened engine
	task, err = e2.TransitionTask(context.Background(), "ns-1", "T1", TransPass, map[string]string{"actor": "reviewer"})
	require.NoError(t, err)
	require.Equal(t, TaskDone, task.State)

	history, err = e2.GetHistory(context.Background(), "ns-1", "T1")
	require.NoError(t, err)
	require.Len(t, history, 4, "should now have 4 events")
}

// ---------------------------------------------------------------------------
// actor_role validation + available_transitions (Direction D)
// ---------------------------------------------------------------------------

func TestAvailableTransitions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		state    TaskState
		wantLen  int
		wantTrans []string
	}{
		{TaskAssigned, 1, []string{"start"}},
		{TaskExecuting, 2, []string{"submit", "cancel"}},
		{TaskReviewPending, 2, []string{"pass", "rework"}},
		{TaskReworkNeeded, 3, []string{"resume", "reassign", "cancel"}},
		{TaskDone, 0, nil},
		{TaskCancelled, 0, nil},
	}

	for _, tc := range tests {
		task := &Task{State: tc.state}
		avail := AvailableTransitions(task)
		require.Len(t, avail, tc.wantLen, "state=%s", tc.state)
		for i, want := range tc.wantTrans {
			require.Equal(t, want, avail[i].Transition, "state=%s idx=%d", tc.state, i)
		}
		// Every transition must have a non-empty Role and ToState
		for _, a := range avail {
			require.NotEmpty(t, a.Role, "state=%s transition=%s missing role", tc.state, a.Transition)
			require.NotEmpty(t, a.ToState, "state=%s transition=%s missing to_state", tc.state, a.Transition)
		}
	}
}

func TestValidateActorRole(t *testing.T) {
	t.Parallel()

	// leader can: start, resume, reassign, cancel
	require.NoError(t, validateActorRole(TransStart, "leader"))
	require.NoError(t, validateActorRole(TransResume, "leader"))
	require.NoError(t, validateActorRole(TransReassign, "leader"))
	require.NoError(t, validateActorRole(TransCancel, "leader"))
	// leader cannot: submit, pass, rework
	require.Error(t, validateActorRole(TransSubmit, "leader"))
	require.Error(t, validateActorRole(TransPass, "leader"))
	require.Error(t, validateActorRole(TransRework, "leader"))

	// worker can: submit
	require.NoError(t, validateActorRole(TransSubmit, "worker"))
	// worker cannot: start, pass, rework, cancel
	require.Error(t, validateActorRole(TransStart, "worker"))
	require.Error(t, validateActorRole(TransPass, "worker"))

	// reviewer can: pass, rework
	require.NoError(t, validateActorRole(TransPass, "reviewer"))
	require.NoError(t, validateActorRole(TransRework, "reviewer"))
	// reviewer cannot: start, submit, cancel
	require.Error(t, validateActorRole(TransStart, "reviewer"))
	require.Error(t, validateActorRole(TransSubmit, "reviewer"))
	require.Error(t, validateActorRole(TransCancel, "reviewer"))

	// unknown role
	err := validateActorRole(TransStart, "alien")
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown actor_role")
}

func TestTransitionTaskWithActorRole(t *testing.T) {
	t.Parallel()

	e, err := NewEngine(NewEngineConfig{})
	require.NoError(t, err)
	defer func() { require.NoError(t, e.Close()) }()

	_, err = e.CreateNamespace(context.Background(), CreateNamespaceRequest{ID: "test", Name: "Test"})
	require.NoError(t, err)

	_, err = e.CreateTask(context.Background(), CreateTaskRequest{
		NamespaceID: "test", ID: "t1", Title: "Role Test", AssignedWorker: "w1",
	})
	require.NoError(t, err)

	// Leader starts
	task, err := e.TransitionTask(context.Background(), "test", "t1", TransStart,
		map[string]string{"actor_role": "leader", "actor": "leader"})
	require.NoError(t, err)
	require.Equal(t, TaskExecuting, task.State)

	// Worker submits
	task, err = e.TransitionTask(context.Background(), "test", "t1", TransSubmit,
		map[string]string{"actor_role": "worker", "actor": "w1"})
	require.NoError(t, err)
	require.Equal(t, TaskReviewPending, task.State)

	// Worker tries to pass → rejected
	_, err = e.TransitionTask(context.Background(), "test", "t1", TransPass,
		map[string]string{"actor_role": "worker"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "not allowed")

	// Leader tries to submit → rejected
	_, err = e.TransitionTask(context.Background(), "test", "t1", TransSubmit,
		map[string]string{"actor_role": "leader"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "not allowed")

	// Reviewer passes → ok
	task, err = e.TransitionTask(context.Background(), "test", "t1", TransPass,
		map[string]string{"actor_role": "reviewer"})
	require.NoError(t, err)
	require.Equal(t, TaskDone, task.State)
}

func TestTransitionTaskBackwardCompat(t *testing.T) {
	t.Parallel()

	e, err := NewEngine(NewEngineConfig{})
	require.NoError(t, err)
	defer func() { require.NoError(t, e.Close()) }()

	_, err = e.CreateNamespace(context.Background(), CreateNamespaceRequest{ID: "test", Name: "Test"})
	require.NoError(t, err)
	_, err = e.CreateTask(context.Background(), CreateTaskRequest{
		NamespaceID: "test", ID: "t1", Title: "Compat", AssignedWorker: "w1",
	})
	require.NoError(t, err)

	// Without actor_role, transitions still work (backward compat)
	task, err := e.TransitionTask(context.Background(), "test", "t1", TransStart, nil)
	require.NoError(t, err)
	require.Equal(t, TaskExecuting, task.State)

	task, err = e.TransitionTask(context.Background(), "test", "t1", TransSubmit,
		map[string]string{"actor": "w1"})
	require.NoError(t, err)
	require.Equal(t, TaskReviewPending, task.State)
}

func TestEngineFilePersistence_DatabaseFileCreated(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "agentflow-check.db")

	e, err := NewEngine(NewEngineConfig{DBPath: dbPath})
	require.NoError(t, err)

	_, err = e.CreateNamespace(context.Background(), CreateNamespaceRequest{ID: "ns-1", Name: "check"})
	require.NoError(t, err)
	require.NoError(t, e.Close())

	// The database file should exist on disk
	info, err := os.Stat(dbPath)
	require.NoError(t, err, "database file should exist after close")
	require.Greater(t, info.Size(), int64(0), "database file should not be empty")
}
