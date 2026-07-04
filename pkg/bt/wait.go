package bt

import "context"

// Wait returns Running for N ticks, then Success.
// Useful for simulation, delays, or pacing in a tree.
type Wait struct {
	maxTicks int
	count    int
}

// NewWait creates a Wait node that returns Running for maxTicks ticks.
func NewWait(maxTicks int) *Wait {
	return &Wait{maxTicks: maxTicks}
}

func (w *Wait) Tick(ctx context.Context) Status {
	w.count++
	if w.count >= w.maxTicks {
		w.count = 0
		return Success
	}
	return Running
}

func (w *Wait) Halt() {
	w.count = 0
}
