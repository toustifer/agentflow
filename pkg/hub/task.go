package hub

import (
	"context"
	"strings"
)

// TaskProjection is the SYNC_CONTRACT whitelist for hub_dag_state (A1).
// Local Task is the source of truth; this is a team-visible mirror only.
type TaskProjection struct {
	TaskID         string
	Title          string
	Status         string // agentflow TaskState string
	AssignedWorker string
	DependsOn      []string
	OutputFiles    []string
	Branch         string
	HeadSHA        string
}

// SyncTask UPSERTs one task row on Hub. Soft-fail; never blocks local transitions.
func (c *Client) SyncTask(ctx context.Context, in TaskProjection) Result {
	const op = "task_sync"
	if r, ok := c.guard(op); !ok {
		return r
	}
	if strings.TrimSpace(in.TaskID) == "" {
		return Result{Status: StatusSkipped, Op: op, Message: "empty task_id"}
	}
	_ = c.EnsureMembership(ctx)

	body := map[string]any{
		"task_id":         in.TaskID,
		"title":           in.Title,
		"status":          in.Status,
		"assigned_worker": in.AssignedWorker,
		"depends_on":      in.DependsOn,
		"output_files":    in.OutputFiles,
		"branch":          in.Branch,
		"head_sha":        in.HeadSHA,
	}
	// Prefer JWT path used by RequireMembership handlers.
	path := "/v1/hub/dag/" + c.cfg.BusinessCode
	// router may be /v1/hub/dag/:code — confirm callers; SyncDAG uses Param code.
	status, _, err := c.doJSON(ctx, "POST", path, body)
	if err != nil {
		return Result{Status: StatusFailed, Op: op, Message: err.Error()}
	}
	if status == 401 || status == 403 {
		c.InvalidateAuth()
		return Result{Status: StatusFailed, Op: op, Code: status, Message: "forbidden"}
	}
	if status >= 300 {
		return Result{Status: StatusFailed, Op: op, Code: status}
	}
	return Result{Status: StatusOK, Op: op}
}
