package server

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/toustifer/agentflow/pkg/bt"
)

func btBridgeForRequest(s *Server) *BTBridge {
	if globalBTBridge == nil {
		bridge := NewBTBridge()
		if err := bridge.Start(s); err == nil {
			globalBTBridge = bridge
		}
	}
	return globalBTBridge
}

func (s *Server) handleBTListTrees(ctx context.Context, input map[string]any) (map[string]any, error) {
	bridge := btBridgeForRequest(s)
	if bridge == nil {
		_, treeReg, _, err := ensureRegistries()
		if err != nil {
			return nil, err
		}
		names := treeReg.List()
		return map[string]any{"trees": names, "count": len(names)}, nil
	}
	result, err := bridge.RPC("list_trees", map[string]any{})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Server) handleBTShowTree(ctx context.Context, input map[string]any) (map[string]any, error) {
	bridge := btBridgeForRequest(s)
	if bridge == nil {
		return nil, fmt.Errorf("bt_service not available")
	}
	name, err := requiredString(input, "tree")
	if err != nil {
		return nil, err
	}
	return bridge.RPC("show_tree", map[string]any{"tree_name": name})
}

func (s *Server) handleBTValidateTree(ctx context.Context, input map[string]any) (map[string]any, error) {
	bridge := btBridgeForRequest(s)
	jsonStr, err := requiredString(input, "json")
	if err != nil {
		return nil, err
	}
	if bridge == nil {
		reg, _, _, err := ensureRegistries()
		if err != nil {
			return nil, err
		}
		if err := bt.ValidateTreeJSON([]byte(jsonStr), reg); err != nil {
			return map[string]any{"valid": false, "errors": []map[string]any{{"path": "$.tree", "code": "validation_error", "message": err.Error()}}}, nil
		}
		return map[string]any{"valid": true, "errors": []any{}}, nil
	}
	var treeJSON map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &treeJSON); err != nil {
		return map[string]any{"valid": false, "errors": []map[string]any{{"path": "$", "code": "invalid_json", "message": err.Error()}}}, nil
	}
	return bridge.RPC("validate", map[string]any{"tree_json": treeJSON})
}

func (s *Server) handleBTTick(ctx context.Context, input map[string]any) (map[string]any, error) {
	bridge := btBridgeForRequest(s)
	if bridge == nil {
		return nil, fmt.Errorf("bt_service not available")
	}
	treeName, _ := input["tree"].(string)
	customBB, _ := input["input"].(map[string]any)
	namespaceID, _ := input["namespace_id"].(string)
	bb := cloneMap(customBB)
	if bb == nil {
		bb = map[string]any{}
	}
	if namespaceID != "" {
		bb["namespace_id"] = namespaceID
	}
	result, err := bridge.RPC("tick", map[string]any{
		"tree_name":  treeName,
		"blackboard": bb,
		"options":    map[string]any{"return_blackboard": true},
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"tree":       treeName,
		"status":     result["status"],
		"outputs":    result["outputs"],
		"blackboard": result["blackboard"],
	}, nil
}

var globalBTBridge *BTBridge

func cloneMap(src map[string]any) map[string]any {
	if src == nil {
		return nil
	}
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
