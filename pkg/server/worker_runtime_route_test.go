package server

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// registerWorkerWithRoute is a small fixture: it registers worker-route with the
// given extra input keys layered on top of the minimal valid registration.
func registerWorkerWithRoute(t *testing.T, srv *Server, extra map[string]any) (map[string]any, error) {
	t.Helper()
	input := map[string]any{
		"namespace_id":    "ns-1",
		"worker_id":       "worker-route",
		"name":            "Route Worker",
		"prompt_template": "Task {task_id}",
	}
	for k, v := range extra {
		input[k] = v
	}
	return srv.Handle(context.Background(), "worker_register", input)
}

func TestWorkerRegisterDeclaresRuntimeRoute(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)

	result, err := registerWorkerWithRoute(t, srv, map[string]any{
		"provider": "deepseek",
		"model":    "deepseek-v4.1-flash",
	})
	require.NoError(t, err)

	meta, ok := result["metadata"].(map[string]string)
	require.True(t, ok, "worker_register should echo metadata")
	require.Equal(t, "deepseek", meta[MetaRouteProvider])
	require.Equal(t, "deepseek-v4.1-flash", meta[MetaRouteModel])

	// worker_get must round-trip the declaration.
	got, err := srv.Handle(context.Background(), "worker_get", map[string]any{
		"namespace_id": "ns-1",
		"worker_id":    "worker-route",
	})
	require.NoError(t, err)
	gotMeta, ok := got["metadata"].(map[string]string)
	require.True(t, ok, "worker_get should echo metadata")
	require.Equal(t, "deepseek", gotMeta[MetaRouteProvider])
	require.Equal(t, "deepseek-v4.1-flash", gotMeta[MetaRouteModel])
}

func TestWorkerRegisterDeclaresRuntimeRouteAlongsideMetadata(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)

	result, err := registerWorkerWithRoute(t, srv, map[string]any{
		"metadata": map[string]any{"env": "prod"},
		"provider": "deepseek",
		"model":    "deepseek-v4.1-flash",
	})
	require.NoError(t, err)

	meta, ok := result["metadata"].(map[string]string)
	require.True(t, ok)
	require.Equal(t, "prod", meta["env"])
	require.Equal(t, "deepseek", meta[MetaRouteProvider])
	require.Equal(t, "deepseek-v4.1-flash", meta[MetaRouteModel])
}

func TestWorkerRegisterRejectsPartialRuntimeRoute(t *testing.T) {
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

			_, err := registerWorkerWithRoute(t, srv, tc.extra)
			require.Error(t, err)
			require.Contains(t, err.Error(), "together")

			// The failed declaration must not create the worker.
			_, err = srv.Handle(context.Background(), "worker_get", map[string]any{
				"namespace_id": "ns-1",
				"worker_id":    "worker-route",
			})
			require.Error(t, err)
		})
	}
}

func TestWorkerRegisterEmptyRuntimeRouteIsUndeclared(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)

	result, err := registerWorkerWithRoute(t, srv, map[string]any{
		"provider": "",
		"model":    "",
	})
	require.NoError(t, err)

	if meta, ok := result["metadata"].(map[string]string); ok {
		_, hasProvider := meta[MetaRouteProvider]
		_, hasModel := meta[MetaRouteModel]
		require.False(t, hasProvider, "empty provider must not be persisted")
		require.False(t, hasModel, "empty model must not be persisted")
	}
}

func TestWorkerRegisterWithoutRuntimeRouteKeepsExistingBehaviour(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)

	result, err := registerWorkerWithRoute(t, srv, map[string]any{
		"metadata": map[string]any{"env": "prod"},
	})
	require.NoError(t, err)
	require.Equal(t, "Route Worker", result["name"])

	meta, ok := result["metadata"].(map[string]string)
	require.True(t, ok)
	require.Equal(t, map[string]string{"env": "prod"}, meta)
}

func TestWorkerUpdateDeclaresRuntimeRouteAndPreservesMetadata(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)

	_, err := registerWorkerWithRoute(t, srv, map[string]any{
		"metadata": map[string]any{"env": "prod"},
	})
	require.NoError(t, err)

	result, err := srv.Handle(context.Background(), "worker_update", map[string]any{
		"namespace_id": "ns-1",
		"worker_id":    "worker-route",
		"provider":     "deepseek",
		"model":        "deepseek-v4.1-flash",
	})
	require.NoError(t, err)

	meta, ok := result["metadata"].(map[string]string)
	require.True(t, ok)
	require.Equal(t, "prod", meta["env"], "declaring a route must not drop existing metadata")
	require.Equal(t, "deepseek", meta[MetaRouteProvider])
	require.Equal(t, "deepseek-v4.1-flash", meta[MetaRouteModel])
}

func TestWorkerUpdateRejectsPartialRuntimeRoute(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)

	_, err := registerWorkerWithRoute(t, srv, nil)
	require.NoError(t, err)

	_, err = srv.Handle(context.Background(), "worker_update", map[string]any{
		"namespace_id": "ns-1",
		"worker_id":    "worker-route",
		"model":        "deepseek-v4.1-flash",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "together")
}

func TestWorkerUpdateWithoutRuntimeRouteLeavesMetadataUntouched(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)

	_, err := registerWorkerWithRoute(t, srv, map[string]any{
		"metadata": map[string]any{"env": "prod"},
	})
	require.NoError(t, err)

	result, err := srv.Handle(context.Background(), "worker_update", map[string]any{
		"namespace_id": "ns-1",
		"worker_id":    "worker-route",
		"scope":        "updated scope",
	})
	require.NoError(t, err)

	meta, ok := result["metadata"].(map[string]string)
	require.True(t, ok)
	require.Equal(t, map[string]string{"env": "prod"}, meta)
}

func TestWorkerRuntimeRouteSchemaExposesProviderAndModel(t *testing.T) {
	t.Parallel()

	for _, tool := range []string{"worker_register", "worker_update"} {
		schema := toolInputSchema(tool)
		props, ok := schema["properties"].(map[string]any)
		require.True(t, ok)
		for _, key := range []string{"provider", "model"} {
			prop, ok := props[key].(map[string]any)
			require.True(t, ok, "%s must expose %s", tool, key)
			require.Equal(t, "string", prop["type"])
		}
	}
}
