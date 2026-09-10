package server

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/toustifer/agentflow/pkg/engine"
)

func TestGoalMCPHandlersLifecycle(t *testing.T) {
	srv := newTestServer(t)
	ctx := context.Background()

	// 1. goal_create
	createRes, err := srv.Handle(ctx, "goal_create", map[string]any{
		"namespace_id": "ns-1",
		"title":        "Optimize Query Engine",
		"description":  "Reduce SQL latency",
		"priority":     float64(10),
		"tags":         []any{"perf", "db"},
	})
	require.NoError(t, err)
	goalMap := createRes["goal"].(map[string]any)
	goalID := goalMap["id"].(string)
	require.Equal(t, "G-1", goalID)

	// 2. goal_get
	getRes, err := srv.Handle(ctx, "goal_get", map[string]any{
		"namespace_id": "ns-1",
		"goal_id":      goalID,
	})
	require.NoError(t, err)
	require.Equal(t, "Optimize Query Engine", getRes["goal"].(map[string]any)["title"])

	// 3. goal_update
	updateRes, err := srv.Handle(ctx, "goal_update", map[string]any{
		"namespace_id": "ns-1",
		"goal_id":      goalID,
		"status":       "deferred",
		"priority":     float64(20),
	})
	require.NoError(t, err)
	require.Equal(t, "deferred", updateRes["goal"].(map[string]any)["status"])

	// 4. goal_list
	listRes, err := srv.Handle(ctx, "goal_list", map[string]any{
		"namespace_id": "ns-1",
	})
	require.NoError(t, err)
	goals := listRes["goals"].([]any)
	require.Len(t, goals, 1)

	// 5. goal_promote
	promoteRes, err := srv.Handle(ctx, "goal_promote", map[string]any{
		"namespace_id": "ns-1",
		"goal_id":      goalID,
	})
	require.NoError(t, err)
	promotedGoal := promoteRes["goal"].(map[string]any)
	require.Equal(t, "promoted", promotedGoal["status"])
	createdDAG := promoteRes["dag"].(map[string]any)
	require.Equal(t, "dag-G-1", createdDAG["id"])
}

func TestProjectInspectIncludesBacklogSummary(t *testing.T) {
	srv := newTestServer(t)
	ctx := context.Background()

	// Create 2 goals: 1 pending, 1 deferred
	_, err := srv.Handle(ctx, "goal_create", map[string]any{
		"namespace_id": "ns-1",
		"title":        "Pending Goal",
		"priority":     float64(5),
	})
	require.NoError(t, err)

	g2, err := srv.Handle(ctx, "goal_create", map[string]any{
		"namespace_id": "ns-1",
		"title":        "Deferred Goal",
		"priority":     float64(1),
	})
	require.NoError(t, err)

	_, err = srv.Handle(ctx, "goal_update", map[string]any{
		"namespace_id": "ns-1",
		"goal_id":      g2["goal"].(map[string]any)["id"],
		"status":       "deferred",
	})
	require.NoError(t, err)

	// Inspect
	res, err := srv.Handle(ctx, "project_inspect", map[string]any{"namespace_id": "ns-1"})
	require.NoError(t, err)

	summary := res["summary"].(map[string]any)
	require.Equal(t, 2, summary["backlog_count"])

	backlog := res["backlog"].([]any)
	require.Len(t, backlog, 2)
}

func TestProjectNextStepsRecommendsBacklogPromotionWhenDone(t *testing.T) {
	srv := newTestServer(t)
	ctx := context.Background()

	_, err := srv.engine.RegisterWorker(ctx, engine.RegisterWorkerRequest{
		NamespaceID:    "ns-1",
		ID:             "worker-1",
		Name:           "Worker 1",
		PromptTemplate: "Do work",
	})
	require.NoError(t, err)

	_, err = srv.engine.CreateDAG(ctx, engine.CreateDAGRequest{
		NamespaceID:     "ns-1",
		ID:              "dag-1",
		Title:           "DAG 1",
		ExecutionBranch: "feat/dag-1",
	})
	require.NoError(t, err)

	_, err = srv.engine.CreateTask(ctx, engine.CreateTaskRequest{
		NamespaceID:    "ns-1",
		ID:             "T1",
		DAGID:          "dag-1",
		Title:          "Task 1",
		AssignedWorker: "worker-1",
	})
	require.NoError(t, err)

	// Transition task to done
	_, err = srv.engine.TransitionTask(ctx, "ns-1", "T1", engine.TransStart, nil)
	require.NoError(t, err)
	_, err = srv.engine.TransitionTask(ctx, "ns-1", "T1", engine.TransSubmit, nil)
	require.NoError(t, err)
	_, err = srv.engine.TransitionTask(ctx, "ns-1", "T1", engine.TransPass, nil)
	require.NoError(t, err)

	// Create pending goal in backlog
	_, err = srv.Handle(ctx, "goal_create", map[string]any{
		"namespace_id": "ns-1",
		"title":        "Next Big Feature",
		"priority":     float64(50),
	})
	require.NoError(t, err)

	// Check project_next_steps
	res, err := srv.Handle(ctx, "project_next_steps", map[string]any{
		"namespace_id": "ns-1",
		"dag_id":       "dag-1",
	})
	require.NoError(t, err)

	actions := res["actions"].([]string)
	hasGoalPromote := false
	for _, a := range actions {
		if a == "goal_promote" {
			hasGoalPromote = true
			break
		}
	}
	require.True(t, hasGoalPromote, "expected goal_promote in actions: %v", actions)
}


