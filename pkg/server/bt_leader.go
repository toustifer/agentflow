package server

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/toustifer/agentflow/pkg/bt"
)

// leaderTreeHolder caches the Go registries for fallback and tooling.
type leaderTreeHolder struct {
	mu          sync.Mutex
	root        bt.Node
	factoryReg  *bt.FactoryRegistry
	treeReg     *bt.Registry
	treeSources map[string]json.RawMessage
	initErr     error
}

var globalHolder leaderTreeHolder

func ensureRegistries() (*bt.FactoryRegistry, *bt.Registry, map[string]json.RawMessage, error) {
	globalHolder.mu.Lock()
	defer globalHolder.mu.Unlock()

	if globalHolder.initErr != nil {
		return nil, nil, nil, globalHolder.initErr
	}
	if globalHolder.root != nil {
		return globalHolder.factoryReg, globalHolder.treeReg, globalHolder.treeSources, nil
	}

	reg := bt.NewFactoryRegistry()
	bt.RegisterDefaultNodes(reg)
	RegisterBuiltinNodes(reg)

	treeReg := bt.NewRegistry()
	reg.SetTreeRegistry(treeReg)

	root, _, err := bt.DeserializeTree([]byte(leaderDefaultJSON), reg)
	if err != nil {
		globalHolder.initErr = fmt.Errorf("build leader tree: %w", err)
		return nil, nil, nil, globalHolder.initErr
	}
	treeReg.Register("leader-default", root)

	sources := map[string]json.RawMessage{
		"leader-default": json.RawMessage(leaderDefaultJSON),
	}

	globalHolder.factoryReg = reg
	globalHolder.treeReg = treeReg
	globalHolder.root = root
	globalHolder.treeSources = sources
	return reg, treeReg, sources, nil
}

// handleLeaderTick now prefers the Python BT runtime.
// Go remains the source of truth for project_next_steps; Python owns BT routing.
func (s *Server) handleLeaderTick(ctx context.Context, input map[string]any) (map[string]any, error) {
	namespaceID, err := requiredString(input, "namespace_id")
	if err != nil {
		return nil, err
	}
	dagID, err := optionalString(input, "dag_id")
	if err != nil {
		return nil, err
	}
	if _, err := s.engine.GetNamespace(ctx, namespaceID); err != nil {
		return nil, err
	}

	phaseData, err := s.buildPhaseProviderResult(ctx, namespaceID, dagID)
	if err != nil {
		return nil, err
	}

	bridge := btBridgeForRequest(s)
	if bridge == nil {
		// Fallback to prior Go behavior if Python is unavailable.
		out := map[string]any{
			"tree_status":  statusFromPhase(phaseData.Phase),
			"phase":        phaseData.Phase,
			"phase_name":   phaseData.PhaseName,
			"progress":     phaseData.Progress,
			"actions":      phaseData.Actions,
			"next_tasks":   phaseData.NextTasks,
			"active_tasks": phaseData.ActiveTasks,
			"stuck_tasks":  phaseData.StuckTasks,
		}
		if phaseData.FocusedDAGID != "" {
			out["focused_dag_id"] = phaseData.FocusedDAGID
		}
		if phaseData.FocusSource != "" {
			out["focus_source"] = phaseData.FocusSource
		}
		return out, nil
	}

	bb := map[string]any{
		"namespace_id": namespaceID,
		"phase_data":   phaseData.toMap(),
	}
	if dagID != "" {
		bb["dag_id"] = dagID
	} else if phaseData.FocusedDAGID != "" {
		bb["dag_id"] = phaseData.FocusedDAGID
	}
	result, err := bridge.RPC("tick", map[string]any{
		"tree_name":  "leader-default",
		"blackboard": bb,
		"options":    map[string]any{"return_blackboard": true},
	})
	if err != nil {
		return nil, err
	}

	outputs, _ := result["outputs"].(map[string]any)
	if outputs == nil {
		outputs = map[string]any{}
	}

	response := map[string]any{
		"tree_status": result["status"],
		"phase":       toString(outputs["phase"]),
		"phase_name":  toString(outputs["phase_name"]),
		"progress":    toString(outputs["progress"]),
		"actions":     toStringSlice(outputs["actions"]),
	}
	if phaseData.FocusedDAGID != "" {
		response["focused_dag_id"] = phaseData.FocusedDAGID
	}
	if phaseData.FocusSource != "" {
		response["focus_source"] = phaseData.FocusSource
	}
	if nextTasks := toMapSlice(outputs["next_tasks"]); len(nextTasks) > 0 {
		response["next_tasks"] = nextTasks
	}
	if activeTasks := toMapSlice(outputs["active_tasks"]); len(activeTasks) > 0 {
		response["active_tasks"] = activeTasks
	}
	if stuckTasks := toMapSlice(outputs["stuck_tasks"]); len(stuckTasks) > 0 {
		response["stuck_tasks"] = stuckTasks
	}
	return response, nil
}

func statusFromPhase(phase string) string {
	if phase == "" {
		return "failure"
	}
	return "success"
}

func toString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func toStringSlice(v any) []string {
	if list, ok := v.([]string); ok {
		return list
	}
	if list, ok := v.([]any); ok {
		out := make([]string, 0, len(list))
		for _, item := range list {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}
