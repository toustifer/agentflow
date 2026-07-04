package bt

import "context"

// ---------------------------------------------------------------------------
// Sequence — ticks children in order.
//   FAILURE or RUNNING → return immediately, remember position.
//   All children SUCCESS → SUCCESS.
// ---------------------------------------------------------------------------

type Sequence struct {
	children []Node
	index    int
}

func NewSequence(children ...Node) *Sequence {
	return &Sequence{children: children}
}

func (s *Sequence) Tick(ctx context.Context) Status {
	for s.index < len(s.children) {
		status := s.children[s.index].Tick(ctx)
		switch status {
		case Running:
			return Running
		case Failure:
			s.index = 0
			return Failure
		case Success:
			s.index++
		}
	}
	// All children completed successfully
	s.index = 0
	return Success
}

func (s *Sequence) Halt() {
	if s.index < len(s.children) {
		if h, ok := s.children[s.index].(Haltable); ok {
			h.Halt()
		}
	}
	s.index = 0
}

// ---------------------------------------------------------------------------
// Fallback — tries each child until one succeeds.
//   SUCCESS or RUNNING → return immediately, remember position.
//   All children FAILURE → FAILURE.
// ---------------------------------------------------------------------------

type Fallback struct {
	children []Node
	index    int
}

func NewFallback(children ...Node) *Fallback {
	return &Fallback{children: children}
}

func (f *Fallback) Tick(ctx context.Context) Status {
	for f.index < len(f.children) {
		status := f.children[f.index].Tick(ctx)
		switch status {
		case Running:
			return Running
		case Success:
			f.index = 0
			return Success
		case Failure:
			f.index++
		}
	}
	f.index = 0
	return Failure
}

func (f *Fallback) Halt() {
	if f.index < len(f.children) {
		if h, ok := f.children[f.index].(Haltable); ok {
			h.Halt()
		}
	}
	f.index = 0
}

// ---------------------------------------------------------------------------
// ReactiveSequence — ticks ALL children every tick.
//   Resets non-RUNNING children each tick.
//   First FAILURE → return FAILURE.
//   All children SUCCESS → SUCCESS.
// ---------------------------------------------------------------------------

type ReactiveSequence struct {
	children []Node
}

func NewReactiveSequence(children ...Node) *ReactiveSequence {
	return &ReactiveSequence{children: children}
}

func (rs *ReactiveSequence) Tick(ctx context.Context) Status {
	anyRunning := false
	for _, child := range rs.children {
		status := child.Tick(ctx)
		switch status {
		case Running:
			anyRunning = true
		case Failure:
			return Failure
		}
	}
	if anyRunning {
		return Running
	}
	return Success
}

func (rs *ReactiveSequence) Halt() {
	for _, child := range rs.children {
		if h, ok := child.(Haltable); ok {
			h.Halt()
		}
	}
}

// ---------------------------------------------------------------------------
// ReactiveFallback — ticks ALL children every tick.
//   Resets non-RUNNING children each tick.
//   First child that is SUCCESS → SUCCESS.
//   All children FAILURE → FAILURE.
// ---------------------------------------------------------------------------

type ReactiveFallback struct {
	children []Node
}

func NewReactiveFallback(children ...Node) *ReactiveFallback {
	return &ReactiveFallback{children: children}
}

func (rf *ReactiveFallback) Tick(ctx context.Context) Status {
	anyRunning := false
	allFailed := true
	for _, child := range rf.children {
		status := child.Tick(ctx)
		switch status {
		case Running:
			anyRunning = true
			allFailed = false
		case Success:
			return Success
		case Failure:
			// continue checking remaining
		}
	}
	if anyRunning {
		return Running
	}
	if allFailed {
		return Failure
	}
	return Failure
}

func (rf *ReactiveFallback) Halt() {
	for _, child := range rf.children {
		if h, ok := child.(Haltable); ok {
			h.Halt()
		}
	}
}
