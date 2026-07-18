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
	FocusedDAGID   string           `json:"focused_dag_id,omitempty"`
	FocusSource    string           `json:"focus_source,omitempty"`
}

// buildPhaseProviderResult converts handleProjectNextSteps output into the
// stable phase payload sent to Python refresh_phase.
// dagID may be empty (single-auto or multi-error via next_steps policy).
func (s *Server) buildPhaseProviderResult(ctx context.Context, namespaceID, dagID string) (phaseProviderResult, error) {
	input := map[string]any{"namespace_id": namespaceID}
	if dagID != "" {
		input["dag_id"] = dagID
	}
	result, err := s.handleProjectNextSteps(ctx, input)
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
		FocusedDAGID:   toString(result["focused_dag_id"]),
		FocusSource:    toString(result["focus_source"]),
	}, nil
}

func (p phaseProviderResult) toMap() map[string]any {
	m := map[string]any{
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
	if p.FocusedDAGID != "" {
		m["focused_dag_id"] = p.FocusedDAGID
	}
	if p.FocusSource != "" {
		m["focus_source"] = p.FocusSource
	}
	return m
}

var _ = fmt.Sprintf
