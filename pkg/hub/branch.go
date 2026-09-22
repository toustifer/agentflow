package hub

import (
	"context"
	"os"
	"strings"
)

// BranchReport is a soft projection of a git branch tip plus an optional binding.
//
// Never include a worktree absolute path here: cross-machine it is meaningless
// and it leaks the local environment (docs/SYNC_CONTRACT.md §3.2). Only the
// hostname goes on the wire.
type BranchReport struct {
	RepoURL  string
	Branch   string
	HeadSHA  string
	BindType string // task|dag|worker|user
	BindID   string
	Reporter string
}

// branchReportItem is one entry of the "branches" array.
type branchReportItem struct {
	Name   string `json:"name"`
	TipSHA string `json:"tip_sha"`
	Source string `json:"source"`
}

// branchBindingItem is one entry of the "bindings" array.
type branchBindingItem struct {
	BindType     string `json:"bind_type"`
	BindID       string `json:"bind_id"`
	BranchName   string `json:"branch_name"`
	HeadSHA      string `json:"head_sha"`
	WorktreeHost string `json:"worktree_host"`
	Status       string `json:"status"`
}

// branchReportBody is the wire shape for
// POST /v1/hub/repos/{business_code}/branches/report.
// Field names are pinned by TestReportBranchBodyShape — do not rename.
type branchReportBody struct {
	Reporter string              `json:"reporter,omitempty"`
	RepoURL  string              `json:"repo_url,omitempty"`
	Branches []branchReportItem  `json:"branches"`
	Bindings []branchBindingItem `json:"bindings,omitempty"`
}

// ReportBranch posts a branch tip plus optional binding projection.
// Soft-fail: it never returns an error to the caller.
func (c *Client) ReportBranch(ctx context.Context, in BranchReport) Result {
	const op = "branch_report"
	if r, ok := c.guard(op); !ok {
		return r
	}
	if strings.TrimSpace(in.Branch) == "" {
		return Result{Status: StatusSkipped, Op: op, Message: "empty branch"}
	}

	// Advisory only: an auth failure is not a hard gate, the server enforces.
	_ = c.EnsureMembership(ctx)

	host, _ := os.Hostname()
	body := branchReportBody{
		Reporter: in.Reporter,
		RepoURL:  in.RepoURL,
		Branches: []branchReportItem{{
			Name:   in.Branch,
			TipSHA: in.HeadSHA,
			Source: "report",
		}},
	}
	if in.BindType != "" && in.BindID != "" {
		body.Bindings = []branchBindingItem{{
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
