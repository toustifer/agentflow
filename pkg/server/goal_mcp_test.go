package server

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
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
