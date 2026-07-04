package bt

import (
	"context"
	"fmt"
	"strings"
)

// Registry holds named behavior trees.
type Registry struct {
	trees map[string]Node
}

func NewRegistry() *Registry {
	return &Registry{trees: make(map[string]Node)}
}

func (r *Registry) Register(name string, root Node) {
	r.trees[name] = root
}

func (r *Registry) Get(name string) (Node, bool) {
	t, ok := r.trees[name]
	return t, ok
}

// Tick ticks a named tree. Returns the root's status.
func (r *Registry) Tick(ctx context.Context, name string) (Status, error) {
	t, ok := r.trees[name]
	if !ok {
		return Failure, fmt.Errorf("tree %q not found", name)
	}
	return t.Tick(ctx), nil
}

// List returns all registered tree names.
func (r *Registry) List() []string {
	names := make([]string, 0, len(r.trees))
	for n := range r.trees {
		names = append(names, n)
	}
	return names
}

// ---------------------------------------------------------------------------
// Ticker — drives a named tree in a loop until Success/Failure, or until
// the context is cancelled. Returns the final status.
// ---------------------------------------------------------------------------

type Ticker struct {
	registry *Registry
}

func NewTicker(registry *Registry) *Ticker {
	return &Ticker{registry: registry}
}

func (t *Ticker) TickOnce(ctx context.Context, treeName string) (Status, error) {
	return t.registry.Tick(ctx, treeName)
}

// TickUntilDone ticks the named tree repeatedly until it returns
// Success, Failure, or the context is cancelled.
func (t *Ticker) TickUntilDone(ctx context.Context, treeName string) (Status, error) {
	for {
		select {
		case <-ctx.Done():
			return Failure, ctx.Err()
		default:
		}
		status, err := t.registry.Tick(ctx, treeName)
		if err != nil {
			return Failure, err
		}
		if status != Running {
			return status, nil
		}
	}
}

// String returns the tree names for display.
func (r *Registry) String() string {
	return fmt.Sprintf("Registry{trees: [%s]}", strings.Join(r.List(), ", "))
}
