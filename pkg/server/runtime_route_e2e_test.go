package server

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRuntimeRouteFullLifecycleEndToEnd walks the whole feature through the real
// MCP surface — task_create (declaration) -> prepare_start (briefing) -> start —
// and then repeats the prepare/start cycle AFTER a reassign to prove the model
// constraint survives the standard Worker-recovery path.
//
// This is the end-to-end guard for the route.*/runtime.* split: unit tests on
// either side can pass while the handler->engine wiring is broken.
func TestRuntimeRouteFullLifecycleEndToEnd(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)

	const taskID = "T-lifecycle"
	const declaredProvider = "deepseek"
	const declaredModel = "deepseek-v4.1-flash"

	// 1. Bootstrap the DAG and Worker through MCP.
	_, err := srv.Handle(context.Background(), "dag_create", map[string]any{
		"namespace_id": "ns-1",
		"dag_id":       "dag-life",
		"title":        "Lifecycle DAG",
		"branch":       "feat/lifecycle",
	})
	require.NoError(t, err)

	_, err = srv.Handle(context.Background(), "worker_register", map[string]any{
		"namespace_id":    "ns-1",
		"worker_id":       "worker-life",
		"name":            "Lifecycle Worker",
		"prompt_template": "Task {task_id} in {worktree_path} on {branch}",
	})
	require.NoError(t, err)

	// 2. task_create carries the DECLARATION.
	created, err := srv.Handle(context.Background(), "task_create", map[string]any{
		"namespace_id":    "ns-1",
		"task_id":         taskID,
		"title":           "lifecycle",
		"assigned_worker": "worker-life",
		"dag_id":          "dag-life",
		"provider":        declaredProvider,
		"model":           declaredModel,
	})
	require.NoError(t, err)

	createdMeta := taskMeta(t, created)
	require.Equal(t, declaredProvider, createdMeta[MetaRouteProvider], "declaration must land in route.*")
	require.Equal(t, declaredModel, createdMeta[MetaRouteModel])
	require.NotContains(t, createdMeta, MetaRuntimeProvider, "create must not write the observation namespace")

	// 3. First prepare_start: briefing must expose declaration, observation and
	//    source as three distinguishable values.
	first := prepareStartBriefing(t, srv, taskID)
	require.Equal(t, RouteSourceTask, first["route_source"])
	require.Equal(t, declaredProvider, first["route.provider"])
	require.Equal(t, declaredModel, first["route.model"])
	require.Equal(t, "", first["runtime.provider"], "nothing has started yet")
	require.Equal(t, "", first["runtime.model"])

	firstTicket := prepareStartTicket(t, srv, taskID)

	// 4. Start with the declared route reported verbatim -> succeeds, and the
	//    observation is recorded separately from the declaration.
	started, err := startTaskWith(t, srv, taskID, firstTicket, declaredProvider, declaredModel)
	require.NoError(t, err)
	require.Equal(t, "executing", started["state"])
	require.Equal(t, declaredModel, started["observed_route"].(map[string]any)["model"])
	require.Equal(t, declaredModel, started["declared_route"].(map[string]any)["model"])

	// 5. The standard Worker-recovery path.
	_, err = srv.Handle(context.Background(), "task_transition", map[string]any{
		"namespace_id": "ns-1",
		"task_id":      taskID,
		"transition":   "reassign",
		"actor_role":   "leader",
	})
	require.NoError(t, err)

	// 6. Re-prepare after the reassign: the SAME declaration must still resolve,
	//    and the stale observation must be gone.
	second := prepareStartBriefing(t, srv, taskID)
	require.Equal(t, RouteSourceTask, second["route_source"], "declaration must survive a reassign")
	require.Equal(t, declaredProvider, second["route.provider"])
	require.Equal(t, declaredModel, second["route.model"])
	require.Equal(t, "", second["runtime.provider"], "reassign clears the stale observation")
	require.Equal(t, "", second["runtime.model"])
	require.NotEmpty(t, findRouteInstruction(t, second),
		"the Leader must still be told to report the pinned route after a reassign")

	secondTicket := prepareStartTicket(t, srv, taskID)

	// 7. The constraint still bites after the reassign.
	_, err = startTaskWith(t, srv, taskID, secondTicket, declaredProvider, "some-other-model")
	require.Error(t, err)
	require.Contains(t, err.Error(), declaredModel)

	// 8. ...and the declared route still starts the replacement Worker.
	final, err := startTaskWith(t, srv, taskID, secondTicket, declaredProvider, declaredModel)
	require.NoError(t, err)
	require.Equal(t, "executing", final["state"])
}
