package bt

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// NodeConfig is the JSON-serializable configuration for a node.
type NodeConfig struct {
	Type       string            `json:"type"`
	Name       string            `json:"name,omitempty"`
	Children   []json.RawMessage `json:"children,omitempty"`
	Properties map[string]any    `json:"properties,omitempty"`
	Blackboard map[string]any    `json:"blackboard,omitempty"`
}

// NodeFactory creates a Node from config.
// children are already-resolved child nodes for control/decorator nodes.
// reg is passed for Condition/Action function lookups.
type NodeFactory func(cfg NodeConfig, children []Node, reg *FactoryRegistry) (Node, error)

// FactoryRegistry maps node type names to constructors.
// It also holds named condition and action functions for the generic
// Condition/Action node types, and an optional tree Registry for SubTree.
type FactoryRegistry struct {
	mu          sync.RWMutex
	factories   map[string]NodeFactory
	condFns     map[string]func(ctx context.Context) bool
	actFns      map[string]func(ctx context.Context) (bool, error)
	treeRegistry *Registry
}

// NewFactoryRegistry creates an empty registry.
func NewFactoryRegistry() *FactoryRegistry {
	return &FactoryRegistry{
		factories: make(map[string]NodeFactory),
		condFns:   make(map[string]func(ctx context.Context) bool),
		actFns:    make(map[string]func(ctx context.Context) (bool, error)),
	}
}

// SetTreeRegistry attaches a tree Registry for SubTree resolution.
func (f *FactoryRegistry) SetTreeRegistry(tr *Registry) {
	f.mu.Lock()
	f.treeRegistry = tr
	f.mu.Unlock()
}

// TreeRegistry returns the attached tree Registry, or nil.
func (f *FactoryRegistry) TreeRegistry() *Registry {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.treeRegistry
}

// Register registers a named node type constructor.
func (f *FactoryRegistry) Register(typeName string, factory NodeFactory) {
	f.mu.Lock()
	f.factories[typeName] = factory
	f.mu.Unlock()
}

// RegisterCondition registers a named condition function for use by the
// generic "Condition" node type.
func (f *FactoryRegistry) RegisterCondition(name string, fn func(ctx context.Context) bool) {
	f.mu.Lock()
	f.condFns[name] = fn
	f.mu.Unlock()
}

// RegisterAction registers a named action function for use by the
// generic "Action" node type.
func (f *FactoryRegistry) RegisterAction(name string, fn func(ctx context.Context) (bool, error)) {
	f.mu.Lock()
	f.actFns[name] = fn
	f.mu.Unlock()
}

// Build recursively constructs a node tree from a NodeConfig.
func (f *FactoryRegistry) Build(cfg NodeConfig) (Node, error) {
	f.mu.RLock()
	factory, ok := f.factories[cfg.Type]
	f.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown node type: %q", cfg.Type)
	}

	// Recursively build children
	children := make([]Node, 0, len(cfg.Children))
	for i, raw := range cfg.Children {
		var childCfg NodeConfig
		if err := json.Unmarshal(raw, &childCfg); err != nil {
			return nil, fmt.Errorf("child %d: %w", i, err)
		}
		child, err := f.Build(childCfg)
		if err != nil {
			return nil, fmt.Errorf("child %d: %w", i, err)
		}
		children = append(children, child)
	}

	return factory(cfg, children, f)
}

// List returns all registered node type names.
func (f *FactoryRegistry) List() []string {
	f.mu.RLock()
	names := make([]string, 0, len(f.factories))
	for n := range f.factories {
		names = append(names, n)
	}
	f.mu.RUnlock()
	return names
}

// RegisterDefaultNodes registers all built-in node types (control nodes,
// decorators, generic Condition/Action) into the registry.
func RegisterDefaultNodes(reg *FactoryRegistry) {
	// Control nodes
	reg.Register("Sequence", func(cfg NodeConfig, children []Node, reg *FactoryRegistry) (Node, error) {
		return NewSequence(children...), nil
	})
	reg.Register("Fallback", func(cfg NodeConfig, children []Node, reg *FactoryRegistry) (Node, error) {
		return NewFallback(children...), nil
	})
	reg.Register("ReactiveSequence", func(cfg NodeConfig, children []Node, reg *FactoryRegistry) (Node, error) {
		return NewReactiveSequence(children...), nil
	})
	reg.Register("ReactiveFallback", func(cfg NodeConfig, children []Node, reg *FactoryRegistry) (Node, error) {
		return NewReactiveFallback(children...), nil
	})

	// Decorators
	reg.Register("Inverter", func(cfg NodeConfig, children []Node, reg *FactoryRegistry) (Node, error) {
		if len(children) != 1 {
			return nil, fmt.Errorf("Inverter requires exactly 1 child, got %d", len(children))
		}
		return NewInverter(children[0]), nil
	})
	reg.Register("Retry", func(cfg NodeConfig, children []Node, reg *FactoryRegistry) (Node, error) {
		if len(children) != 1 {
			return nil, fmt.Errorf("Retry requires exactly 1 child, got %d", len(children))
		}
		maxRetry := 3
		if v, ok := cfg.Properties["max_retry"].(float64); ok {
			maxRetry = int(v)
		}
		return NewRetryUntilSuccessful(maxRetry, children[0]), nil
	})

	// Generic Condition — looks up fn from registered condition functions
	reg.Register("Condition", func(cfg NodeConfig, children []Node, reg *FactoryRegistry) (Node, error) {
		fnName, _ := cfg.Properties["fn"].(string)
		if fnName == "" {
			return nil, fmt.Errorf("Condition requires 'fn' property")
		}
		reg.mu.RLock()
		fn, ok := reg.condFns[fnName]
		reg.mu.RUnlock()
		if !ok {
			return nil, fmt.Errorf("unknown condition function: %q", fnName)
		}
		return NewCondition(fn), nil
	})

	// Generic Action — looks up fn from registered action functions
	reg.Register("Action", func(cfg NodeConfig, children []Node, reg *FactoryRegistry) (Node, error) {
		fnName, _ := cfg.Properties["fn"].(string)
		if fnName == "" {
			return nil, fmt.Errorf("Action requires 'fn' property")
		}
		reg.mu.RLock()
		fn, ok := reg.actFns[fnName]
		reg.mu.RUnlock()
		if !ok {
			return nil, fmt.Errorf("unknown action function: %q", fnName)
		}
		return NewAction(fn), nil
	})

	// SubTree — embeds a registered named tree
	reg.Register("SubTree", func(cfg NodeConfig, children []Node, reg *FactoryRegistry) (Node, error) {
		treeName, _ := cfg.Properties["tree"].(string)
		if treeName == "" {
			return nil, fmt.Errorf("SubTree requires 'tree' property")
		}
		tr := reg.TreeRegistry()
		if tr == nil {
			return nil, fmt.Errorf("SubTree: no tree registry set on FactoryRegistry")
		}
		return NewSubTree(treeName, tr), nil
	})

	// Wait — returns Running for N ticks
	reg.Register("Wait", func(cfg NodeConfig, children []Node, reg *FactoryRegistry) (Node, error) {
		ticks := 1
		if v, ok := cfg.Properties["ticks"].(float64); ok {
			ticks = int(v)
		}
		return NewWait(ticks), nil
	})

	// Log — decorator that logs a message on each tick
	reg.Register("Log", func(cfg NodeConfig, children []Node, reg *FactoryRegistry) (Node, error) {
		if len(children) != 1 {
			return nil, fmt.Errorf("Log requires exactly 1 child, got %d", len(children))
		}
		message, _ := cfg.Properties["message"].(string)
		return NewLogDecorator(message, children[0]), nil
	})
}
