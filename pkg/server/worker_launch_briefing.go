package server

import (
	"context"
	"os"
	"path/filepath"

	"github.com/toustifer/agentflow/pkg/engine"
)

type workerLaunchBriefing struct {
	Required          bool     `json:"required"`
	Started           bool     `json:"started"`
	LeaderNextAction  string   `json:"leader_next_action"`
	Warning           string   `json:"warning"`
	WorkerID          string   `json:"worker_id"`
	TaskID            string   `json:"task_id"`
	TaskTitle         string   `json:"task_title"`
	WorktreePath      string   `json:"worktree_path,omitempty"`
	Branch            string   `json:"branch,omitempty"`
	DispatchMode      string   `json:"dispatch_mode"`
	PromptTemplate    string   `json:"prompt_template"`
	RequiredReads     []string `json:"required_reads"`
	RecommendedMCP    []string `json:"recommended_mcp"`
	RecommendedSkills []string `json:"recommended_skills"`
	LaunchInstructions []string `json:"launch_instructions"`
}

func (s *Server) buildWorkerLaunchBriefing(ctx context.Context, ns *engine.Namespace, task *engine.Task) (map[string]any, error) {
	prompt, err := s.engine.WorkerPromptGet(ctx, ns.ID, task.AssignedWorker, task.ID, task.Title, false)
	if err != nil {
		return nil, err
	}

	requiredReads := make([]string, 0, 1)
	if ns != nil && ns.Metadata != nil {
		if workdir := ns.Metadata["workdir"]; workdir != "" {
			shapePath := filepath.Join(workdir, ".claude", "PROJECT_FINAL_SHAPE.md")
			if _, err := os.Stat(shapePath); err == nil {
				requiredReads = append(requiredReads, shapePath)
			}
		}
	}

	briefing := workerLaunchBriefing{
		Required:         true,
		Started:          false,
		LeaderNextAction: "launch_worker_manually",
		Warning:          "This call only prepared the worker context. The leader must explicitly launch the worker.",
		WorkerID:         task.AssignedWorker,
		TaskID:           task.ID,
		TaskTitle:        task.Title,
		WorktreePath:     task.Metadata["git.worktree_path"],
		Branch:           task.Metadata["git.branch"],
		DispatchMode:     "manual_subagent",
		PromptTemplate:   prompt,
		RequiredReads:    requiredReads,
		RecommendedMCP: []string{
			"doc_search",
			"worker_handbook_get",
			"find_knowledge",
			"find_pitfalls",
		},
		RecommendedSkills: []string{},
		LaunchInstructions: []string{
			"Read required_reads first.",
			"Launch a subagent in the prepared worktree.",
			"Pass prompt_template together with task context.",
			"Do not assume this MCP call already started the worker.",
		},
	}
	return map[string]any{
		"required":            briefing.Required,
		"started":             briefing.Started,
		"leader_next_action":  briefing.LeaderNextAction,
		"warning":             briefing.Warning,
		"worker_id":           briefing.WorkerID,
		"task_id":             briefing.TaskID,
		"task_title":          briefing.TaskTitle,
		"worktree_path":       briefing.WorktreePath,
		"branch":              briefing.Branch,
		"dispatch_mode":       briefing.DispatchMode,
		"prompt_template":     briefing.PromptTemplate,
		"required_reads":      stringSliceToAny(briefing.RequiredReads),
		"recommended_mcp":     stringSliceToAny(briefing.RecommendedMCP),
		"recommended_skills":  stringSliceToAny(briefing.RecommendedSkills),
		"launch_instructions": stringSliceToAny(briefing.LaunchInstructions),
	}, nil
}

func stringSliceToAny(values []string) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}
