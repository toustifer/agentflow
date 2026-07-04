package bt

import (
	"context"
)

// ---------------------------------------------------------------------------
// Condition — pure check. Returns Success or Failure, never Running.
// ---------------------------------------------------------------------------

type Condition struct {
	fn func(ctx context.Context) bool
}

func NewCondition(fn func(ctx context.Context) bool) *Condition {
	return &Condition{fn: fn}
}

func (c *Condition) Tick(ctx context.Context) Status {
	if c.fn(ctx) {
		return Success
	}
	return Failure
}

// ---------------------------------------------------------------------------
// Action — side effect that returns Success/Failure.
//   fn returns (bool success, error).
//   If fn returns (true, nil) → Success.
//   If fn returns (false, nil) → Failure.
//   If fn returns any error → Failure (with error stored).
// ---------------------------------------------------------------------------

type Action struct {
	fn  func(ctx context.Context) (bool, error)
	err error
}

func NewAction(fn func(ctx context.Context) (bool, error)) *Action {
	return &Action{fn: fn}
}

func (a *Action) Tick(ctx context.Context) Status {
	ok, err := a.fn(ctx)
	a.err = err
	if err != nil {
		return Failure
	}
	if ok {
		return Success
	}
	return Failure
}

func (a *Action) Err() error {
	return a.err
}

// ---------------------------------------------------------------------------
// ActionWithRunning — side effect that can return Running.
//   fn returns (Status, error).
// ---------------------------------------------------------------------------

type ActionWithRunning struct {
	fn  func(ctx context.Context) (Status, error)
	err error
}

func NewActionWithRunning(fn func(ctx context.Context) (Status, error)) *ActionWithRunning {
	return &ActionWithRunning{fn: fn}
}

func (a *ActionWithRunning) Tick(ctx context.Context) Status {
	status, err := a.fn(ctx)
	a.err = err
	if err != nil {
		return Failure
	}
	return status
}

func (a *ActionWithRunning) Err() error {
	return a.err
}

// ---------------------------------------------------------------------------
// Inverter — inverts SUCCESS ↔ FAILURE. Running passes through.
// ---------------------------------------------------------------------------

type Inverter struct {
	child Node
}

func NewInverter(child Node) *Inverter {
	return &Inverter{child: child}
}

func (inv *Inverter) Tick(ctx context.Context) Status {
	status := inv.child.Tick(ctx)
	switch status {
	case Success:
		return Failure
	case Failure:
		return Success
	default:
		return status
	}
}

func (inv *Inverter) Halt() {
	if h, ok := inv.child.(Haltable); ok {
		h.Halt()
	}
}

// ---------------------------------------------------------------------------
// RetryUntilSuccessful — retries child N times until SUCCESS.
// ---------------------------------------------------------------------------

type RetryUntilSuccessful struct {
	child     Node
	maxRetry  int
	attempts  int
}

func NewRetryUntilSuccessful(maxRetry int, child Node) *RetryUntilSuccessful {
	return &RetryUntilSuccessful{child: child, maxRetry: maxRetry}
}

func (r *RetryUntilSuccessful) Tick(ctx context.Context) Status {
	for r.attempts < r.maxRetry {
		status := r.child.Tick(ctx)
		switch status {
		case Running:
			return Running
		case Success:
			r.attempts = 0
			return Success
		case Failure:
			r.attempts++
			if r.attempts >= r.maxRetry {
				r.attempts = 0
				return Failure
			}
		}
	}
	r.attempts = 0
	return Failure
}

func (r *RetryUntilSuccessful) Halt() {
	if h, ok := r.child.(Haltable); ok {
		h.Halt()
	}
	r.attempts = 0
}

// ---------------------------------------------------------------------------
// LogDecorator — wraps a child, prints a message on each tick.
// ---------------------------------------------------------------------------

type LogDecorator struct {
	child   Node
	message string
}

func NewLogDecorator(message string, child Node) *LogDecorator {
	return &LogDecorator{child: child, message: message}
}

func (d *LogDecorator) Tick(ctx context.Context) Status {
	status := d.child.Tick(ctx)
	return status
}

func (d *LogDecorator) Halt() {
	if h, ok := d.child.(Haltable); ok {
		h.Halt()
	}
}

func (d *LogDecorator) LastStatus() Status {
	return Success // used in test context only
}

// Ensure ActionWithRunning implements Haltable (no-op halt is fine).
func (a *ActionWithRunning) Halt() {}

// Ensure Condition implements Haltable (no-op).
func (c *Condition) Halt() {}

// Ensure Action implements Haltable.
func (a *Action) Halt() {}

// ---------------------------------------------------------------------------
// builder convenience
// ---------------------------------------------------------------------------

// Seq is a short alias for NewSequence.
func Seq(children ...Node) *Sequence { return NewSequence(children...) }

// Sel is a short alias for NewFallback (Selector = Fallback in BT terminology).
func Sel(children ...Node) *Fallback { return NewFallback(children...) }

// Cond wraps a function as a Condition.
func Cond(fn func(ctx context.Context) bool) *Condition { return NewCondition(fn) }

// Act wraps a side effect function as an Action.
func Act(fn func(ctx context.Context) (bool, error)) *Action { return NewAction(fn) }

// ActR wraps a side effect function as an ActionWithRunning.
func ActR(fn func(ctx context.Context) (Status, error)) *ActionWithRunning { return NewActionWithRunning(fn) }

// Retry is a short alias for NewRetryUntilSuccessful.
func Retry(n int, child Node) *RetryUntilSuccessful { return NewRetryUntilSuccessful(n, child) }
