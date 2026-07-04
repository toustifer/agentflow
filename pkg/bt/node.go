// Package bt provides a lightweight BehaviorTree runtime inspired by BehaviorTree.CPP.
//
// Core concepts:
//   - Node: interface with Tick(ctx) => Status
//   - Status: Success, Failure, Running
//   - ControlNode: Sequence (all succeed), Fallback (first succeed)
//   - LeafNode: Condition (pure check), Action (side effect)
//   - ReactiveSequence/ReactiveFallback: re-evaluate children every tick
package bt

import "context"

// Status returned by Tick().
type Status int

const (
	Success Status = iota
	Failure
	Running
)

func (s Status) String() string {
	switch s {
	case Success:
		return "Success"
	case Failure:
		return "Failure"
	case Running:
		return "Running"
	default:
		return "Unknown"
	}
}

// Node is the core interface. Every node must implement Tick.
type Node interface {
	Tick(ctx context.Context) Status
}

// Haltable is implemented by nodes that need cleanup when interrupted.
type Haltable interface {
	Halt()
}

// NodeFunc wraps a function as a Node.
type NodeFunc func(ctx context.Context) Status

func (f NodeFunc) Tick(ctx context.Context) Status { return f(ctx) }

// StatusText returns a human-readable label.
func StatusText(s Status) string {
	return s.String()
}
