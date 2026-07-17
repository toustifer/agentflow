package server

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/toustifer/agentflow/pkg/engine"
)

type taskGitRuntime struct {
	Branch       string
	BaseBranch   string
	RepoPath     string
	WorktreePath string
	HeadSHA      string
	Status       string
}

func (s *Server) prepareTaskGitRuntime(ctx context.Context, ns *engine.Namespace, dag *engine.DAG, task *engine.Task, allowRepair bool) (*taskGitRuntime, error) {
	if ns == nil {
		return nil, fmt.Errorf("namespace is required")
	}
	if dag == nil {
		return nil, fmt.Errorf("dag is required")
	}
	if task == nil {
		return nil, fmt.Errorf("task is required")
	}
	if ns.Metadata == nil || ns.Metadata["workdir"] == "" {
		return nil, fmt.Errorf("start 被拒绝：namespace %q 没有绑定 workdir。请在创建 namespace 时传入 metadata.workdir（绝对路径）", ns.ID)
	}
	if dag.ExecutionBranch == "" {
		return nil, fmt.Errorf("start 被拒绝：task %q 所属 DAG %q 没有 execution_branch。请先为 DAG 设置 execution_branch", task.ID, dag.ID)
	}

	repoPath, err := validateGitRepo(ctx, ns.Metadata["workdir"])
	if err != nil {
		return nil, err
	}
	baseBranch, err := detectBaseBranch(ctx, repoPath, ns.Metadata["git_main_branch"])
	if err != nil {
		return nil, err
	}
	if dag.BaseBranch != "" {
		baseBranch = dag.BaseBranch
	}
	if dag.ExecutionBranch == baseBranch {
		return nil, fmt.Errorf("start 被拒绝：DAG %q 的 execution_branch %q 不能等于 base_branch %q", dag.ID, dag.ExecutionBranch, baseBranch)
	}
	hasHead, err := repoHasHeadCommit(ctx, repoPath)
	if err != nil {
		return nil, err
	}
	if !hasHead {
		return nil, fmt.Errorf("start 被拒绝：git repo %q 还没有首个 commit。请先在 repo root 创建 seed 文件并提交首个 commit，再进入 worktree 主链", repoPath)
	}
	worktreePath := worktreePathForTask(repoPath, ns, dag, task)
	if err := os.MkdirAll(filepath.Dir(worktreePath), 0o755); err != nil {
		return nil, fmt.Errorf("create worktree parent: %w", err)
	}
	if err := ensureTaskWorktree(ctx, repoPath, worktreePath, dag.ExecutionBranch, baseBranch, allowRepair); err != nil {
		return nil, err
	}
	status, err := inspectTaskGitRuntime(ctx, repoPath, worktreePath, dag.ExecutionBranch, baseBranch)
	if err != nil {
		return nil, err
	}
	return status, nil
}

func (s *Server) resolveTaskGitRuntime(ctx context.Context, ns *engine.Namespace, dag *engine.DAG, task *engine.Task) (*taskGitRuntime, error) {
	if ns == nil {
		return nil, fmt.Errorf("namespace is required")
	}
	if task == nil {
		return nil, fmt.Errorf("task is required")
	}
	if ns.Metadata == nil || ns.Metadata["workdir"] == "" {
		return nil, fmt.Errorf("namespace %q 没有绑定 workdir", ns.ID)
	}
	branch := ""
	baseBranch := ""
	if dag != nil {
		branch = dag.ExecutionBranch
		baseBranch = dag.BaseBranch
	}
	if task.Metadata != nil && task.Metadata["git.branch"] != "" {
		branch = task.Metadata["git.branch"]
	}
	if branch == "" {
		return nil, fmt.Errorf("task %q 没有关联 git branch", task.ID)
	}
	repoPath, err := validateGitRepo(ctx, ns.Metadata["workdir"])
	if err != nil {
		return nil, err
	}
	if baseBranch == "" {
		baseBranch, err = detectBaseBranch(ctx, repoPath, ns.Metadata["git_main_branch"])
		if err != nil {
			return nil, err
		}
	}
	worktreePath := worktreePathForTask(repoPath, ns, dag, task)
	if task.Metadata != nil && task.Metadata["git.worktree_path"] != "" {
		worktreePath = task.Metadata["git.worktree_path"]
	}
	if _, err := os.Stat(worktreePath); err == nil {
		return inspectTaskGitRuntime(ctx, repoPath, worktreePath, branch, baseBranch)
	}
	return &taskGitRuntime{
		Branch:       branch,
		BaseBranch:   baseBranch,
		RepoPath:     repoPath,
		WorktreePath: worktreePath,
		HeadSHA:      "",
		Status:       "missing",
	}, nil
}

func (s *Server) getTaskGitRuntime(ctx context.Context, ns *engine.Namespace, dag *engine.DAG, task *engine.Task) (*taskGitRuntime, error) {
	runtime, err := s.resolveTaskGitRuntime(ctx, ns, dag, task)
	if err != nil {
		return nil, err
	}
	if runtime.Status == "missing" {
		return nil, fmt.Errorf("worktree 不存在: %s", runtime.WorktreePath)
	}
	return runtime, nil
}

func worktreePathForTask(repoPath string, ns *engine.Namespace, dag *engine.DAG, task *engine.Task) string {
	root := filepath.Join(filepath.Dir(repoPath), ".agentflow-worktrees", filepath.Base(repoPath))
	if ns != nil && ns.Metadata != nil && ns.Metadata["worktree_root"] != "" {
		root = ns.Metadata["worktree_root"]
	}
	if dag != nil && dag.ID != "" {
		return filepath.Join(root, dag.ID)
	}
	if task != nil && task.ID != "" {
		return filepath.Join(root, "standalone", task.ID)
	}
	return filepath.Join(root, "standalone")
}

func validateGitRepo(ctx context.Context, workdir string) (string, error) {
	if workdir == "" {
		return "", fmt.Errorf("git repo path is empty")
	}
	info, err := os.Stat(workdir)
	if err != nil {
		return "", fmt.Errorf("workdir 不存在: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("workdir %q 不是目录", workdir)
	}
	root, err := runGit(ctx, workdir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("workdir %q 不是 git repo: %w", workdir, err)
	}
	return filepath.Clean(root), nil
}

func detectBaseBranch(ctx context.Context, repoPath, configured string) (string, error) {
	if configured != "" {
		return configured, nil
	}
	if ref, err := runGit(ctx, repoPath, "symbolic-ref", "--quiet", "refs/remotes/origin/HEAD"); err == nil {
		if idx := strings.LastIndex(ref, "/"); idx >= 0 && idx+1 < len(ref) {
			return ref[idx+1:], nil
		}
	}
	if branch, err := runGit(ctx, repoPath, "branch", "--show-current"); err == nil && branch != "" {
		return branch, nil
	}
	for _, candidate := range []string{"main", "master"} {
		if err := gitRefExists(ctx, repoPath, "refs/heads/"+candidate); err == nil {
			return candidate, nil
		}
		if err := gitRefExists(ctx, repoPath, "refs/remotes/origin/"+candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("无法检测默认 base branch")
}

func ensureTaskWorktree(ctx context.Context, repoPath, worktreePath, branch, baseBranch string, allowRepair bool) error {
	if runtime, err := inspectTaskGitRuntime(ctx, repoPath, worktreePath, branch, baseBranch); err == nil {
		if runtime.Branch == branch {
			return nil
		}
		return fmt.Errorf("现有 worktree %q 绑定到分支 %q，不是期望的 %q", worktreePath, runtime.Branch, branch)
	}
	if _, err := os.Stat(worktreePath); err == nil {
		if err := repairInvalidWorktreePath(worktreePath, allowRepair); err != nil {
			return fmt.Errorf("路径 %q 已存在但不是有效 worktree，请先手动清理: %w", worktreePath, err)
		}
	}

	// Main workdir must not hold execution_branch; otherwise `git worktree add`
	// fails with "already checked out". Free it back to base when clean.
	if err := ensureMainRepoOnBaseBranch(ctx, repoPath, branch, baseBranch); err != nil {
		return err
	}

	localRef := "refs/heads/" + branch
	remoteRef := "refs/remotes/origin/" + branch
	if err := gitRefExists(ctx, repoPath, localRef); err == nil {
		_, err = runGit(ctx, repoPath, "worktree", "add", worktreePath, branch)
		if err != nil {
			return fmt.Errorf("create worktree from local branch: %w", err)
		}
		return nil
	}
	if err := gitRefExists(ctx, repoPath, remoteRef); err == nil {
		_, err = runGit(ctx, repoPath, "worktree", "add", "-b", branch, worktreePath, "origin/"+branch)
		if err != nil {
			return fmt.Errorf("create worktree from remote branch: %w", err)
		}
		return nil
	}

	baseRef := baseBranch
	if err := gitRefExists(ctx, repoPath, "refs/remotes/origin/"+baseBranch); err == nil {
		baseRef = "origin/" + baseBranch
	}
	_, err := runGit(ctx, repoPath, "worktree", "add", "-b", branch, worktreePath, baseRef)
	if err != nil {
		return fmt.Errorf("create worktree from base branch: %w", err)
	}
	return nil
}

// ensureMainRepoOnBaseBranch keeps the primary workdir off the DAG execution
// branch so a shared worktree can own that branch.
func ensureMainRepoOnBaseBranch(ctx context.Context, repoPath, executionBranch, baseBranch string) error {
	if repoPath == "" || executionBranch == "" || baseBranch == "" {
		return nil
	}
	if executionBranch == baseBranch {
		return nil
	}
	current, err := runGit(ctx, repoPath, "branch", "--show-current")
	if err != nil {
		// Detached HEAD or non-branch checkout: leave alone.
		return nil
	}
	if current != executionBranch {
		return nil
	}
	status, err := runGit(ctx, repoPath, "status", "--porcelain")
	if err != nil {
		return fmt.Errorf("检查主仓状态失败: %w", err)
	}
	if strings.TrimSpace(status) != "" {
		return fmt.Errorf(
			"主仓 workdir 当前在 execution_branch %q 上且有未提交改动，无法创建 worktree。请先提交/贮藏改动，并把主仓切回 base branch %q",
			executionBranch,
			baseBranch,
		)
	}
	if _, err := runGit(ctx, repoPath, "checkout", baseBranch); err != nil {
		return fmt.Errorf(
			"主仓 workdir 占用 execution_branch %q，自动切回 base branch %q 失败: %w",
			executionBranch,
			baseBranch,
			err,
		)
	}
	return nil
}

func inspectTaskGitRuntime(ctx context.Context, repoPath, worktreePath, expectedBranch, baseBranch string) (*taskGitRuntime, error) {
	if _, err := os.Stat(worktreePath); err != nil {
		return nil, fmt.Errorf("worktree 不存在: %w", err)
	}
	branch, err := runGit(ctx, worktreePath, "branch", "--show-current")
	if err != nil {
		return nil, fmt.Errorf("读取 worktree 分支失败: %w", err)
	}
	headSHA, err := runGit(ctx, worktreePath, "rev-parse", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("读取 HEAD 失败: %w", err)
	}
	statusText, err := runGit(ctx, worktreePath, "status", "--short")
	if err != nil {
		return nil, fmt.Errorf("读取 git status 失败: %w", err)
	}
	status := "clean"
	if strings.TrimSpace(statusText) != "" {
		status = "dirty"
	}
	if expectedBranch != "" && branch != expectedBranch {
		status = "branch_mismatch"
	}
	return &taskGitRuntime{
		Branch:       branch,
		BaseBranch:   baseBranch,
		RepoPath:     repoPath,
		WorktreePath: worktreePath,
		HeadSHA:      headSHA,
		Status:       status,
	}, nil
}

func repoHasHeadCommit(ctx context.Context, repoPath string) (bool, error) {
	if repoPath == "" {
		return false, fmt.Errorf("git repo path is empty")
	}
	_, err := runGit(ctx, repoPath, "rev-parse", "HEAD")
	if err == nil {
		return true, nil
	}
	if strings.Contains(err.Error(), "ambiguous argument 'HEAD'") || strings.Contains(err.Error(), "unknown revision or path not in the working tree") || strings.Contains(err.Error(), "Needed a single revision") {
		return false, nil
	}
	return false, err
}

func gitRefExists(ctx context.Context, repoPath, ref string) error {
	_, err := runGit(ctx, repoPath, "show-ref", "--verify", "--quiet", ref)
	return err
}

// ensureReadyGitRepo returns (repoPath, defaultBranch, error). It either runs
// `git init -b mainBranch` when `initGit` is true and workdir is not a repo,
// or validates an existing repository. Returns the parent repo root.
func ensureReadyGitRepo(ctx context.Context, workdir, mainBranch string, initGit bool, userName, userEmail string) (string, string, error) {
	if workdir == "" {
		return "", "", fmt.Errorf("workdir is required")
	}
	info, err := os.Stat(workdir)
	if err != nil {
		return "", "", fmt.Errorf("workdir %q not found: %w", workdir, err)
	}
	if !info.IsDir() {
		return "", "", fmt.Errorf("workdir %q is not a directory", workdir)
	}

	repoPath, repoErr := validateGitRepo(ctx, workdir)
	if repoErr == nil {
		branch, bErr := detectBaseBranch(ctx, repoPath, mainBranch)
		if bErr != nil {
			return "", "", bErr
		}
		return repoPath, branch, nil
	}
	if !initGit {
		return "", "", fmt.Errorf("workdir %q is not a git repo and init_git=false", workdir)
	}
	if _, err := runGit(ctx, workdir, "init", "-b", mainBranch); err != nil {
		return "", "", fmt.Errorf("git init: %w", err)
	}
	if userName != "" {
		_, _ = runGit(ctx, workdir, "config", "user.name", userName)
	}
	if userEmail != "" {
		_, _ = runGit(ctx, workdir, "config", "user.email", userEmail)
	}
	repoPath, err = validateGitRepo(ctx, workdir)
	if err != nil {
		return "", "", err
	}
	branch, err := detectBaseBranch(ctx, repoPath, mainBranch)
	if err != nil {
		return "", "", err
	}
	return repoPath, branch, nil
}

func runGit(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s failed: %s", strings.Join(args, " "), msg)
	}
	return strings.TrimSpace(stdout.String()), nil
}
