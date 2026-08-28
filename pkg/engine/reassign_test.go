package engine

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestReassignClearsDeadWorkerBinding locks in the engine fix for the
// "already bound to another worker_agent_id" deadlock: a task whose Worker
// died mid-flight keeps the old binding; reassign must clear it (and the
// runtime/launch metadata) so the Leader can re-prepare and start with a
// fresh agent without cancelling the task.
func TestReassignClearsDeadWorkerBinding(t *testing.T) {
	t.Parallel()

	e, err := NewEngine(NewEngineConfig{DBPath: ":memory:"})
	require.NoError(t, err)
	defer func() { require.NoError(t, e.Close()) }()

	_, err = e.CreateNamespace(context.Background(), CreateNamespaceRequest{ID: "ns-1", Name: "reassign"})
	require.NoError(t, err)
	_, err = e.CreateTask(context.Background(), CreateTaskRequest{NamespaceID: "ns-1", ID: "T1", Title: "rebind"})
	require.NoError(t, err)

	// Simulate a dead Worker still bound to the assigned task (the InsightTutor
	// incident state: zombie agent id on an assigned task with runtime=started).
	task, err := e.UpdateTask(context.Background(), "ns-1", "T1", UpdateTaskRequest{
		WorkerAgentID: "zombie-1",
		Metadata: map[string]string{
			"worker_agent_id":   "zombie-1",
			"runtime.provider":  "claude_code",
			"runtime.status":    "started",
			"launch.ticket":     "lt_old",
			"launch.ticket_state": "issued",
		},
	})
	require.NoError(t, err)
	require.Equal(t, "zombie-1", task.WorkerAgentID)

	// An assigned task may now be reassigned (the escape hatch).
	avail := AvailableTransitions(task)
	foundReassign := false
	for _, a := range avail {
		if a.Transition == string(TransReassign) {
			foundReassign = true
			break
		}
	}
	require.True(t, foundReassign, "assigned task must offer reassign")

	// Reassign: state stays assigned, the whole binding is dropped.
	task, err = e.TransitionTask(context.Background(), "ns-1", "T1", TransReassign, nil)
	require.NoError(t, err)
	require.Equal(t, TaskAssigned, task.State)
	require.Empty(t, task.WorkerAgentID)
	require.Empty(t, task.Metadata["worker_agent_id"])
	require.Empty(t, task.Metadata["runtime.provider"])
	require.Empty(t, task.Metadata["runtime.status"])
	require.Empty(t, task.Metadata["launch.ticket"])
	require.Empty(t, task.Metadata["launch.ticket_state"])

	// Re-prepare issues a fresh ticket, then start binds the NEW agent and
	// must no longer be rejected as "already bound".
	_, err = e.UpdateTask(context.Background(), "ns-1", "T1", UpdateTaskRequest{Metadata: map[string]string{
		"launch.ticket":          "lt_new",
		"launch.ticket_state":    "issued",
		"launch.ticket_issued_at": time.Now().UTC().Format(time.RFC3339),
	}})
	require.NoError(t, err)

	task, err = e.TransitionTask(context.Background(), "ns-1", "T1", TransStart, map[string]string{
		"launch.ticket":    "lt_new",
		"worker_agent_id":  "fresh-1",
		"runtime.provider": "opencode2",
		"runtime.status":   "started",
	})
	require.NoError(t, err)
	require.Equal(t, TaskExecuting, task.State)
	require.Equal(t, "fresh-1", task.WorkerAgentID)
	require.Equal(t, "consumed", task.Metadata["launch.ticket_state"])
}

// TestReassignKeepsSemanticsForOtherStates guards against regressing the
// original reassign path: from executing, reassign returns to assigned and
// clears the binding there too.
func TestReassignKeepsSemanticsForOtherStates(t *testing.T) {
	t.Parallel()

	e, err := NewEngine(NewEngineConfig{DBPath: ":memory:"})
	require.NoError(t, err)
	defer func() { require.NoError(t, e.Close()) }()

	_, err = e.CreateNamespace(context.Background(), CreateNamespaceRequest{ID: "ns-1", Name: "reassign2"})
	require.NoError(t, err)
	_, err = e.CreateTask(context.Background(), CreateTaskRequest{NamespaceID: "ns-1", ID: "T1", Title: "swap"})
	require.NoError(t, err)

	task, err := e.UpdateTask(context.Background(), "ns-1", "T1", UpdateTaskRequest{
		WorkerAgentID: "worker-a",
		Metadata:      map[string]string{"worker_agent_id": "worker-a", "runtime.status": "started"},
	})
	require.NoError(t, err)
	task, err = e.TransitionTask(context.Background(), "ns-1", "T1", TransStart, map[string]string{})
	require.NoError(t, err)
	require.Equal(t, TaskExecuting, task.State)

	task, err = e.TransitionTask(context.Background(), "ns-1", "T1", TransReassign, nil)
	require.NoError(t, err)
	require.Equal(t, TaskAssigned, task.State)
	require.Empty(t, task.WorkerAgentID)
	require.Empty(t, task.Metadata["worker_agent_id"])
}