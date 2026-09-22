package server

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/toustifer/agentflow/pkg/engine"
)

// seedBriefingRoute registers worker-b with a worker-level route and creates a
// DAG-backed task with a task-level route (either may be nil).
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

func TestPrepareStartBriefingTaskRouteOutranksWorkerRoute(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	seedBriefingRoute(t, srv, "T-route",
		map[string]string{MetaRuntimeProvider: "anthropic", MetaRuntimeModel: "claude-sonnet-4"},
		map[string]string{MetaRuntimeProvider: "deepseek", MetaRuntimeModel: "deepseek-v4.1-flash"},
	)

	briefing := prepareStartBriefing(t, srv, "T-route")

	require.Equal(t, RouteSourceTask, briefing["runtime.route_source"])
	require.Equal(t, "deepseek", briefing["runtime.provider"])
	require.Equal(t, "deepseek-v4.1-flash", briefing["runtime.model"])
}

func TestPrepareStartBriefingFallsBackToWorkerRoute(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	seedBriefingRoute(t, srv, "T-route",
		map[string]string{MetaRuntimeProvider: "anthropic", MetaRuntimeModel: "claude-sonnet-4"},
		nil,
	)

	briefing := prepareStartBriefing(t, srv, "T-route")

	require.Equal(t, RouteSourceWorker, briefing["runtime.route_source"])
	require.Equal(t, "anthropic", briefing["runtime.provider"])
	require.Equal(t, "claude-sonnet-4", briefing["runtime.model"])
}

func TestPrepareStartBriefingRouteUnsetWhenNeitherDeclares(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	seedBriefingRoute(t, srv, "T-route", nil, nil)

	briefing := prepareStartBriefing(t, srv, "T-route")

	require.Equal(t, RouteSourceUnset, briefing["runtime.route_source"])
	require.Equal(t, "", briefing["runtime.provider"])
	require.Equal(t, "", briefing["runtime.model"])

	// With no effective route there must be no route instruction to follow.
	for _, instr := range briefingInstructions(t, briefing) {
		require.NotContains(t, instr, "Runtime route is pinned")
	}
}

func TestPrepareStartBriefingInstructionCarriesConcreteRoute(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	seedBriefingRoute(t, srv, "T-route", nil,
		map[string]string{MetaRuntimeProvider: "deepseek", MetaRuntimeModel: "deepseek-v4.1-flash"},
	)

	briefing := prepareStartBriefing(t, srv, "T-route")

	var routeInstruction string
	for _, instr := range briefingInstructions(t, briefing) {
		if len(instr) > 0 && instr[:len("Runtime route is pinned")] == "Runtime route is pinned" {
			routeInstruction = instr
			break
		}
	}
	require.NotEmpty(t, routeInstruction, "an effective route must produce an explicit start instruction")

	// The concrete values must be present verbatim — a vague reminder is not
	// enough, because TransStart rejects a missing or mismatching model.
	require.Contains(t, routeInstruction, "deepseek")
	require.Contains(t, routeInstruction, "deepseek-v4.1-flash")
	require.Contains(t, routeInstruction, "runtime.provider")
	require.Contains(t, routeInstruction, "runtime.model")
	require.Contains(t, routeInstruction, "task_transition(start)")
}

func TestPrepareStartBriefingWorkerRouteAlsoProducesInstruction(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	seedBriefingRoute(t, srv, "T-route",
		map[string]string{MetaRuntimeProvider: "anthropic", MetaRuntimeModel: "claude-sonnet-4"},
		nil,
	)

	briefing := prepareStartBriefing(t, srv, "T-route")

	found := false
	for _, instr := range briefingInstructions(t, briefing) {
		if len(instr) >= len("Runtime route is pinned") && instr[:len("Runtime route is pinned")] == "Runtime route is pinned" {
			require.Contains(t, instr, "claude-sonnet-4")
			require.Contains(t, instr, "source=worker")
			found = true
		}
	}
	require.True(t, found, "a worker-sourced route must also be spelled out for the Leader")
}

func TestResolveRuntimeRoutePrefersTaskAndReportsSource(t *testing.T) {
	t.Parallel()

	taskRoute := map[string]string{MetaRuntimeProvider: "deepseek", MetaRuntimeModel: "deepseek-v4.1-flash"}
	workerRoute := map[string]string{MetaRuntimeProvider: "anthropic", MetaRuntimeModel: "claude-sonnet-4"}

	tests := []struct {
		name         string
		task, worker map[string]string
		wantSource   string
		wantProvider string
		wantModel    string
	}{
		{name: "task wins", task: taskRoute, worker: workerRoute, wantSource: RouteSourceTask, wantProvider: "deepseek", wantModel: "deepseek-v4.1-flash"},
		{name: "worker fallback", worker: workerRoute, wantSource: RouteSourceWorker, wantProvider: "anthropic", wantModel: "claude-sonnet-4"},
		{name: "task only", task: taskRoute, wantSource: RouteSourceTask, wantProvider: "deepseek", wantModel: "deepseek-v4.1-flash"},
		{name: "neither", wantSource: RouteSourceUnset},
		{name: "nil maps", task: nil, worker: nil, wantSource: RouteSourceUnset},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			provider, model, source := resolveRuntimeRoute(tc.task, tc.worker)
			require.Equal(t, tc.wantSource, source)
			require.Equal(t, tc.wantProvider, provider)
			require.Equal(t, tc.wantModel, model)
		})
	}
}

// TestPrepareStartThenStartEnforcesDeclaredRouteEndToEnd walks the real MCP
// surface: declaring a route, reading the briefing, then starting. It guards the
// handler -> engine wiring, which briefing-only unit tests cannot cover.
func TestPrepareStartThenStartEnforcesDeclaredRouteEndToEnd(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	seedBriefingRoute(t, srv, "T-e2e", nil,
		map[string]string{MetaRuntimeProvider: "deepseek", MetaRuntimeModel: "deepseek-v4.1-flash"},
	)

	briefing := prepareStartBriefing(t, srv, "T-e2e")
	ticket, ok := briefing["launch_ticket"].(string)
	require.True(t, ok)
	require.NotEmpty(t, ticket)

	startWith := func(model string) (map[string]any, error) {
		return srv.Handle(context.Background(), "task_transition", map[string]any{
			"namespace_id": "ns-1",
			"task_id":      "T-e2e",
			"transition":   "start",
			"actor_role":   "leader",
			"metadata": map[string]any{
				"launch.ticket":    ticket,
				"worker_agent_id":  "agent-e2e",
				"runtime.provider": "deepseek",
				"runtime.status":   "started",
				"runtime.model":    model,
			},
		})
	}

	// A mismatching model is rejected, and the rejection names the declared value.
	_, err := startWith("some-other-model")
	require.Error(t, err)
	require.Contains(t, err.Error(), "deepseek-v4.1-flash")

	// The rejected start must not have consumed the ticket, so the correct
	// declaration can still start the task.
	result, err := startWith("deepseek-v4.1-flash")
	require.NoError(t, err)
	require.Equal(t, "executing", result["state"])
}
