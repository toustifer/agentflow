package engine

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// newRouteTestEngine builds an in-memory engine with one namespace.
func newRouteTestEngine(t *testing.T) *Engine {
	t.Helper()
	e, err := NewEngine(NewEngineConfig{DBPath: ":memory:"})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, e.Close()) })
	_, err = e.CreateNamespace(context.Background(), CreateNamespaceRequest{ID: "ns-1", Name: "route"})
	require.NoError(t, err)
	return e
}

// seedRouteTask creates an assigned task with the given route declaration and a
// valid issued launch ticket, i.e. a task that is ready to be started.
func seedRouteTask(t *testing.T, e *Engine, taskID string, declared map[string]string) {
	t.Helper()
	_, err := e.CreateTask(context.Background(), CreateTaskRequest{
		NamespaceID: "ns-1",
		ID:          taskID,
		Title:       "route task",
		Metadata:    declared,
	})
	require.NoError(t, err)
	_, err = e.UpdateTask(context.Background(), "ns-1", taskID, UpdateTaskRequest{
		Metadata: map[string]string{
			"launch.ticket":           "lt_1",
			"launch.ticket_state":     "issued",
			"launch.ticket_issued_at": time.Now().UTC().Format(time.RFC3339),
		},
	})
	require.NoError(t, err)
}

// startRouteMeta builds valid start metadata, optionally reporting a model.
func startRouteMeta(model string, reportModel bool) map[string]string {
	meta := map[string]string{
		"launch.ticket":    "lt_1",
		"worker_agent_id":  "agent-1",
		"runtime.provider": "deepseek",
		"runtime.status":   "started",
	}
	if reportModel {
		meta["runtime.model"] = model
	}
	return meta
}

func TestTransStartRejectsMissingDeclaredModel(t *testing.T) {
	t.Parallel()

	e := newRouteTestEngine(t)
	seedRouteTask(t, e, "T1", map[string]string{
		"runtime.provider": "deepseek",
		"runtime.model":    "deepseek-v4.1-flash",
	})

	_, err := e.TransitionTask(context.Background(), "ns-1", "T1", TransStart, startRouteMeta("", false))
	require.Error(t, err)
	require.ErrorIs(t, err, ErrInvalidTransition)
	// The error must be actionable: name the declared value and both ways out.
	require.Contains(t, err.Error(), "deepseek-v4.1-flash")
	require.Contains(t, err.Error(), "runtime.model")
	require.Contains(t, err.Error(), "re-declare")
}

func TestTransStartRejectsMismatchedDeclaredModel(t *testing.T) {
	t.Parallel()

	e := newRouteTestEngine(t)
	seedRouteTask(t, e, "T1", map[string]string{
		"runtime.provider": "deepseek",
		"runtime.model":    "deepseek-v4.1-flash",
	})

	_, err := e.TransitionTask(context.Background(), "ns-1", "T1", TransStart, startRouteMeta("some-other-model", true))
	require.Error(t, err)
	require.ErrorIs(t, err, ErrInvalidTransition)
	// Both the declared and the reported value must appear so the caller can see
	// exactly what disagreed.
	require.Contains(t, err.Error(), "deepseek-v4.1-flash")
	require.Contains(t, err.Error(), "some-other-model")
}

func TestTransStartAcceptsMatchingDeclaredModel(t *testing.T) {
	t.Parallel()

	e := newRouteTestEngine(t)
	seedRouteTask(t, e, "T1", map[string]string{
		"runtime.provider": "deepseek",
		"runtime.model":    "deepseek-v4.1-flash",
	})

	task, err := e.TransitionTask(context.Background(), "ns-1", "T1", TransStart, startRouteMeta("deepseek-v4.1-flash", true))
	require.NoError(t, err)
	require.Equal(t, TaskExecuting, task.State)
	require.Equal(t, "deepseek-v4.1-flash", task.Metadata["runtime.model"])
}

func TestTransStartWithoutDeclarationKeepsExistingBehaviour(t *testing.T) {
	t.Parallel()

	// No task-level declaration: reporting a model, reporting none, or even
	// reporting an unrelated model must all keep working exactly as before.
	cases := []struct {
		name        string
		model       string
		reportModel bool
	}{
		{name: "no model reported", reportModel: false},
		{name: "any model reported", model: "whatever-model", reportModel: true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			e := newRouteTestEngine(t)
			seedRouteTask(t, e, "T1", nil)

			task, err := e.TransitionTask(context.Background(), "ns-1", "T1", TransStart, startRouteMeta(tc.model, tc.reportModel))
			require.NoError(t, err)
			require.Equal(t, TaskExecuting, task.State)
		})
	}
}

func TestReassignClearsDeclaredRuntimeModel(t *testing.T) {
	t.Parallel()

	e := newRouteTestEngine(t)
	seedRouteTask(t, e, "T1", map[string]string{
		"runtime.provider": "deepseek",
		"runtime.model":    "deepseek-v4.1-flash",
	})

	task, err := e.TransitionTask(context.Background(), "ns-1", "T1", TransReassign, nil)
	require.NoError(t, err)
	require.Equal(t, TaskAssigned, task.State)
	require.Empty(t, task.Metadata["runtime.model"], "stale route must not survive a reassign")
	require.Empty(t, task.Metadata["runtime.provider"], "stale route must not survive a reassign")
}
