package server

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDAGPriorityMCPIntegration(t *testing.T) {
	srv := newTestServer(t)
	ctx := context.Background()

	// 1. dag_create with priority
	createRes, err := srv.Handle(ctx, "dag_create", map[string]any{
		"namespace_id":     "ns-1",
		"dag_id":           "dag-crit",
		"title":            "Critical Bug",
		"priority":         "p0",
		"execution_branch": "feat/crit",
	})
	require.NoError(t, err)
	require.Equal(t, "P0", createRes["priority"])

	// 2. dag_update priority
	updateRes, err := srv.Handle(ctx, "dag_update", map[string]any{
		"namespace_id": "ns-1",
		"dag_id":       "dag-crit",
		"priority":     "P1",
	})
	require.NoError(t, err)
	require.Equal(t, "P1", updateRes["priority"])

	// 3. goal_promote with priority
	_, err = srv.Handle(ctx, "goal_create", map[string]any{
		"namespace_id": "ns-1",
		"goal_id":      "G-urgent",
		"title":        "Urgent Goal",
	})
	require.NoError(t, err)

	promRes, err := srv.Handle(ctx, "goal_promote", map[string]any{
		"namespace_id": "ns-1",
		"goal_id":      "G-urgent",
		"priority":     "P0",
	})
	require.NoError(t, err)
	promDAG := promRes["dag"].(map[string]any)
	require.Equal(t, "P0", promDAG["priority"])
}

func TestDAGPrioritySchedulingAndInspect(t *testing.T) {
	srv := newTestServer(t)
	ctx := context.Background()

	_, err := srv.Handle(ctx, "dag_create", map[string]any{
		"namespace_id":     "ns-1",
		"dag_id":           "dag-low",
		"title":            "Low Priority DAG",
		"priority":         "P3",
		"execution_branch": "feat/low",
	})
	require.NoError(t, err)

	_, err = srv.Handle(ctx, "dag_create", map[string]any{
		"namespace_id":     "ns-1",
		"dag_id":           "dag-high",
		"title":            "High Priority DAG",
		"priority":         "P0",
		"execution_branch": "feat/high",
	})
	require.NoError(t, err)

	// project_inspect
	insp, err := srv.Handle(ctx, "project_inspect", map[string]any{"namespace_id": "ns-1"})
	require.NoError(t, err)
	dags := insp["dags"].([]any)
	require.GreaterOrEqual(t, len(dags), 2)
	firstDAG := dags[0].(map[string]any)["dag"].(map[string]any)
	require.Equal(t, "dag-high", firstDAG["id"])
	require.Equal(t, "P0", firstDAG["priority"])
}

