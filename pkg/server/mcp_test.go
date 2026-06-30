package server

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stifer/agentflow/pkg/engine"
)

func TestToolRegistryIncludesCoreTools(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	tools := srv.Tools()
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}

	require.Contains(t, names, "namespace_create")
	require.Contains(t, names, "namespace_list")
	require.Contains(t, names, "task_create")
	require.Contains(t, names, "task_transition")
	require.Contains(t, names, "task_get")
	require.Contains(t, names, "task_list")
	require.Contains(t, names, "task_history")
	require.Contains(t, names, "flow_ping")
}

func TestTaskCreateHandlerCallsEngine(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)

	result, err := srv.Handle(context.Background(), "task_create", map[string]any{
		"namespace_id":    "ns-1",
		"task_id":         "T1",
		"title":           "bootstrap",
		"assigned_worker": "worker-ops",
	})
	require.NoError(t, err)
	require.Equal(t, "T1", result["id"])

	task, err := srv.engine.GetTask(context.Background(), "ns-1", "T1")
	require.NoError(t, err)
	require.Equal(t, "ns-1", task.NamespaceID)
	require.Equal(t, "bootstrap", task.Title)
	require.Equal(t, "worker-ops", task.AssignedWorker)
}

func TestTaskGetReturnsEngineTask(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	_, err := srv.engine.CreateTask(context.Background(), engine.CreateTaskRequest{
		NamespaceID:    "ns-1",
		ID:             "T-get",
		Title:          "inspect",
		AssignedWorker: "worker-a",
	})
	require.NoError(t, err)

	result, err := srv.Handle(context.Background(), "task_get", map[string]any{
		"namespace_id": "ns-1",
		"task_id":      "T-get",
	})
	require.NoError(t, err)
	require.Equal(t, "T-get", result["id"])
	require.Equal(t, "ns-1", result["namespace_id"])
	require.Equal(t, "inspect", result["title"])
	require.Equal(t, "assigned", result["state"])
}

func TestTaskTransitionAdvancesState(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	_, err := srv.engine.CreateTask(context.Background(), engine.CreateTaskRequest{
		NamespaceID:    "ns-1",
		ID:             "T-transition",
		Title:          "execute",
		AssignedWorker: "worker-b",
	})
	require.NoError(t, err)

	result, err := srv.Handle(context.Background(), "task_transition", map[string]any{
		"namespace_id": "ns-1",
		"task_id":      "T-transition",
		"transition":   "start",
		"metadata": map[string]any{
			"actor": "leader",
		},
	})
	require.NoError(t, err)
	require.Equal(t, "executing", result["state"])

	task, err := srv.engine.GetTask(context.Background(), "ns-1", "T-transition")
	require.NoError(t, err)
	require.Equal(t, engine.TaskExecuting, task.State)
}

func TestFlowPingReturnsSuccess(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)

	result, err := srv.Handle(context.Background(), "flow_ping", map[string]any{})
	require.NoError(t, err)
	require.Equal(t, map[string]any{"ok": true}, result)
}

func TestTaskGetListHistoryReturnCorrectResults(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	_, err := srv.engine.CreateTask(context.Background(), engine.CreateTaskRequest{
		NamespaceID:    "ns-1",
		ID:             "T-list-1",
		Title:          "first",
		AssignedWorker: "w1",
	})
	require.NoError(t, err)
	_, err = srv.engine.CreateTask(context.Background(), engine.CreateTaskRequest{
		NamespaceID:    "ns-1",
		ID:             "T-list-2",
		Title:          "second",
		AssignedWorker: "w2",
	})
	require.NoError(t, err)

	// task_list should return all tasks in the namespace
	listResult, err := srv.Handle(context.Background(), "task_list", map[string]any{
		"namespace_id": "ns-1",
	})
	require.NoError(t, err)
	items, ok := listResult["tasks"].([]any)
	require.True(t, ok)
	require.Len(t, items, 2)

	// task_history should return non-empty history for a created task
	historyResult, err := srv.Handle(context.Background(), "task_history", map[string]any{
		"namespace_id": "ns-1",
		"task_id":      "T-list-1",
	})
	require.NoError(t, err)
	historyItems, ok := historyResult["history"].([]any)
	require.True(t, ok)
	require.GreaterOrEqual(t, len(historyItems), 1)
}

func TestHubSyncFailureDoesNotFailFlowPing(t *testing.T) {
	t.Parallel()

	srv := newTestServerWithConfig(t, Config{HubEnabled: true})
	srv.hub = failingHubSyncer{err: errors.New("hub unavailable")}

	result, err := srv.Handle(context.Background(), "flow_ping", map[string]any{})
	require.NoError(t, err)
	require.Equal(t, map[string]any{"ok": true}, result)
}

func TestHubSyncFailureDoesNotFailNamespaceCreate(t *testing.T) {
	t.Parallel()

	srv := newTestServerWithConfig(t, Config{HubEnabled: true})
	srv.hub = failingHubSyncer{err: errors.New("hub unavailable")}

	result, err := srv.Handle(context.Background(), "namespace_create", map[string]any{
		"id":   "ns-2",
		"name": "namespace-sync-fallback",
	})
	require.NoError(t, err)
	require.Equal(t, "ns-2", result["id"])

	ns, err := srv.engine.GetNamespace(context.Background(), "ns-2")
	require.NoError(t, err)
	require.Equal(t, "namespace-sync-fallback", ns.Name)
}

func TestHubSyncFailureDoesNotFailTaskCreateAndTransition(t *testing.T) {
	t.Parallel()

	srv := newTestServerWithConfig(t, Config{HubEnabled: true})
	srv.hub = failingHubSyncer{err: errors.New("hub unavailable")}

	created, err := srv.Handle(context.Background(), "task_create", map[string]any{
		"namespace_id":    "ns-1",
		"task_id":         "T-sync",
		"title":           "sync fallback",
		"assigned_worker": "worker-sync",
	})
	require.NoError(t, err)
	require.Equal(t, "T-sync", created["id"])

	transitioned, err := srv.Handle(context.Background(), "task_transition", map[string]any{
		"namespace_id": "ns-1",
		"task_id":      "T-sync",
		"transition":   "start",
	})
	require.NoError(t, err)
	require.Equal(t, "executing", transitioned["state"])
}

func TestHubSyncFailureDoesNotFailTaskGet(t *testing.T) {
	t.Parallel()

	srv := newTestServerWithConfig(t, Config{HubEnabled: true})
	srv.hub = failingHubSyncer{err: errors.New("hub unavailable")}

	_, err := srv.engine.CreateTask(context.Background(), engine.CreateTaskRequest{
		NamespaceID:    "ns-1",
		ID:             "T-get-fail",
		Title:          "get with sync failure",
		AssignedWorker: "w",
	})
	require.NoError(t, err)

	result, err := srv.Handle(context.Background(), "task_get", map[string]any{
		"namespace_id": "ns-1",
		"task_id":      "T-get-fail",
	})
	require.NoError(t, err)
	require.Equal(t, "T-get-fail", result["id"])
	require.Equal(t, "get with sync failure", result["title"])
}

func TestHubSyncFailureDoesNotFailTaskList(t *testing.T) {
	t.Parallel()

	srv := newTestServerWithConfig(t, Config{HubEnabled: true})
	srv.hub = failingHubSyncer{err: errors.New("hub unavailable")}

	_, err := srv.engine.CreateTask(context.Background(), engine.CreateTaskRequest{
		NamespaceID:    "ns-1",
		ID:             "T-list-fail-1",
		Title:          "list sync fallback",
		AssignedWorker: "w",
	})
	require.NoError(t, err)

	result, err := srv.Handle(context.Background(), "task_list", map[string]any{
		"namespace_id": "ns-1",
	})
	require.NoError(t, err)
	items, ok := result["tasks"].([]any)
	require.True(t, ok)
	require.GreaterOrEqual(t, len(items), 1)
}

func TestHubSyncFailureDoesNotFailTaskHistory(t *testing.T) {
	t.Parallel()

	srv := newTestServerWithConfig(t, Config{HubEnabled: true})
	srv.hub = failingHubSyncer{err: errors.New("hub unavailable")}

	_, err := srv.engine.CreateTask(context.Background(), engine.CreateTaskRequest{
		NamespaceID:    "ns-1",
		ID:             "T-hist-fail",
		Title:          "history with sync failure",
		AssignedWorker: "w",
	})
	require.NoError(t, err)

	result, err := srv.Handle(context.Background(), "task_history", map[string]any{
		"namespace_id": "ns-1",
		"task_id":      "T-hist-fail",
	})
	require.NoError(t, err)
	items, ok := result["history"].([]any)
	require.True(t, ok)
	require.GreaterOrEqual(t, len(items), 1)
}

func TestTaskGetCallsSyncWithCorrectTask(t *testing.T) {
	t.Parallel()

	srv := newTestServerWithConfig(t, Config{HubEnabled: true})
	tracker := &trackingHubSyncer{}
	srv.hub = tracker

	_, err := srv.engine.CreateTask(context.Background(), engine.CreateTaskRequest{
		NamespaceID:    "ns-1",
		ID:             "T-sync-check",
		Title:          "sync verification",
		AssignedWorker: "w",
	})
	require.NoError(t, err)

	result, err := srv.Handle(context.Background(), "task_get", map[string]any{
		"namespace_id": "ns-1",
		"task_id":      "T-sync-check",
	})
	require.NoError(t, err)
	require.Equal(t, "T-sync-check", result["id"])

	// verify SyncTask was called with the correct task ID
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	require.Contains(t, tracker.syncedTasks, "T-sync-check")
}

func TestTaskListCallsSyncForEachTask(t *testing.T) {
	t.Parallel()

	srv := newTestServerWithConfig(t, Config{HubEnabled: true})
	tracker := &trackingHubSyncer{}
	srv.hub = tracker

	_, err := srv.engine.CreateTask(context.Background(), engine.CreateTaskRequest{
		NamespaceID:    "ns-1",
		ID:             "T-list-sync-1",
		Title:          "list sync A",
		AssignedWorker: "w",
	})
	require.NoError(t, err)
	_, err = srv.engine.CreateTask(context.Background(), engine.CreateTaskRequest{
		NamespaceID:    "ns-1",
		ID:             "T-list-sync-2",
		Title:          "list sync B",
		AssignedWorker: "w",
	})
	require.NoError(t, err)

	result, err := srv.Handle(context.Background(), "task_list", map[string]any{
		"namespace_id": "ns-1",
	})
	require.NoError(t, err)
	items, ok := result["tasks"].([]any)
	require.True(t, ok)
	require.Len(t, items, 2)

	// verify SyncTask was called for each task
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	require.Contains(t, tracker.syncedTasks, "T-list-sync-1")
	require.Contains(t, tracker.syncedTasks, "T-list-sync-2")
}

func newTestServer(t *testing.T) *Server {
	t.Helper()
	return newTestServerWithConfig(t, Config{})
}

func newTestServerWithConfig(t *testing.T, cfg Config) *Server {
	t.Helper()

	eng, err := engine.NewEngine(engine.NewEngineConfig{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, eng.Close()) })

	_, err = eng.CreateNamespace(context.Background(), engine.CreateNamespaceRequest{
		ID:   "ns-1",
		Name: "agent-company",
	})
	require.NoError(t, err)

	srv, err := New(eng, cfg)
	require.NoError(t, err)
	return srv
}

type failingHubSyncer struct {
	err error
}

func (f failingHubSyncer) SyncTask(context.Context, *engine.Task) error {
	return f.err
}

func (f failingHubSyncer) SyncNamespace(context.Context, *engine.Namespace) error {
	return f.err
}

func (f failingHubSyncer) Ping(context.Context) error {
	return f.err
}

// trackingHubSyncer records synced task IDs for verification in tests.
type trackingHubSyncer struct {
	mu           sync.Mutex
	syncedTasks  []string
}

func (t *trackingHubSyncer) SyncTask(_ context.Context, task *engine.Task) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.syncedTasks = append(t.syncedTasks, task.ID)
	return nil
}

func (t *trackingHubSyncer) SyncNamespace(_ context.Context, ns *engine.Namespace) error {
	return nil
}

func (t *trackingHubSyncer) Ping(_ context.Context) error {
	return nil
}
