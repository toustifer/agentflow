package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/toustifer/agentflow/pkg/engine"
)

func TestTaskSubmitForReviewOnceTransitionsToReviewPending(t *testing.T) {
	t.Parallel()

	repoPath := initTestGitRepo(t)
	eng, err := engine.NewEngine(engine.NewEngineConfig{})
	require.NoError(t, err)
	defer eng.Close()

	srv, err := New(eng, Config{})
	require.NoError(t, err)

	_, err = eng.CreateNamespace(context.Background(), engine.CreateNamespaceRequest{ID: "ns-1", Name: "test", Metadata: map[string]string{"workdir": repoPath}})
	require.NoError(t, err)
	_, err = eng.RegisterWorker(context.Background(), engine.RegisterWorkerRequest{NamespaceID: "ns-1", ID: "worker-a", Name: "Worker A", PromptTemplate: "Task {task_id}"})
	require.NoError(t, err)
	_, err = eng.CreateDAG(context.Background(), engine.CreateDAGRequest{NamespaceID: "ns-1", ID: "dag-1", Title: "DAG 1", Branch: "feat/test"})
	require.NoError(t, err)
	_, err = eng.CreateTask(context.Background(), engine.CreateTaskRequest{NamespaceID: "ns-1", ID: "T1", Title: "task 1", AssignedWorker: "worker-a", DAGID: "dag-1"})
	require.NoError(t, err)
	_, err = srv.enterWorktreeOnce(context.Background(), "ns-1", "T1", "worker-a")
	require.NoError(t, err)
	_, err = eng.TransitionTask(context.Background(), "ns-1", "T1", engine.TransStart, map[string]string{"actor_role": "leader"})
	require.NoError(t, err)
	_, err = srv.gitCommitChangesOnce(context.Background(), "ns-1", "T1", "worker-a")
	require.NoError(t, err)
	_, err = eng.WriteWorkerDiary(context.Background(), engine.WriteDiaryRequest{NamespaceID: "ns-1", WorkerID: "worker-a", Date: time.Now().UTC().Format("2006-01-02"), Content: "done"})
	require.NoError(t, err)

	resp, err := srv.taskSubmitForReviewOnce(context.Background(), "ns-1", "T1", "worker-a")
	require.NoError(t, err)
	require.Equal(t, "review_pending", resp.State)
	require.NotEmpty(t, resp.Review["commit"])
}

func TestTaskSubmitForReviewOnceRejectsMissingReviewCommit(t *testing.T) {
	t.Parallel()

	repoPath := initTestGitRepo(t)
	eng, err := engine.NewEngine(engine.NewEngineConfig{})
	require.NoError(t, err)
	defer eng.Close()

	srv, err := New(eng, Config{})
	require.NoError(t, err)

	_, err = eng.CreateNamespace(context.Background(), engine.CreateNamespaceRequest{ID: "ns-1", Name: "test", Metadata: map[string]string{"workdir": repoPath}})
	require.NoError(t, err)
	_, err = eng.RegisterWorker(context.Background(), engine.RegisterWorkerRequest{NamespaceID: "ns-1", ID: "worker-a", Name: "Worker A", PromptTemplate: "Task {task_id}"})
	require.NoError(t, err)
	_, err = eng.CreateDAG(context.Background(), engine.CreateDAGRequest{NamespaceID: "ns-1", ID: "dag-1", Title: "DAG 1", Branch: "feat/test"})
	require.NoError(t, err)
	_, err = eng.CreateTask(context.Background(), engine.CreateTaskRequest{NamespaceID: "ns-1", ID: "T1", Title: "task 1", AssignedWorker: "worker-a", DAGID: "dag-1"})
	require.NoError(t, err)
	_, err = eng.TransitionTask(context.Background(), "ns-1", "T1", engine.TransStart, map[string]string{"actor_role": "leader"})
	require.NoError(t, err)
	_, err = eng.WriteWorkerDiary(context.Background(), engine.WriteDiaryRequest{NamespaceID: "ns-1", WorkerID: "worker-a", Date: time.Now().UTC().Format("2006-01-02"), Content: "done"})
	require.NoError(t, err)

	_, err = srv.taskSubmitForReviewOnce(context.Background(), "ns-1", "T1", "worker-a")
	require.Error(t, err)
	require.Contains(t, err.Error(), "review.commit")
}

func TestTaskSubmitForReviewProviderHandlesValidation(t *testing.T) {
	t.Parallel()

	eng, err := engine.NewEngine(engine.NewEngineConfig{})
	require.NoError(t, err)
	defer eng.Close()

	srv, err := New(eng, Config{})
	require.NoError(t, err)
	provider, err := newBTSubmitForReviewProvider(srv)
	require.NoError(t, err)
	defer provider.close()

	req := httptest.NewRequest(http.MethodPost, "/task-submit-for-review", nil)
	req.Header.Set("X-Agentflow-BT-Token", provider.token)
	rec := httptest.NewRecorder()
	provider.handleTaskSubmitForReview(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)

	req = httptest.NewRequest(http.MethodPost, "/task-submit-for-review", nil)
	req.Header.Set("X-Agentflow-BT-Token", "wrong")
	rec = httptest.NewRecorder()
	provider.handleTaskSubmitForReview(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)

	_, err = eng.CreateNamespace(context.Background(), engine.CreateNamespaceRequest{ID: "ns-1", Name: "test"})
	require.NoError(t, err)
	req = httptest.NewRequest(http.MethodPost, "/task-submit-for-review", strings.NewReader(`{"namespace_id":"ns-1","task_id":"missing","worker_id":"worker-a"}`))
	req.Header.Set("X-Agentflow-BT-Token", provider.token)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	provider.handleTaskSubmitForReview(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code)
}
