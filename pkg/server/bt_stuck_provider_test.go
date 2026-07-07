package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/toustifer/agentflow/pkg/engine"
)

func TestReportStuckOnceReturnsAssignedTaskSnapshot(t *testing.T) {
	t.Parallel()

	eng, err := engine.NewEngine(engine.NewEngineConfig{})
	require.NoError(t, err)
	defer eng.Close()

	srv, err := New(eng, Config{})
	require.NoError(t, err)

	_, err = eng.CreateNamespace(context.Background(), engine.CreateNamespaceRequest{ID: "ns-1", Name: "test"})
	require.NoError(t, err)
	_, err = eng.RegisterWorker(context.Background(), engine.RegisterWorkerRequest{
		NamespaceID:    "ns-1",
		ID:             "worker-a",
		Name:           "Worker A",
		PromptTemplate: "Task {task_id}",
		RecoveryPolicy: []string{"search_docs", "deeper_research", "escalate"},
		FallbackMCP:    []string{"find_knowledge", "find_pitfalls"},
		StuckPlaybook:  "Try docs first, then deeper research, then escalate.",
		EscalationMode: "leader_then_user",
	})
	require.NoError(t, err)
	_, err = eng.CreateTask(context.Background(), engine.CreateTaskRequest{NamespaceID: "ns-1", ID: "T1", Title: "task 1", AssignedWorker: "worker-a"})
	require.NoError(t, err)

	resp, err := srv.reportStuckOnce(context.Background(), "ns-1", "T1")
	require.NoError(t, err)
	require.Equal(t, "T1", resp.TaskID)
	require.Equal(t, "assigned", resp.State)
	require.Equal(t, []string{"start"}, resp.AvailableTransitions)
	require.Equal(t, []string{"task_get", "worker_prompt_get", "worker_handbook_get", "find_knowledge", "find_pitfalls"}, resp.SuggestedActions)
	require.Equal(t, map[string]any{"total": 0, "dependency": 0, "worker": 0}, resp.BlockerSummary)
	require.Equal(t, []string{"search_docs", "deeper_research", "escalate"}, resp.RecoveryPolicy)
	require.Equal(t, []string{"find_knowledge", "find_pitfalls"}, resp.FallbackMCP)
	require.Equal(t, "Try docs first, then deeper research, then escalate.", resp.StuckPlaybook)
	require.Equal(t, "leader_then_user", resp.EscalationMode)
	require.Equal(t, true, resp.OwnershipExpected)
	require.Equal(t, true, resp.ReassignmentExceptional)
}

func TestReportStuckOnceReturnsReworkNeededTaskSnapshot(t *testing.T) {
	t.Parallel()

	eng, err := engine.NewEngine(engine.NewEngineConfig{})
	require.NoError(t, err)
	defer eng.Close()

	srv, err := New(eng, Config{})
	require.NoError(t, err)

	_, err = eng.CreateNamespace(context.Background(), engine.CreateNamespaceRequest{ID: "ns-1", Name: "test"})
	require.NoError(t, err)
	_, err = eng.RegisterWorker(context.Background(), engine.RegisterWorkerRequest{
		NamespaceID:    "ns-1",
		ID:             "worker-a",
		Name:           "Worker A",
		PromptTemplate: "Task {task_id}",
		RecoveryPolicy: []string{"search_docs", "deeper_research", "escalate"},
		FallbackMCP:    []string{"find_knowledge", "find_pitfalls"},
		StuckPlaybook:  "Try docs first, then deeper research, then escalate.",
		EscalationMode: "leader_then_user",
	})
	require.NoError(t, err)
	_, err = eng.CreateTask(context.Background(), engine.CreateTaskRequest{NamespaceID: "ns-1", ID: "T1", Title: "task 1", AssignedWorker: "worker-a"})
	require.NoError(t, err)
	_, err = eng.TransitionTask(context.Background(), "ns-1", "T1", engine.TransStart, map[string]string{"actor_role": "leader"})
	require.NoError(t, err)
	_, err = eng.TransitionTask(context.Background(), "ns-1", "T1", engine.TransSubmit, map[string]string{"actor_role": "worker"})
	require.NoError(t, err)
	_, err = eng.TransitionTask(context.Background(), "ns-1", "T1", engine.TransRework, map[string]string{"actor_role": "reviewer"})
	require.NoError(t, err)

	resp, err := srv.reportStuckOnce(context.Background(), "ns-1", "T1")
	require.NoError(t, err)
	require.Equal(t, "rework_needed", resp.State)
	require.Equal(t, []string{"resume", "reassign", "cancel"}, resp.AvailableTransitions)
	require.Equal(t, []string{"search_docs", "deeper_research", "escalate"}, resp.RecoveryPolicy)
	require.Equal(t, []string{"find_knowledge", "find_pitfalls"}, resp.FallbackMCP)
}

func TestReportStuckOnceRejectsExecutingTask(t *testing.T) {
	t.Parallel()

	repoPath := initTestGitRepo(t)
	eng, err := engine.NewEngine(engine.NewEngineConfig{})
	require.NoError(t, err)
	defer eng.Close()

	srv, err := New(eng, Config{})
	require.NoError(t, err)

	_, err = eng.CreateNamespace(context.Background(), engine.CreateNamespaceRequest{
		ID:   "ns-1",
		Name: "test",
		Metadata: map[string]string{
			"workdir": repoPath,
		},
	})
	require.NoError(t, err)
	_, err = eng.RegisterWorker(context.Background(), engine.RegisterWorkerRequest{NamespaceID: "ns-1", ID: "worker-a", Name: "Worker A", PromptTemplate: "Task {task_id}", RecoveryPolicy: []string{"search_docs"}, FallbackMCP: []string{"find_knowledge"}, StuckPlaybook: "Investigate before escalating.", EscalationMode: "leader_first"})
	require.NoError(t, err)
	_, err = eng.CreateDAG(context.Background(), engine.CreateDAGRequest{NamespaceID: "ns-1", ID: "dag-1", Title: "DAG 1", Branch: "feat/test"})
	require.NoError(t, err)
	_, err = eng.CreateTask(context.Background(), engine.CreateTaskRequest{NamespaceID: "ns-1", ID: "T1", Title: "task 1", AssignedWorker: "worker-a", DAGID: "dag-1"})
	require.NoError(t, err)
	_, err = srv.dispatchTaskOnce(context.Background(), "ns-1", "T1")
	require.NoError(t, err)

	_, err = srv.reportStuckOnce(context.Background(), "ns-1", "T1")
	require.Error(t, err)
	require.Contains(t, err.Error(), "state")
}

func TestReportStuckOnceIncludesProjectBlockers(t *testing.T) {
	t.Parallel()

	eng, err := engine.NewEngine(engine.NewEngineConfig{})
	require.NoError(t, err)
	defer eng.Close()

	srv, err := New(eng, Config{})
	require.NoError(t, err)

	_, err = eng.CreateNamespace(context.Background(), engine.CreateNamespaceRequest{ID: "ns-1", Name: "test"})
	require.NoError(t, err)
	_, err = eng.RegisterWorker(context.Background(), engine.RegisterWorkerRequest{NamespaceID: "ns-1", ID: "worker-a", Name: "Worker A", PromptTemplate: "Task {task_id}", RecoveryPolicy: []string{"search_docs"}, FallbackMCP: []string{"find_knowledge"}, StuckPlaybook: "Investigate before escalating.", EscalationMode: "leader_first"})
	require.NoError(t, err)
	_, err = eng.CreateTask(context.Background(), engine.CreateTaskRequest{NamespaceID: "ns-1", ID: "T1", Title: "task 1", DependsOn: []string{"T0"}, AssignedWorker: "worker-a"})
	require.NoError(t, err)

	resp, err := srv.reportStuckOnce(context.Background(), "ns-1", "T1")
	require.NoError(t, err)
	require.Len(t, resp.Blockers, 1)
	require.Equal(t, "dependency", resp.Blockers[0]["type"])
	require.Equal(t, "T0", resp.Blockers[0]["blocked_by"])
	require.Equal(t, map[string]any{"total": 1, "dependency": 1, "worker": 0}, resp.BlockerSummary)
	require.Equal(t, []string{"task_get", "worker_prompt_get", "worker_handbook_get", "find_knowledge", "find_pitfalls", "project_blockers"}, resp.SuggestedActions)
	require.Equal(t, []string{"search_docs"}, resp.RecoveryPolicy)
	require.Equal(t, []string{"find_knowledge"}, resp.FallbackMCP)
}

func TestReportStuckProviderHandlesValidationAndTaskNotFound(t *testing.T) {
	t.Parallel()

	eng, err := engine.NewEngine(engine.NewEngineConfig{})
	require.NoError(t, err)
	defer eng.Close()

	srv, err := New(eng, Config{})
	require.NoError(t, err)
	provider, err := newBTStuckProvider(srv)
	require.NoError(t, err)
	defer provider.close()

	req := httptest.NewRequest(http.MethodPost, "/report-stuck", nil)
	req.Header.Set("X-Agentflow-BT-Token", provider.token)
	rec := httptest.NewRecorder()
	provider.handleReportStuck(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)

	req = httptest.NewRequest(http.MethodPost, "/report-stuck", nil)
	req.Header.Set("X-Agentflow-BT-Token", "wrong")
	rec = httptest.NewRecorder()
	provider.handleReportStuck(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)

	_, err = eng.CreateNamespace(context.Background(), engine.CreateNamespaceRequest{ID: "ns-1", Name: "test"})
	require.NoError(t, err)

	req = httptest.NewRequest(http.MethodPost, "/report-stuck", strings.NewReader(`{"namespace_id":"ns-1","task_id":"missing"}`))
	req.Header.Set("X-Agentflow-BT-Token", provider.token)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	provider.handleReportStuck(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code)
}
