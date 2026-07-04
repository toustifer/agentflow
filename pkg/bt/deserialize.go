package bt

import "encoding/json"

// TreeFile represents a JSON file with a named behavior tree.
// The Blackboard field holds initial data populated when the tree is loaded.
type TreeFile struct {
	Name       string         `json:"name"`
	Blackboard map[string]any `json:"blackboard,omitempty"`
	Tree       json.RawMessage `json:"tree"`
}

// DeserializeTree parses a TreeFile JSON document and builds the node tree.
// Returns the root Node and the initial blackboard data (may be nil).
// The caller should create a Blackboard from the returned data and pass it
// via ContextWithBlackboard before ticking.
func DeserializeTree(data []byte, reg *FactoryRegistry) (Node, map[string]any, error) {
	var tf TreeFile
	if err := json.Unmarshal(data, &tf); err != nil {
		return nil, nil, err
	}

	var rootCfg NodeConfig
	if err := json.Unmarshal(tf.Tree, &rootCfg); err != nil {
		return nil, nil, err
	}

	// Merge top-level blackboard initial data into the root config
	if len(tf.Blackboard) > 0 {
		if rootCfg.Blackboard == nil {
			rootCfg.Blackboard = make(map[string]any)
		}
		for k, v := range tf.Blackboard {
			if _, exists := rootCfg.Blackboard[k]; !exists {
				rootCfg.Blackboard[k] = v
			}
		}
	}

	node, err := reg.Build(rootCfg)
	if err != nil {
		return nil, nil, err
	}
	return node, tf.Blackboard, nil
}

// DeserializeNode parses a single NodeConfig JSON and builds it.
func DeserializeNode(data []byte, reg *FactoryRegistry) (Node, error) {
	var cfg NodeConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return reg.Build(cfg)
}
