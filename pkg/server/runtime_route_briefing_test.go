package server

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/toustifer/agentflow/pkg/engine"
)

const routeInstructionPrefix = "Runtime route is declared"

// seedBriefingRoute registers worker-b with a worker-level route declaration and
// creates a DAG-backed task with a task-level declaration (either may be nil).
func seedBriefingRoute(t *testing.T, srv *Server, taskID string, workerMeta, taskMeta map[string]string) {
	t.Helper()
	_, err := srv.engine.RegisterWorker(context.Background(), engine.RegisterWorkerRequest{
		NamespaceID:    "ns-1",
		ID:             "worker-b",
		Name:           "Worker B",
		PromptTemplate: "Task {task_id} in {worktree_path} on {branch}",
		Metadata:       workerMeta,
	})
	require.NoError(t, err)
	_, err = srv.engine.CreateDAG(context.Background(), engine.CreateDAGRequest{
		NamespaceID:     "ns-1",
		ID:              "dag-1",
		Title:           "Test DAG",
		ExecutionBranch: "feat/test",
	})
	require.NoError(t, err)
	_, err = srv.engine.CreateTask(context.Background(), engine.CreateTaskRequest{
		NamespaceID:    "ns-1",
		ID:             taskID,
		Title:          "execute",
		AssignedWorker: "worker-b",
		DAGID:          "dag-1",
		Metadata:       taskMeta,
	})
	require.NoError(t, err)
}

func prepareStartBriefing(t *testing.T, srv *Server, taskID string) map[string]any {
	t.Helper()
	result, err := srv.Handle(context.Background(), "task_prepare_start", map[string]any{
		"namespace_id": "ns-1",
		"task_id":      taskID,
	})
	require.NoError(t, err)
	briefing, ok := result["worker_launch"].(map[string]any)
	require.True(t, ok, "prepare_start must return a worker_launch briefing")
	return briefing
}

func prepareStartTicket(t *testing.T, srv *Server, taskID string) string {
	t.Helper()
	briefing := prepareStartBriefing(t, srv, taskID)
	ticket, ok := briefing["launch_ticket"].(string)
	require.True(t, ok)
	require.NotEmpty(t, ticket)
	return ticket
}

func briefingInstructions(t *testing.T, briefing map[string]any) []string {
	t.Helper()
	raw, ok := briefing["launch_instructions"].([]any)
	require.True(t, ok)
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		s, ok := item.(string)
		require.True(t, ok)
		out = append(out, s)
	}
	return out
}

func findRouteInstruction(t *testing.T, briefing map[string]any) string {
	t.Helper()
	for _, instr := range briefingInstructions(t, briefing) {
		if strings.HasPrefix(instr, routeInstructionPrefix) {
			return instr
		}
	}
	return ""
}

func startTaskWith(t *testing.T, srv *Server, taskID, ticket, provider, model string) (map[string]any, error) {
	t.Helper()
	return srv.Handle(context.Background(), "task_transition", map[string]any{
		"namespace_id": "ns-1",
		"task_id":      taskID,
		"transition":   "start",
		"actor_role":   "leader",
		"metadata": map[string]any{
			"launch.ticket":    ticket,
			"worker_agent_id":  "agent-e2e",
			"runtime.provider": provider,
			"runtime.status":   "started",
			"runtime.model":    model,
		},
	})
}

func declaredDeepseekRoute() map[string]string {
	return map[string]string{MetaRouteProvider: "deepseek", MetaRouteModel: "deepseek-v4.1-flash"}
}

func TestPrepareStartBriefingTaskRouteOutranksWorkerRoute(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	seedBriefingRoute(t, srv, "T-route",
		map[string]string{MetaRouteProvider: "anthropic", MetaRouteModel: "claude-sonnet-4"},
		declaredDeepseekRoute(),
	)

	briefing := prepareStartBriefing(t, srv, "T-route")

	require.Equal(t, RouteSourceTask, briefing["route_source"])
	require.Equal(t, "deepseek", briefing["route.provider"])
	require.Equal(t, "deepseek-v4.1-flash", briefing["route.model"])
	require.Equal(t, false, briefing["route.legacy"])
}

func TestPrepareStartBriefingFallsBackToWorkerRoute(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	seedBriefingRoute(t, srv, "T-route",
		map[string]string{MetaRouteProvider: "anthropic", MetaRouteModel: "claude-sonnet-4"},
		nil,
	)

	briefing := prepareStartBriefing(t, srv, "T-route")

	require.Equal(t, RouteSourceWorker, briefing["route_source"])
	require.Equal(t, "anthropic", briefing["route.provider"])
	require.Equal(t, "claude-sonnet-4", briefing["route.model"])
}

func TestPrepareStartBriefingRouteUnsetWhenNeitherDeclares(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	seedBriefingRoute(t, srv, "T-route", nil, nil)

	briefing := prepareStartBriefing(t, srv, "T-route")

	require.Equal(t, RouteSourceUnset, briefing["route_source"])
	require.Equal(t, "", briefing["route.provider"])
	require.Equal(t, "", briefing["route.model"])
	require.Equal(t, "", briefing["runtime.provider"])
	require.Equal(t, "", briefing["runtime.model"])

	// With no declared route there must be no route instruction to follow.
	require.Empty(t, findRouteInstruction(t, briefing))
}

func TestPrepareStartBriefingInstructionCarriesConcreteRoute(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	seedBriefingRoute(t, srv, "T-route", nil, declaredDeepseekRoute())

	briefing := prepareStartBriefing(t, srv, "T-route")

	instr := findRouteInstruction(t, briefing)
	require.NotEmpty(t, instr, "a declared route must produce an explicit start instruction")

	// The concrete values must be present verbatim — a vague reminder is not
	// enough, because TransStart rejects a missing or mismatching model.
	require.Contains(t, instr, "deepseek")
	require.Contains(t, instr, "deepseek-v4.1-flash")
	require.Contains(t, instr, "runtime.provider")
	require.Contains(t, instr, "runtime.model")
	require.Contains(t, instr, "task_transition(start)")
	require.Contains(t, instr, "source=task")
}

func TestPrepareStartBriefingWorkerRouteAlsoProducesInstruction(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	seedBriefingRoute(t, srv, "T-route",
		map[string]string{MetaRouteProvider: "anthropic", MetaRouteModel: "claude-sonnet-4"},
		nil,
	)

	briefing := prepareStartBriefing(t, srv, "T-route")

	instr := findRouteInstruction(t, briefing)
	require.NotEmpty(t, instr, "a worker-sourced route must also be spelled out for the Leader")
	require.Contains(t, instr, "claude-sonnet-4")
	require.Contains(t, instr, "source=worker")
}

func TestResolveRuntimeRouteSeparatesDeclarationFromObservation(t *testing.T) {
	t.Parallel()

	taskRoute := map[string]string{MetaRouteProvider: "deepseek", MetaRouteModel: "deepseek-v4.1-flash"}
	workerRoute := map[string]string{MetaRouteProvider: "anthropic", MetaRouteModel: "claude-sonnet-4"}
	observed := map[string]string{
		MetaRouteProvider:   "deepseek",
		MetaRouteModel:      "deepseek-v4.1-flash",
		MetaRuntimeProvider: "dsh",
		MetaRuntimeModel:    "deepseek-v4.1-flash",
		MetaRuntimeStatus:   "started",
	}

	tests := []struct {
		name         string
		task, worker map[string]string
		wantSource   string
		wantProvider string
		wantModel    string
	}{
		{name: "task wins", task: taskRoute, worker: workerRoute, wantSource: RouteSourceTask, wantProvider: "deepseek", wantModel: "deepseek-v4.1-flash"},
		{name: "worker fallback", worker: workerRoute, wantSource: RouteSourceWorker, wantProvider: "anthropic", wantModel: "claude-sonnet-4"},
		{name: "neither", wantSource: RouteSourceUnset},
		{name: "nil maps", task: nil, worker: nil, wantSource: RouteSourceUnset},
		{name: "declaration with observation", task: observed, wantSource: RouteSourceTask, wantProvider: "deepseek", wantModel: "deepseek-v4.1-flash"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := resolveRuntimeRoute(tc.task, tc.worker)
			require.Equal(t, tc.wantSource, got.Source)
			require.Equal(t, tc.wantProvider, got.Provider)
			require.Equal(t, tc.wantModel, got.Model)
		})
	}

	// The observation is reported independently of the declaration, and never
	// overwrites it.
	got := resolveRuntimeRoute(observed, workerRoute)
	require.Equal(t, "deepseek", got.Provider, "declaration must not be overwritten by the observation")
	require.Equal(t, "deepseek-v4.1-flash", got.Model)
	require.Equal(t, "dsh", got.ObservedProvider)
	require.Equal(t, "deepseek-v4.1-flash", got.ObservedModel)
	require.False(t, got.Legacy)
}

// TestDeclaredRouteSurvivesReassign is the server-level guard for the core hole:
// after a reassign the declaration must still drive both the briefing and the
// start-time validation.
func TestDeclaredRouteSurvivesReassign(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	seedBriefingRoute(t, srv, "T-survive", nil, declaredDeepseekRoute())

	firstBriefing := prepareStartBriefing(t, srv, "T-survive")
	require.Equal(t, RouteSourceTask, firstBriefing["route_source"])
	require.Equal(t, "deepseek-v4.1-flash", firstBriefing["route.model"])

	// The standard Worker-recovery path.
	_, err := srv.Handle(context.Background(), "task_transition", map[string]any{
		"namespace_id": "ns-1",
		"task_id":      "T-survive",
		"transition":   "reassign",
		"actor_role":   "leader",
	})
	require.NoError(t, err)

	// 1. The declaration must still be resolved after the reassign.
	secondBriefing := prepareStartBriefing(t, srv, "T-survive")
	require.Equal(t, RouteSourceTask, secondBriefing["route_source"], "declaration must survive a reassign")
	require.Equal(t, "deepseek", secondBriefing["route.provider"])
	require.Equal(t, "deepseek-v4.1-flash", secondBriefing["route.model"])
	require.NotEmpty(t, findRouteInstruction(t, secondBriefing))

	// The reassign cleared the stale observation.
	require.Empty(t, secondBriefing["runtime.provider"])
	require.Empty(t, secondBriefing["runtime.model"])

	ticket := prepareStartTicket(t, srv, "T-survive")

	// 2. The start-time constraint must still bite after the reassign.
	_, err = startTaskWith(t, srv, "T-survive", ticket, "deepseek", "wrong-model")
	require.Error(t, err)
	require.Contains(t, err.Error(), "deepseek-v4.1-flash")

	result, err := startTaskWith(t, srv, "T-survive", ticket, "deepseek", "deepseek-v4.1-flash")
	require.NoError(t, err)
	require.Equal(t, "executing", result["state"])
}

func TestPrepareStartBriefingReportsObservedRouteAfterStart(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	seedBriefingRoute(t, srv, "T-obs", nil, declaredDeepseekRoute())

	ticket := prepareStartTicket(t, srv, "T-obs")
	_, err := startTaskWith(t, srv, "T-obs", ticket, "dsh", "deepseek-v4.1-flash")
	require.NoError(t, err)

	briefing := prepareStartBriefing(t, srv, "T-obs")

	// Declaration (contract) and observation (what ran) are both present and
	// distinguishable.
	require.Equal(t, RouteSourceTask, briefing["route_source"])
	require.Equal(t, "deepseek", briefing["route.provider"])
	require.Equal(t, "deepseek-v4.1-flash", briefing["route.model"])
	require.Equal(t, "dsh", briefing["runtime.provider"])
	require.Equal(t, "deepseek-v4.1-flash", briefing["runtime.model"])
}

// TestTaskGetShowsDeclaredAndObservedRouteSeparately covers AC 6: the projection
// must present both routes side by side without either overwriting the other.
func TestTaskGetShowsDeclaredAndObservedRouteSeparately(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	seedBriefingRoute(t, srv, "T-audit", nil, declaredDeepseekRoute())

	// Simulate an observation that differs from the declaration so that any
	// accidental key collision would be visible.
	_, err := srv.engine.UpdateTask(context.Background(), "ns-1", "T-audit", engine.UpdateTaskRequest{
		Metadata: map[string]string{
			MetaRuntimeProvider: "dsh",
			MetaRuntimeModel:    "observed-model",
			MetaRuntimeStatus:   "started",
		},
	})
	require.NoError(t, err)

	got, err := srv.Handle(context.Background(), "task_get", map[string]any{
		"namespace_id": "ns-1",
		"task_id":      "T-audit",
	})
	require.NoError(t, err)

	declared, ok := got["declared_route"].(map[string]any)
	require.True(t, ok, "task_get must project declared_route")
	require.Equal(t, "deepseek", declared["provider"])
	require.Equal(t, "deepseek-v4.1-flash", declared["model"])
	require.Equal(t, RouteSourceTask, declared["source"])

	observed, ok := got["observed_route"].(map[string]any)
	require.True(t, ok, "task_get must project observed_route")
	require.Equal(t, "dsh", observed["provider"])
	require.Equal(t, "observed-model", observed["model"])

	// Neither overwrites the other, and both survive under their own metadata
	// keys as well.
	meta := taskMeta(t, got)
	require.Equal(t, "deepseek-v4.1-flash", meta[MetaRouteModel])
	require.Equal(t, "observed-model", meta[MetaRuntimeModel])
}

// TestTaskGetDeclaredRouteUsesLegacyFallback covers the AC 4 compatibility rule
// at the projection layer.
func TestTaskGetDeclaredRouteUsesLegacyFallback(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	seedBriefingRoute(t, srv, "T-legacy", nil, map[string]string{
		MetaRuntimeProvider: "legacy-provider",
		MetaRuntimeModel:    "legacy-model",
	})

	got, err := srv.Handle(context.Background(), "task_get", map[string]any{
		"namespace_id": "ns-1",
		"task_id":      "T-legacy",
	})
	require.NoError(t, err)

	declared, ok := got["declared_route"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "legacy-provider", declared["provider"])
	require.Equal(t, "legacy-model", declared["model"])
	require.Equal(t, true, declared["legacy"], "the projection must disclose that the legacy rule fired")
}
