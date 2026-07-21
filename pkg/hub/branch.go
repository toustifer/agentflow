package hub

import (
	"context"
	"os"
	"strings"
)

// BranchReport is a soft projection of git branch tip + optional binding.
type BranchReport struct {
	RepoURL  string
	Branch   string
	HeadSHA  string
	BindType string // task|dag|worker|user
	BindID   string
	Reporter string
}

// ReportBranch posts branch tip + bindings. Soft-fail; never errors to caller.
func (c *Client) ReportBranch(ctx context.Context, in BranchReport) Result {
	const op = "branch_report"
	if r, ok := c.guard(op); !ok {
		return r
	}
	if strings.TrimSpace(in.Branch) == "" {
		return Result{Status: StatusSkipped, Op: op, Message: "empty branch"}
	}

	// Optional soft membership check (cached). Failure does not block the HTTP attempt
	// when we already have credentials — server enforces; we only skip if clearly disabled.
	_ = c.EnsureMembership(ctx)

	host, _ := os.Hostname()
	type branchItem struct {
		Name   string `json:"name"`
		TipSHA string `json:"tip_sha"`
		Source string `json:"source"`
	}
	type bindItem struct {
		BindType     string `json:"bind_type"`
		BindID       string `json:"bind_id"`
		BranchName   string `json:"branch_name"`
		HeadSHA      string `json:"head_sha"`
		WorktreeHost string `json:"worktree_host"`
		Status       string `json:"status"`
	}
	body := map[string]any{
		"reporter": in.Reporter,
		"branches": []branchItem{{
			Name:   in.Branch,
			TipSHA: in.HeadSHA,
			Source: "report",
		}},
	}
	if in.RepoURL != "" {
		body["repo_url"] = in.RepoURL
	}
	if in.BindType != "" && in.BindID != "" {
		body["bindings"] = []bindItem{{
			BindType:     in.BindType,
			BindID:       in.BindID,
			BranchName:   in.Branch,
			HeadSHA:      in.HeadSHA,
			WorktreeHost: host,
			Status:       "active",
		}}
	}

	path := "/v1/hub/repos/" + c.cfg.BusinessCode + "/branches/report"
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
