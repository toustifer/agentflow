package server

import (
	"context"
	"strings"

	"github.com/toustifer/agentflow/pkg/engine"
	"github.com/toustifer/agentflow/pkg/hub"
)

// This file is the only place where the local task lifecycle touches agent-hub.
//
// Direction is strictly L→H (agentflow → Hub): nothing here ever reads Hub state
// back into SQLite. Every call is a soft side effect — it returns a hub.Result,
// never an error, and a Hub outage can neither fail a tool call nor roll back a
// local transition (docs/SYNC_CONTRACT.md §1).
//
// pkg/engine must not import pkg/hub; the MCP edge owns the projection.

// MCP payload keys carrying the soft-sync notes back to the caller.
//
// The value is hub.Result.Note() verbatim ("hub_task_sync_ok",
// "hub_task_sync_skipped: no login token / business_code", ...), so a caller can
// tell "not configured" apart from "tried and failed" without parsing prose.
const (
	hubNoteKey       = "hub_note"
	hubBranchNoteKey = "hub_branch_note"
)

// hubProjector is the note-capable Hub seam.
//
// The namespace metadata (team code) and the workdir (credential layers) are
// passed in rather than baked into a client, so a bind or a kill-switch change
// is picked up on the next tool call.
type hubProjector interface {
	ProjectTask(ctx context.Context, nsMeta map[string]string, workdir string, p hub.TaskProjection) hub.Result
	ProjectBranch(ctx context.Context, nsMeta map[string]string, workdir string, r hub.BranchReport) hub.Result
}

// realHubProjector is the production projector: it builds a pkg/hub client per
// call via hub.NewFromNamespace.
//
// Building per call is deliberate. A hub_bind_team, a credential change or
// HUB_DISABLED=1 takes effect immediately instead of at process start, and the
// kill-switch cannot be defeated by a client cached before it was set. The cost
// is that pkg/hub's membership cache starts cold each time, so an *enabled*
// projection that must re-probe costs 1 auth probe + 1 write. With no team code
// or no credential the client returns StatusSkipped and dials nothing at all
// (docs/SYNC_CONTRACT.md §2).
type realHubProjector struct{}

func (realHubProjector) ProjectTask(ctx context.Context, nsMeta map[string]string, workdir string, p hub.TaskProjection) hub.Result {
	return hub.NewFromNamespace(nsMeta, workdir).SyncTask(ctx, p)
}

func (realHubProjector) ProjectBranch(ctx context.Context, nsMeta map[string]string, workdir string, r hub.BranchReport) hub.Result {
	return hub.NewFromNamespace(nsMeta, workdir).ReportBranch(ctx, r)
}

// hubNamespaceTarget resolves the namespace metadata + workdir a projection
// needs to find its team code and credentials.
//
// ok=false means the namespace could not be read. The caller then reports
// nothing at all rather than a note that would blame missing credentials for
// what is really a lookup failure.
func (s *Server) hubNamespaceTarget(ctx context.Context, namespaceID string) (nsMeta map[string]string, workdir string, ok bool) {
	if s == nil || s.engine == nil || strings.TrimSpace(namespaceID) == "" {
		return nil, "", false
	}
	ns, err := s.engine.GetNamespace(ctx, namespaceID)
	if err != nil || ns == nil {
		return nil, "", false
	}
	if ns.Metadata != nil {
		workdir = strings.TrimSpace(ns.Metadata["workdir"])
	}
	return ns.Metadata, workdir, true
}

// taskGitRefs returns the branch and tip an L→H projection should carry.
//
// review.commit is preferred over git.head_sha only for transitions that
// advance the tip past what task start recorded (submit and later): at start
// time git.head_sha is the fresh base tip while review.commit may still be a
// stale value from an earlier review cycle.
func taskGitRefs(task *engine.Task, preferReviewTip bool) (branch, headSHA string) {
	if task == nil || task.Metadata == nil {
		return "", ""
	}
	branch = strings.TrimSpace(task.Metadata["git.branch"])
	headSHA = strings.TrimSpace(task.Metadata["git.head_sha"])
	if preferReviewTip {
		if commit := strings.TrimSpace(task.Metadata["review.commit"]); commit != "" {
			headSHA = commit
		}
	}
	return branch, headSHA
}

// projectTask soft-projects one task row and returns the note to backfill
// ("" when there is nothing to report).
func (s *Server) projectTask(ctx context.Context, task *engine.Task, branch, headSHA string) string {
	if s == nil || s.projector == nil || task == nil {
		return ""
	}
	nsMeta, workdir, ok := s.hubNamespaceTarget(ctx, task.NamespaceID)
	if !ok {
		return ""
	}
	res := s.projector.ProjectTask(ctx, nsMeta, workdir, hub.TaskProjection{
		TaskID:         task.ID,
		Title:          task.Title,
		Status:         string(task.State),
		AssignedWorker: task.AssignedWorker,
		DependsOn:      task.DependsOn,
		OutputFiles:    task.OutputFiles,
		Branch:         branch,
		HeadSHA:        headSHA,
	})
	return res.Note()
}

// projectBranch soft-reports a branch tip bound to a task and returns the note.
//
// Only the hostname goes on the wire (pkg/hub adds it); the worktree's absolute
// path is deliberately never sent.
func (s *Server) projectBranch(ctx context.Context, task *engine.Task, branch, headSHA string) string {
	if s == nil || s.projector == nil || task == nil {
		return ""
	}
	nsMeta, workdir, ok := s.hubNamespaceTarget(ctx, task.NamespaceID)
	if !ok {
		return ""
	}
	res := s.projector.ProjectBranch(ctx, nsMeta, workdir, hub.BranchReport{
		Branch:   branch,
		HeadSHA:  headSHA,
		BindType: "task",
		BindID:   task.ID,
		Reporter: "agentflow",
	})
	return res.Note()
}

// attachLifecycleHubNotes runs the L→H projection for one lifecycle trigger and
// backfills the notes into the tool payload.
//
// It is called after the handler has already committed the local change, so a
// Hub fault is invisible to the lifecycle: the tool still returns the advanced
// task, only the note records what the mirror did.
func (s *Server) attachLifecycleHubNotes(ctx context.Context, result taskResult, withBranchReport, preferReviewTip bool) {
	if result.task == nil || result.payload == nil {
		return
	}
	branch, headSHA := taskGitRefs(result.task, preferReviewTip)
	if note := s.projectTask(ctx, result.task, branch, headSHA); note != "" {
		result.payload[hubNoteKey] = note
	}
	if !withBranchReport {
		return
	}
	if note := s.projectBranch(ctx, result.task, branch, headSHA); note != "" {
		result.payload[hubBranchNoteKey] = note
	}
}

// preferReviewTip reports whether a transition carries the reviewed tip rather
// than the tip the task started from. Only start/resume re-record git.head_sha;
// every later verb moves the tip forward via review.commit.
func preferReviewTip(input map[string]any) bool {
	transition, _ := input["transition"].(string)
	switch strings.TrimSpace(transition) {
	case "start", "resume":
		return false
	default:
		return true
	}
}
