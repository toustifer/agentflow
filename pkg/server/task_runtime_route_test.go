package server

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// taskMeta extracts the projected task metadata, which taskToMap renders as
// map[string]any rather than the raw map[string]string.
func taskMeta(t *testing.T, payload map[string]any) map[string]any {
	t.Helper()
	meta, ok := payload["metadata"].(map[string]any)
	require.True(t, ok, "task payload should carry metadata")
	return meta
}

func createTaskWithRoute(t *testing.T, srv *Server, taskID string, extra map[string]any) (map[string]any, error) {
	t.Helper()
	input := map[string]any{
		"namespace_id": "ns-1",
		"task_id":      taskID,
		"title":        "Route task",
	}
	for k, v := range extra {
		input[k] = v
	}
	return srv.Handle(context.Background(), "task_create", input)
}

func TestTaskCreateDeclaresRuntimeRoute(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)

	result, err := createTaskWithRoute(t, srv, "T-route", map[string]any{
		"provider": "deepseek",
		"model":    "deepseek-v4.1-flash",
	})
	require.NoError(t, err)

	meta := taskMeta(t, result)
	require.Equal(t, "deepseek", meta[MetaRuntimeProvider])
	require.Equal(t, "deepseek-v4.1-flash", meta[MetaRuntimeModel])

	// task_get must round-trip the declaration.
	got, err := srv.Handle(context.Background(), "task_get", map[string]any{
		"namespace_id": "ns-1",
		"task_id":      "T-route",
	})
	require.NoError(t, err)
	gotMeta := taskMeta(t, got)
	require.Equal(t, "deepseek", gotMeta[MetaRuntimeProvider])
	require.Equal(t, "deepseek-v4.1-flash", gotMeta[MetaRuntimeModel])
}

func TestTaskCreateDeclaresRuntimeRouteAlongsideMetadata(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)

	result, err := createTaskWithRoute(t, srv, "T-route-meta", map[string]any{
		"metadata": map[string]any{"env": "prod"},
		"provider": "deepseek",
		"model":    "deepseek-v4.1-flash",
	})
	require.NoError(t, err)

	meta := taskMeta(t, result)
	require.Equal(t, "prod", meta["env"], "route declaration must not drop existing metadata")
	require.Equal(t, "deepseek", meta[MetaRuntimeProvider])
	require.Equal(t, "deepseek-v4.1-flash", meta[MetaRuntimeModel])
}

func TestTaskCreateRejectsPartialRuntimeRoute(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		extra map[string]any
	}{
		{name: "provider only", extra: map[string]any{"provider": "deepseek"}},
		{name: "model only", extra: map[string]any{"model": "deepseek-v4.1-flash"}},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := newTestServer(t)

			_, err := createTaskWithRoute(t, srv, "T-partial", tc.extra)
			require.Error(t, err)
			require.Contains(t, err.Error(), "together")

			// Nothing may be persisted by a rejected declaration.
			_, err = srv.Handle(context.Background(), "task_get", map[string]any{
				"namespace_id": "ns-1",
				"task_id":      "T-partial",
			})
			require.Error(t, err)
		})
	}
}

func TestTaskCreateWithoutRuntimeRouteKeepsExistingBehaviour(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)

	result, err := createTaskWithRoute(t, srv, "T-plain", map[string]any{
		"metadata": map[string]any{"env": "prod"},
	})
	require.NoError(t, err)
	require.Equal(t, "Route task", result["title"])

	meta := taskMeta(t, result)
	require.Equal(t, map[string]any{"env": "prod"}, meta)
}

func TestTaskCreateBatchAllowsDistinctRoutesPerItem(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)

	result, err := srv.Handle(context.Background(), "task_create_batch", map[string]any{
		"namespace_id": "ns-1",
		"tasks": []any{
			map[string]any{
				"task_id": "T-a", "title": "A",
				"provider": "deepseek", "model": "deepseek-v4.1-flash",
			},
			map[string]any{
				"task_id": "T-b", "title": "B",
				"provider": "anthropic", "model": "claude-sonnet-4",
			},
		},
	})
	require.NoError(t, err)

	tasks, ok := result["tasks"].([]any)
	require.True(t, ok)
	require.Len(t, tasks, 2)

	byID := map[string]map[string]any{}
	for _, raw := range tasks {
		m, ok := raw.(map[string]any)
		require.True(t, ok)
		byID[m["id"].(string)] = taskMeta(t, m)
	}

	require.Equal(t, "deepseek", byID["T-a"][MetaRuntimeProvider])
	require.Equal(t, "deepseek-v4.1-flash", byID["T-a"][MetaRuntimeModel])
	require.Equal(t, "anthropic", byID["T-b"][MetaRuntimeProvider])
	require.Equal(t, "claude-sonnet-4", byID["T-b"][MetaRuntimeModel])
}

func TestTaskCreateBatchRejectsPartialItemRoute(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)

	_, err := srv.Handle(context.Background(), "task_create_batch", map[string]any{
		"namespace_id": "ns-1",
		"tasks": []any{
			map[string]any{
				"task_id": "T-ok", "title": "OK",
				"provider": "deepseek", "model": "deepseek-v4.1-flash",
			},
			map[string]any{
				"task_id": "T-bad", "title": "Bad",
				"provider": "anthropic",
			},
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "tasks[1]")
	require.Contains(t, err.Error(), "together")

	// The batch must not be partially applied.
	for _, id := range []string{"T-ok", "T-bad"} {
		_, err := srv.Handle(context.Background(), "task_get", map[string]any{
			"namespace_id": "ns-1",
			"task_id":      id,
		})
		require.Error(t, err, "task %s must not exist after a rejected batch", id)
	}
}

func TestTaskCreateBatchPerItemRouteOverridesBatchFallback(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)

	result, err := srv.Handle(context.Background(), "task_create_batch", map[string]any{
		"namespace_id": "ns-1",
		"provider":     "deepseek",
		"model":        "deepseek-v4.1-flash",
		"tasks": []any{
			map[string]any{"task_id": "T-inherit", "title": "Inherits batch route"},
			map[string]any{
				"task_id": "T-override", "title": "Overrides batch route",
				"provider": "anthropic", "model": "claude-sonnet-4",
			},
		},
	})
	require.NoError(t, err)

	tasks, ok := result["tasks"].([]any)
	require.True(t, ok)
	byID := map[string]map[string]any{}
	for _, raw := range tasks {
		m := raw.(map[string]any)
		byID[m["id"].(string)] = taskMeta(t, m)
	}

	require.Equal(t, "deepseek", byID["T-inherit"][MetaRuntimeProvider])
	require.Equal(t, "deepseek-v4.1-flash", byID["T-inherit"][MetaRuntimeModel])
	require.Equal(t, "anthropic", byID["T-override"][MetaRuntimeProvider])
	require.Equal(t, "claude-sonnet-4", byID["T-override"][MetaRuntimeModel])
}

func TestTaskCreateBatchRejectsPartialBatchRoute(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)

	_, err := srv.Handle(context.Background(), "task_create_batch", map[string]any{
		"namespace_id": "ns-1",
		"model":        "deepseek-v4.1-flash",
		"tasks": []any{
			map[string]any{"task_id": "T-x", "title": "X"},
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "together")
}

func TestTaskCreateBatchWithoutRouteKeepsExistingBehaviour(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)

	result, err := srv.Handle(context.Background(), "task_create_batch", map[string]any{
		"namespace_id": "ns-1",
		"tasks": []any{
			map[string]any{
				"task_id": "T-plain", "title": "Plain",
				"metadata": map[string]any{"env": "prod"},
			},
		},
	})
	require.NoError(t, err)

	tasks := result["tasks"].([]any)
	require.Len(t, tasks, 1)
	require.Equal(t, map[string]any{"env": "prod"}, taskMeta(t, tasks[0].(map[string]any)))
}

func TestTaskRuntimeRouteSchemaExposesProviderAndModel(t *testing.T) {
	t.Parallel()

	stringType := func(props map[string]any, key, tool string) {
		prop, ok := props[key].(map[string]any)
		require.True(t, ok, "%s must expose %s", tool, key)
		require.Equal(t, "string", prop["type"])
	}

	// task_create: single flat layer.
	props := toolInputSchema("task_create")["properties"].(map[string]any)
	stringType(props, "provider", "task_create")
	stringType(props, "model", "task_create")

	// task_create_batch: outer tool params AND the inline items schema.
	batchProps := toolInputSchema("task_create_batch")["properties"].(map[string]any)
	stringType(batchProps, "provider", "task_create_batch outer")
	stringType(batchProps, "model", "task_create_batch outer")

	items := batchProps["tasks"].(map[string]any)["items"].(map[string]any)
	itemProps := items["properties"].(map[string]any)
	stringType(itemProps, "provider", "task_create_batch items")
	stringType(itemProps, "model", "task_create_batch items")
}
