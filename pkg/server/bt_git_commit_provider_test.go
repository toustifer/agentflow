package server

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/toustifer/agentflow/pkg/engine"
)

func TestGitCommitChangesOnceReturnsReviewMetadata(t *testing.T) {
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
	_, err = eng.CreateDAG(context.Background(), engine.CreateDAGRequest{NamespaceID: "ns-1", ID: "dag-1", Title: "DAG 1", ExecutionBranch: "feat/test"})
	require.NoError(t, err)
	_, err = eng.CreateTask(context.Background(), engine.CreateTaskRequest{NamespaceID: "ns-1", ID: "T1", Title: "task 1", AssignedWorker: "worker-a", DAGID: "dag-1"})
	require.NoError(t, err)
	_, err = srv.enterWorktreeOnce(context.Background(), "ns-1", "T1", "worker-a")
	require.NoError(t, err)

	resp, err := srv.gitCommitChangesOnce(context.Background(), "ns-1", "T1", "worker-a")
	require.NoError(t, err)
	require.NotEmpty(t, resp.Review["commit"])
	require.Equal(t, "feat/test", resp.Git["branch"])

	task, err := eng.GetTask(context.Background(), "ns-1", "T1")
	require.NoError(t, err)
	require.Equal(t, resp.Review["commit"], task.Metadata["review.commit"])
}

func TestGitCommitChangesOnceTruncatesLargeDiff(t *testing.T) {
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
	_, err = eng.CreateDAG(context.Background(), engine.CreateDAGRequest{NamespaceID: "ns-1", ID: "dag-1", Title: "DAG 1", ExecutionBranch: "feat/test"})
	require.NoError(t, err)
	_, err = eng.CreateTask(context.Background(), engine.CreateTaskRequest{NamespaceID: "ns-1", ID: "T1", Title: "task 1", AssignedWorker: "worker-a", DAGID: "dag-1"})
	require.NoError(t, err)
	_, err = srv.enterWorktreeOnce(context.Background(), "ns-1", "T1", "worker-a")
	require.NoError(t, err)

	task, err := eng.GetTask(context.Background(), "ns-1", "T1")
	require.NoError(t, err)
	wtPath := task.Metadata["git.worktree_path"]
	largeContent := bytes.Repeat([]byte("0123456789abcdef0123456789abcdef\n"), 7500) // ~255KB
	require.NoError(t, os.WriteFile(filepath.Join(wtPath, "large.txt"), largeContent, 0o644))
	runGitTest(t, wtPath, "add", "large.txt")
	runGitTest(t, wtPath, "commit", "-m", "add large file")

	resp, err := srv.gitCommitChangesOnce(context.Background(), "ns-1", "T1", "worker-a")
	require.NoError(t, err)
	diffVal, ok := resp.Review["diff"].(string)
	require.True(t, ok)
	require.True(t, len(diffVal) < 110*1024, "diff length %d should be safely truncated near 100KB", len(diffVal))
	require.Contains(t, diffVal, "[git diff truncated: total ")

	task, err = eng.GetTask(context.Background(), "ns-1", "T1")
	require.NoError(t, err)
	require.Equal(t, diffVal, task.Metadata["review.diff"])
}

func TestGitCommitChangesOnceRejectsDirtyWorktree(t *testing.T) {
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
	_, err = eng.CreateDAG(context.Background(), engine.CreateDAGRequest{NamespaceID: "ns-1", ID: "dag-1", Title: "DAG 1", ExecutionBranch: "feat/test"})
	require.NoError(t, err)
	_, err = eng.CreateTask(context.Background(), engine.CreateTaskRequest{NamespaceID: "ns-1", ID: "T1", Title: "task 1", AssignedWorker: "worker-a", DAGID: "dag-1"})
	require.NoError(t, err)
	_, err = srv.enterWorktreeOnce(context.Background(), "ns-1", "T1", "worker-a")
	require.NoError(t, err)

	task, err := eng.GetTask(context.Background(), "ns-1", "T1")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(task.Metadata["git.worktree_path"], "note.txt"), []byte("dirty\n"), 0o644))

	_, err = srv.gitCommitChangesOnce(context.Background(), "ns-1", "T1", "worker-a")
	require.Error(t, err)
	require.Contains(t, err.Error(), "worktree status")
}

func TestGitCommitChangesOnceRejectsWorkerMismatch(t *testing.T) {
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
	_, err = eng.CreateDAG(context.Background(), engine.CreateDAGRequest{NamespaceID: "ns-1", ID: "dag-1", Title: "DAG 1", ExecutionBranch: "feat/test"})
	require.NoError(t, err)
	_, err = eng.CreateTask(context.Background(), engine.CreateTaskRequest{NamespaceID: "ns-1", ID: "T1", Title: "task 1", AssignedWorker: "worker-a", DAGID: "dag-1"})
	require.NoError(t, err)
	_, err = srv.enterWorktreeOnce(context.Background(), "ns-1", "T1", "worker-a")
	require.NoError(t, err)

	_, err = srv.gitCommitChangesOnce(context.Background(), "ns-1", "T1", "worker-b")
	require.Error(t, err)
	require.Contains(t, err.Error(), "assigned to worker")
}

func TestGitCommitChangesProviderHandlesValidation(t *testing.T) {
	t.Parallel()

	eng, err := engine.NewEngine(engine.NewEngineConfig{})
	require.NoError(t, err)
	defer eng.Close()

	srv, err := New(eng, Config{})
	require.NoError(t, err)
	provider, err := newBTGitCommitProvider(srv)
	require.NoError(t, err)
	defer provider.close()

	req := httptest.NewRequest(http.MethodPost, "/git-commit-changes", nil)
	req.Header.Set("X-Agentflow-BT-Token", provider.token)
	rec := httptest.NewRecorder()
	provider.handleGitCommitChanges(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)

	req = httptest.NewRequest(http.MethodPost, "/git-commit-changes", nil)
	req.Header.Set("X-Agentflow-BT-Token", "wrong")
	rec = httptest.NewRecorder()
	provider.handleGitCommitChanges(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)

	_, err = eng.CreateNamespace(context.Background(), engine.CreateNamespaceRequest{ID: "ns-1", Name: "test"})
	require.NoError(t, err)
	req = httptest.NewRequest(http.MethodPost, "/git-commit-changes", strings.NewReader(`{"namespace_id":"ns-1","task_id":"missing","worker_id":"worker-a"}`))
	req.Header.Set("X-Agentflow-BT-Token", provider.token)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	provider.handleGitCommitChanges(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code)
}
