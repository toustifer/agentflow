package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnsureMainRepoOnBaseBranchChecksOutBaseWhenClean(t *testing.T) {
	t.Parallel()

	repo := initTestGitRepo(t)
	runGitTest(t, repo, "checkout", "-b", "feat/exec")

	err := ensureMainRepoOnBaseBranch(context.Background(), repo, "feat/exec", "main")
	require.NoError(t, err)

	branch, err := runGit(context.Background(), repo, "branch", "--show-current")
	require.NoError(t, err)
	require.Equal(t, "main", branch)
}

func TestEnsureMainRepoOnBaseBranchRejectsDirtyExecutionBranch(t *testing.T) {
	t.Parallel()

	repo := initTestGitRepo(t)
	runGitTest(t, repo, "checkout", "-b", "feat/exec")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "dirty.txt"), []byte("x"), 0o644))

	err := ensureMainRepoOnBaseBranch(context.Background(), repo, "feat/exec", "main")
	require.Error(t, err)
	require.Contains(t, err.Error(), "未提交改动")
	require.Contains(t, err.Error(), "feat/exec")

	branch, err := runGit(context.Background(), repo, "branch", "--show-current")
	require.NoError(t, err)
	require.Equal(t, "feat/exec", branch)
}

func TestEnsureMainRepoOnBaseBranchNoopWhenAlreadyOnBase(t *testing.T) {
	t.Parallel()

	repo := initTestGitRepo(t)
	runGitTest(t, repo, "branch", "feat/exec")

	err := ensureMainRepoOnBaseBranch(context.Background(), repo, "feat/exec", "main")
	require.NoError(t, err)

	branch, err := runGit(context.Background(), repo, "branch", "--show-current")
	require.NoError(t, err)
	require.Equal(t, "main", branch)
}

func TestEnsureTaskWorktreeFreesPrimaryExecutionBranch(t *testing.T) {
	t.Parallel()

	repo := initTestGitRepo(t)
	runGitTest(t, repo, "checkout", "-b", "feat/exec")
	worktreePath := filepath.Join(t.TempDir(), "wt-exec")

	err := ensureTaskWorktree(context.Background(), repo, worktreePath, "feat/exec", "main", false)
	require.NoError(t, err)

	primaryBranch, err := runGit(context.Background(), repo, "branch", "--show-current")
	require.NoError(t, err)
	require.Equal(t, "main", primaryBranch)

	wtBranch, err := runGit(context.Background(), worktreePath, "branch", "--show-current")
	require.NoError(t, err)
	require.Equal(t, "feat/exec", wtBranch)
}
