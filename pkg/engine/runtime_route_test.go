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

// seedRouteTask creates an assigned task with the given metadata and a valid
// issued launch ticket, i.e. a task that is ready to be started.
func seedRouteTask(t *testing.T, e *Engine, taskID string, metadata map[string]string) {
	t.Helper()
	_, err := e.CreateTask(context.Background(), CreateTaskRequest{
		NamespaceID: "ns-1",
		ID:          taskID,
		Title:       "route task",
		Metadata:    metadata,
	})
	require.NoError(t, err)
	issueTicket(t, e, taskID, "lt_1")
}

// issueTicket simulates the re-prepare step that mints a fresh launch ticket.
func issueTicket(t *testing.T, e *Engine, taskID, ticket string) {
	t.Helper()
	_, err := e.UpdateTask(context.Background(), "ns-1", taskID, UpdateTaskRequest{
		Metadata: map[string]string{
			"launch.ticket":           ticket,
			"launch.ticket_state":     "issued",
			"launch.ticket_issued_at": time.Now().UTC().Format(time.RFC3339),
		},
	})
	require.NoError(t, err)
}

// startRouteMeta builds valid start metadata, optionally reporting the observed
// model (the runtime.* observation).
func startRouteMeta(ticket, model string, reportModel bool) map[string]string {
	meta := map[string]string{
		"launch.ticket":    ticket,
		"worker_agent_id":  "agent-1",
		"runtime.provider": "deepseek",
		"runtime.status":   "started",
	}
	if reportModel {
		meta["runtime.model"] = model
	}
	return meta
}

func declaredDeepseek() map[string]string {
	return map[string]string{
		MetaRouteProvider: "deepseek",
		MetaRouteModel:    "deepseek-v4.1-flash",
	}
}

func TestTransStartRejectsMissingDeclaredModel(t *testing.T) {
	t.Parallel()

	e := newRouteTestEngine(t)
	seedRouteTask(t, e, "T1", declaredDeepseek())

	_, err := e.TransitionTask(context.Background(), "ns-1", "T1", TransStart, startRouteMeta("lt_1", "", false))
	require.Error(t, err)
	require.ErrorIs(t, err, ErrInvalidTransition)
	// The error must be actionable: name the declared value and both ways out.
	require.Contains(t, err.Error(), "deepseek-v4.1-flash")
	require.Contains(t, err.Error(), MetaRouteModel)
	require.Contains(t, err.Error(), "re-declare")
}

func TestTransStartRejectsMismatchedDeclaredModel(t *testing.T) {
	t.Parallel()

	e := newRouteTestEngine(t)
	seedRouteTask(t, e, "T1", declaredDeepseek())

	_, err := e.TransitionTask(context.Background(), "ns-1", "T1", TransStart, startRouteMeta("lt_1", "some-other-model", true))
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
	seedRouteTask(t, e, "T1", declaredDeepseek())

	task, err := e.TransitionTask(context.Background(), "ns-1", "T1", TransStart, startRouteMeta("lt_1", "deepseek-v4.1-flash", true))
	require.NoError(t, err)
	require.Equal(t, TaskExecuting, task.State)
	// Declaration and observation are stored under separate keys.
	require.Equal(t, "deepseek-v4.1-flash", task.Metadata[MetaRouteModel])
	require.Equal(t, "deepseek-v4.1-flash", task.Metadata[MetaRuntimeModel])
}

func TestTransStartWithoutDeclarationKeepsExistingBehaviour(t *testing.T) {
	t.Parallel()

	// No declaration: reporting a model, reporting none, or even reporting an
	// unrelated model must all keep working exactly as before.
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

			task, err := e.TransitionTask(context.Background(), "ns-1", "T1", TransStart, startRouteMeta("lt_1", tc.model, tc.reportModel))
			require.NoError(t, err)
			require.Equal(t, TaskExecuting, task.State)
		})
	}
}

func TestReassignPreservesDeclaredRouteAndClearsObservation(t *testing.T) {
	t.Parallel()

	e := newRouteTestEngine(t)
	seedRouteTask(t, e, "T1", declaredDeepseek())

	// A real start records the observation alongside the declaration.
	_, err := e.TransitionTask(context.Background(), "ns-1", "T1", TransStart, startRouteMeta("lt_1", "deepseek-v4.1-flash", true))
	require.NoError(t, err)

	task, err := e.TransitionTask(context.Background(), "ns-1", "T1", TransReassign, nil)
	require.NoError(t, err)
	require.Equal(t, TaskAssigned, task.State)

	// The declaration is the contract and must survive.
	require.Equal(t, "deepseek", task.Metadata[MetaRouteProvider])
	require.Equal(t, "deepseek-v4.1-flash", task.Metadata[MetaRouteModel])
	// The observation is stale and must be cleared.
	require.Empty(t, task.Metadata[MetaRuntimeProvider])
	require.Empty(t, task.Metadata[MetaRuntimeModel])
	require.Empty(t, task.Metadata[MetaRuntimeStatus])
}

// TestDeclaredRouteConstraintSurvivesReassign is the core regression guard for
// the hole this refactor closes: reassign used to clear the declaration too, so
// the model constraint silently vanished on the standard Worker-recovery path.
func TestDeclaredRouteConstraintSurvivesReassign(t *testing.T) {
	t.Parallel()

	e := newRouteTestEngine(t)
	seedRouteTask(t, e, "T1", declaredDeepseek())

	_, err := e.TransitionTask(context.Background(), "ns-1", "T1", TransStart, startRouteMeta("lt_1", "deepseek-v4.1-flash", true))
	require.NoError(t, err)
	_, err = e.TransitionTask(context.Background(), "ns-1", "T1", TransReassign, nil)
	require.NoError(t, err)

	// Re-prepare mints a fresh ticket for the replacement Worker.
	issueTicket(t, e, "T1", "lt_2")

	// The constraint must still bite after the reassign.
	_, err = e.TransitionTask(context.Background(), "ns-1", "T1", TransStart, startRouteMeta("lt_2", "wrong-model", true))
	require.Error(t, err)
	require.ErrorIs(t, err, ErrInvalidTransition)
	require.Contains(t, err.Error(), "deepseek-v4.1-flash")

	// ...and the declared model still starts the task.
	task, err := e.TransitionTask(context.Background(), "ns-1", "T1", TransStart, startRouteMeta("lt_2", "deepseek-v4.1-flash", true))
	require.NoError(t, err)
	require.Equal(t, TaskExecuting, task.State)
}

// TestLegacyRuntimeKeysActAsDeclarationBeforeStart locks in the compatibility
// rule: pre-split data stored the declaration in the runtime.* keys, and it is
// still honoured while the task has never started.
func TestLegacyRuntimeKeysActAsDeclarationBeforeStart(t *testing.T) {
	t.Parallel()

	e := newRouteTestEngine(t)
	seedRouteTask(t, e, "T1", map[string]string{
		"runtime.provider": "legacy-provider",
		"runtime.model":    "legacy-model",
	})

	_, err := e.TransitionTask(context.Background(), "ns-1", "T1", TransStart, startRouteMeta("lt_1", "different-model", true))
	require.Error(t, err)
	require.ErrorIs(t, err, ErrInvalidTransition)
	require.Contains(t, err.Error(), "legacy-model")
}

// TestLegacyRuntimeKeysIgnoredAfterStart is the other half of the rule: once a
// task has started, runtime.model means "what actually ran" and must not be
// reinterpreted as a contract.
func TestLegacyRuntimeKeysIgnoredAfterStart(t *testing.T) {
	t.Parallel()

	e := newRouteTestEngine(t)
	seedRouteTask(t, e, "T1", map[string]string{
		"runtime.provider": "legacy-provider",
		"runtime.model":    "legacy-model",
		"runtime.status":   "started",
	})

	task, err := e.TransitionTask(context.Background(), "ns-1", "T1", TransStart, startRouteMeta("lt_1", "different-model", true))
	require.NoError(t, err)
	require.Equal(t, TaskExecuting, task.State)
}

// TestDeclaredRouteModelPrefersRouteKey covers precedence: an explicit route.*
// declaration wins over any leftover runtime.* value, and the legacy fallback
// only applies before the task has started.
func TestDeclaredRouteModelPrefersRouteKey(t *testing.T) {
	t.Parallel()

	task := &Task{Metadata: map[string]string{
		MetaRouteModel:    "declared-model",
		MetaRuntimeModel:  "observed-model",
		MetaRuntimeStatus: "started",
	}}
	require.Equal(t, "declared-model", declaredRouteModel(task))

	legacy := &Task{Metadata: map[string]string{MetaRuntimeModel: "observed-model"}}
	require.Equal(t, "observed-model", declaredRouteModel(legacy))

	startedLegacy := &Task{Metadata: map[string]string{
		MetaRuntimeModel:  "observed-model",
		MetaRuntimeStatus: "started",
	}}
	require.Empty(t, declaredRouteModel(startedLegacy))

	require.Empty(t, declaredRouteModel(&Task{}))
}
