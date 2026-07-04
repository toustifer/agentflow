package bt

import (
	"context"
	"sync"
)

type bbKey struct{}

// Blackboard is a thread-safe key-value store for sharing data between nodes.
// Supports a parent chain for SubTree data passing: Get/Has traverse up the chain.
type Blackboard struct {
	mu     sync.RWMutex
	data   map[string]any
	parent *Blackboard
}

// NewBlackboard creates an empty Blackboard.
func NewBlackboard() *Blackboard {
	return &Blackboard{data: make(map[string]any)}
}

// Set stores a value under key.
func (bb *Blackboard) Set(key string, value any) {
	bb.mu.Lock()
	bb.data[key] = value
	bb.mu.Unlock()
}

// Get retrieves a value. Traverses the parent chain if not found locally.
func (bb *Blackboard) Get(key string) (any, bool) {
	bb.mu.RLock()
	val, ok := bb.data[key]
	bb.mu.RUnlock()
	if ok {
		return val, true
	}
	if bb.parent != nil {
		return bb.parent.Get(key)
	}
	return nil, false
}

// Has checks if a key exists. Traverses the parent chain.
func (bb *Blackboard) Has(key string) bool {
	bb.mu.RLock()
	_, ok := bb.data[key]
	bb.mu.RUnlock()
	if ok {
		return true
	}
	if bb.parent != nil {
		return bb.parent.Has(key)
	}
	return false
}

// GetString returns the string value for a key, or "" if missing or wrong type.
func (bb *Blackboard) GetString(key string) string {
	v, ok := bb.Get(key)
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

// GetBool returns the bool value for a key, or false if missing or wrong type.
func (bb *Blackboard) GetBool(key string) bool {
	v, ok := bb.Get(key)
	if !ok {
		return false
	}
	b, _ := v.(bool)
	return b
}

// SetParent sets the parent blackboard for chain traversal.
func (bb *Blackboard) SetParent(parent *Blackboard) {
	bb.mu.Lock()
	bb.parent = parent
	bb.mu.Unlock()
}

// Parent returns the parent blackboard (nil for root).
func (bb *Blackboard) Parent() *Blackboard {
	bb.mu.RLock()
	defer bb.mu.RUnlock()
	return bb.parent
}

// ContextWithBlackboard stores a Blackboard in the context.
func ContextWithBlackboard(ctx context.Context, bb *Blackboard) context.Context {
	return context.WithValue(ctx, bbKey{}, bb)
}

// BlackboardFromContext retrieves the Blackboard from context. Returns nil if not set.
func BlackboardFromContext(ctx context.Context) *Blackboard {
	bb, _ := ctx.Value(bbKey{}).(*Blackboard)
	return bb
}
