package server

import (
	"context"
	"strings"

	"github.com/toustifer/agentflow/pkg/engine"
	"github.com/toustifer/agentflow/pkg/hub"
)

// namespaceWorkdir soft-reads namespace.metadata["workdir"] for Hub client config.
// Empty string is fine: hub.Load still checks env + ~/.agent-hub/config.json.
func (s *Server) namespaceWorkdir(ctx context.Context, namespaceID string) string {
	if s == nil || s.engine == nil || strings.TrimSpace(namespaceID) == "" {
		return ""
	}
	ns, err := s.engine.GetNamespace(ctx, namespaceID)
	if err != nil || ns == nil || ns.Metadata == nil {
		return ""
	}
	return ns.Metadata["workdir"]
}

// taskGitRefs extracts branch/head from task metadata with optional overrides
// (e.g. review.commit after submit, or prepare_start runtime metadata).
func taskGitRefs(task *engine.Task, branchOverride, headOverride string) (branch, head string) {
	branch = strings.TrimSpace(branchOverride)
	head = strings.TrimSpace(headOverride)
	if task != nil && task.Metadata != nil {
		if branch == "" {
			branch = task.Metadata["git.branch"]
		}
		if head == "" {
			head = firstNonEmpty(
				task.Metadata["review.commit"],
				task.Metadata["git.head_sha"],
			)
		}
	}
	return branch, head
}

// projectTask builds SYNC_CONTRACT A1 whitelist from a local task.
func projectTask(task *engine.Task, branch, head string) hub.TaskProjection {
	if task == nil {
		return hub.TaskProjection{}
	}
	b, h := taskGitRefs(task, branch, head)
	return hub.TaskProjection{
		TaskID:         task.ID,
		Title:          task.Title,
		Status:         string(task.State),
		AssignedWorker: task.AssignedWorker,
		DependsOn:      append([]string(nil), task.DependsOn...),
		OutputFiles:    append([]string(nil), task.OutputFiles...),
		Branch:         b,
		HeadSHA:        h,
	}
}

// softSyncTaskToHub projects task → Hub hub_dag_state. Never fails callers.
// workdir may be empty (env/home config still apply).
func softSyncTaskToHub(ctx context.Context, workdir string, task *engine.Task, branchOverride, headOverride string) string {
	if task == nil {
		return hub.Result{Status: hub.StatusSkipped, Op: "task_sync", Message: "nil task"}.Note()
	}
	return syncTaskToHub(ctx, workdir, projectTask(task, branchOverride, headOverride))
}

// softHubAfterTask is the standard post-success hook: task row projection.
// Branch report stays at call sites that already know repoURL / bind needs.
func softHubAfterTask(ctx context.Context, workdir string, task *engine.Task, branchOverride, headOverride string) string {
	return softSyncTaskToHub(ctx, workdir, task, branchOverride, headOverride)
}
