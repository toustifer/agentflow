package server

import (
	"context"
	"fmt"
)

// phaseProviderResult is the normalized shape of project_next_steps data
// that crosses into the Python BT runtime.
type phaseProviderResult struct {
	Phase          string           `json:"phase"`
	PhaseName      string           `json:"phase_name"`
	Progress       string           `json:"progress"`
	Actions        []string         `json:"actions"`
	NextTasks      []map[string]any `json:"next_tasks,omitempty"`
	ActiveTasks    []map[string]any `json:"active_tasks,omitempty"`
	StuckTasks     []map[string]any `json:"stuck_tasks,omitempty"`
	HasNextTasks   bool             `json:"has_next_tasks"`
	HasActiveTasks bool             `json:"has_active_tasks"`
	HasStuckTasks  bool             `json:"has_stuck_tasks"`
}

// buildPhaseProviderResult converts handleProjectNextSteps output into the
// stable phase payload sent to Python refresh_phase.
func (s *Server) buildPhaseProviderResult(ctx context.Context, namespaceID string) (phaseProviderResult, error) {
	result, err := s.handleProjectNextSteps(ctx, map[string]any{"namespace_id": namespaceID})
	if err != nil {
		return phaseProviderResult{}, err
	}

	nextTasks := toMapSlice(result["next_tasks"])
	activeTasks := toMapSlice(result["active_tasks"])
	stuckTasks := toMapSlice(result["stuck_tasks"])

	return phaseProviderResult{
		Phase:          toString(result["phase"]),
		PhaseName:      toString(result["phase_name"]),
		Progress:       toString(result["progress"]),
		Actions:        toStringSlice(result["actions"]),
		NextTasks:      nextTasks,
		ActiveTasks:    activeTasks,
		StuckTasks:     stuckTasks,
		HasNextTasks:   len(nextTasks) > 0,
		HasActiveTasks: len(activeTasks) > 0,
		HasStuckTasks:  len(stuckTasks) > 0,
	}, nil
}

func (p phaseProviderResult) toMap() map[string]any {
	return map[string]any{
		"phase":            p.Phase,
		"phase_name":       p.PhaseName,
		"progress":         p.Progress,
		"actions":          p.Actions,
		"next_tasks":       p.NextTasks,
		"active_tasks":     p.ActiveTasks,
		"stuck_tasks":      p.StuckTasks,
		"has_next_tasks":   p.HasNextTasks,
		"has_active_tasks": p.HasActiveTasks,
		"has_stuck_tasks":  p.HasStuckTasks,
	}
}

var _ = fmt.Sprintf
