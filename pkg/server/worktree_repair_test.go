package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClassifyInvalidExistingWorktreePath(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.txt"), []byte("junk"), 0o644))

	state := classifyWorktreePath(dir)
	require.Equal(t, worktreePathInvalidExisting, state)
}

func TestRepairInvalidWorktreePathRequiresExplicitOptIn(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "junk.txt"), []byte("x"), 0o644))

	err := repairInvalidWorktreePath(dir, false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "repair not allowed")
}

func TestRepairInvalidWorktreePathQuarantinesInsteadOfDelete(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dir := filepath.Join(root, "broken-worktree")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "junk.txt"), []byte("x"), 0o644))

	err := repairInvalidWorktreePath(dir, true)
	require.NoError(t, err)

	_, err = os.Stat(dir)
	require.True(t, os.IsNotExist(err))

	backup := dir + ".invalid-bak"
	info, err := os.Stat(backup)
	require.NoError(t, err)
	require.True(t, info.IsDir())
	_, err = os.Stat(filepath.Join(backup, "junk.txt"))
	require.NoError(t, err)
}

func TestTaskPrepareStartRejectsRepairWithoutTargetedResume(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	createDagTaskForStart(t, srv, "T-legacy-repair-denied")

	_, err := srv.Handle(context.Background(), "task_prepare_start", map[string]any{
		"namespace_id": "ns-1",
		"task_id":      "T-legacy-repair-denied",
		"allow_repair": true,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "allow_repair requires resume_targeted=true")
}

func TestTaskPrepareStartCanRepairInvalidLegacyWorktreeWhenTargeted(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	createDagTaskForStart(t, srv, "T-legacy-repair")

	task, err := srv.engine.GetTask(context.Background(), "ns-1", "T-legacy-repair")
	require.NoError(t, err)
	dag, err := srv.engine.GetDAG(context.Background(), "ns-1", task.DAGID)
	require.NoError(t, err)
	ns, err := srv.engine.GetNamespace(context.Background(), "ns-1")
	require.NoError(t, err)
	worktreePath := worktreePathForTask(ns.Metadata["workdir"], ns, dag, task)
	require.NoError(t, os.MkdirAll(worktreePath, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(worktreePath, "junk.txt"), []byte("x"), 0o644))

	_, err = srv.Handle(context.Background(), "task_prepare_start", map[string]any{
		"namespace_id":    "ns-1",
		"task_id":         "T-legacy-repair",
		"allow_repair":    true,
		"resume_targeted": true,
	})
	require.NoError(t, err)
}
