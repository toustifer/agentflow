package hub

import (
	"context"
	"strings"
)

// TaskProjection is the SYNC_CONTRACT whitelist for one task row on the Hub
// (hub_dag_state). The local agentflow Task stays the source of truth; this is a
// team-visible mirror only.
//
// Never add Description/AcceptanceCriteria/paths here — see docs/SYNC_CONTRACT.md §3.3.
type TaskProjection struct {
	TaskID         string
	Title          string
	Status         string // agentflow TaskState string, passed through verbatim
	AssignedWorker string
	DependsOn      []string
	OutputFiles    []string
	Branch         string
	HeadSHA        string
}

// taskProjectionBody is the wire shape. Field names are part of the Hub
// contract and are pinned by TestSyncTaskBodyFields — do not rename.
type taskProjectionBody struct {
	TaskID         string   `json:"task_id"`
	Title          string   `json:"title"`
	Status         string   `json:"status"`
	AssignedWorker string   `json:"assigned_worker"`
	DependsOn      []string `json:"depends_on"`
	OutputFiles    []string `json:"output_files"`
	Branch         string   `json:"branch"`
	HeadSHA        string   `json:"head_sha"`
}

// SyncTask UPSERTs one task row on the Hub via POST /v1/hub/dag/{business_code}.
//
// Soft-fail: it never blocks a local task transition and never returns an error.
func (c *Client) SyncTask(ctx context.Context, in TaskProjection) Result {
	const op = "task_sync"
	if r, ok := c.guard(op); !ok {
		return r
	}
	if strings.TrimSpace(in.TaskID) == "" {
		return Result{Status: StatusSkipped, Op: op, Message: "empty task_id"}
	}

	// Advisory only: auth failure is not a hard gate, the server enforces.
	_ = c.EnsureMembership(ctx)

	body := taskProjectionBody{
		TaskID:         in.TaskID,
		Title:          in.Title,
		Status:         in.Status,
		AssignedWorker: in.AssignedWorker,
		DependsOn:      in.DependsOn,
		OutputFiles:    in.OutputFiles,
		Branch:         in.Branch,
		HeadSHA:        in.HeadSHA,
	}
	path := "/v1/hub/dag/" + c.cfg.BusinessCode
	status, _, err := c.doJSON(ctx, "POST", path, body)
	if err != nil {
		return Result{Status: StatusFailed, Op: op, Message: err.Error()}
	}
	if status == 401 || status == 403 {
		c.InvalidateAuth()
		return Result{Status: StatusFailed, Op: op, Code: status, Message: "forbidden"}
	}
	if status >= 300 {
		// Message intentionally empty so the note reads "hub_task_sync_failed: status N".
		return Result{Status: StatusFailed, Op: op, Code: status}
	}
	return Result{Status: StatusOK, Op: op}
}
