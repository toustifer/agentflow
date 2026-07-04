package bt

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func ctx() context.Context {
	return context.Background()
}

// ---------------------------------------------------------------------------
// Sequence tests
// ---------------------------------------------------------------------------

func TestSequenceAllSuccess(t *testing.T) {
	t.Parallel()

	var order []string
	s := NewSequence(
		NodeFunc(func(_ context.Context) Status {
			order = append(order, "A"); return Success
		}),
		NodeFunc(func(_ context.Context) Status {
			order = append(order, "B"); return Success
		}),
	)
	require.Equal(t, Success, s.Tick(ctx()))
	require.Equal(t, []string{"A", "B"}, order)
}

func TestSequenceFailureStops(t *testing.T) {
	t.Parallel()

	var order []string
	s := NewSequence(
		NodeFunc(func(_ context.Context) Status {
			order = append(order, "A"); return Success
		}),
		NodeFunc(func(_ context.Context) Status {
			order = append(order, "B"); return Failure
		}),
		NodeFunc(func(_ context.Context) Status {
			order = append(order, "C"); return Success
		}),
	)
	require.Equal(t, Failure, s.Tick(ctx()))
	require.Equal(t, []string{"A", "B"}, order)
}

func TestSequenceRunning(t *testing.T) {
	t.Parallel()

	var order []string
	s := NewSequence(
		NodeFunc(func(_ context.Context) Status {
			order = append(order, "A"); return Success
		}),
		NodeFunc(func(_ context.Context) Status {
			order = append(order, "B"); return Running
		}),
	)
	require.Equal(t, Running, s.Tick(ctx()))
	require.Equal(t, []string{"A", "B"}, order)

	// Second tick should resume at B
	require.Equal(t, Running, s.Tick(ctx()))
	require.Equal(t, []string{"A", "B", "B"}, order)

	// B completes
	s.children[1] = NodeFunc(func(_ context.Context) Status {
		order = append(order, "B-ok"); return Success
	})
	require.Equal(t, Success, s.Tick(ctx()))
	require.Equal(t, []string{"A", "B", "B", "B-ok"}, order)
}

// ---------------------------------------------------------------------------
// Fallback tests
// ---------------------------------------------------------------------------

func TestFallbackFirstSuccess(t *testing.T) {
	t.Parallel()

	f := NewFallback(
		NodeFunc(func(_ context.Context) Status {
			return Success
		}),
		NodeFunc(func(_ context.Context) Status {
			t.Fatal("should not be ticked")
			return Failure
		}),
	)
	require.Equal(t, Success, f.Tick(ctx()))
}

func TestFallbackAllFail(t *testing.T) {
	t.Parallel()

	f := NewFallback(
		NodeFunc(func(_ context.Context) Status { return Failure }),
		NodeFunc(func(_ context.Context) Status { return Failure }),
	)
	require.Equal(t, Failure, f.Tick(ctx()))
}

func TestFallbackFallthroughToSecond(t *testing.T) {
	t.Parallel()

	var order []string
	f := NewFallback(
		NodeFunc(func(_ context.Context) Status {
			order = append(order, "1-fail")
			return Failure
		}),
		NodeFunc(func(_ context.Context) Status {
			order = append(order, "2-ok")
			return Success
		}),
	)
	require.Equal(t, Success, f.Tick(ctx()))
	require.Equal(t, []string{"1-fail", "2-ok"}, order)
}

func TestFallbackRunning(t *testing.T) {
	t.Parallel()

	f := NewFallback(
		NodeFunc(func(_ context.Context) Status { return Failure }),
		NodeFunc(func(_ context.Context) Status { return Running }),
	)
	require.Equal(t, Running, f.Tick(ctx()))
	// resume at index 1
	require.Equal(t, Running, f.Tick(ctx()))
}

// ---------------------------------------------------------------------------
// ReactiveSequence tests
// ---------------------------------------------------------------------------

func TestReactiveSequenceAllSuccess(t *testing.T) {
	t.Parallel()

	rs := NewReactiveSequence(
		NewCondition(func(_ context.Context) bool { return true }),
		NewCondition(func(_ context.Context) bool { return true }),
	)
	// both conditions pass
	require.Equal(t, Success, rs.Tick(ctx()))
}

func TestReactiveSequenceFailure(t *testing.T) {
	t.Parallel()

	rs := NewReactiveSequence(
		NewCondition(func(_ context.Context) bool { return true }),
		NewCondition(func(_ context.Context) bool { return false }),
	)
	require.Equal(t, Failure, rs.Tick(ctx()))
}

// ---------------------------------------------------------------------------
// ReactiveFallback tests
// ---------------------------------------------------------------------------

func TestReactiveFallbackSuccess(t *testing.T) {
	t.Parallel()

	rf := NewReactiveFallback(
		NewCondition(func(_ context.Context) bool { return false }),
		NewCondition(func(_ context.Context) bool { return true }),
	)
	require.Equal(t, Success, rf.Tick(ctx()))
}

func TestReactiveFallbackAllFail(t *testing.T) {
	t.Parallel()

	rf := NewReactiveFallback(
		NewCondition(func(_ context.Context) bool { return false }),
		NewCondition(func(_ context.Context) bool { return false }),
	)
	require.Equal(t, Failure, rf.Tick(ctx()))
}

// ---------------------------------------------------------------------------
// Condition tests
// ---------------------------------------------------------------------------

func TestConditionTrue(t *testing.T) {
	t.Parallel()
	c := NewCondition(func(_ context.Context) bool { return true })
	require.Equal(t, Success, c.Tick(ctx()))
}

func TestConditionFalse(t *testing.T) {
	t.Parallel()
	c := NewCondition(func(_ context.Context) bool { return false })
	require.Equal(t, Failure, c.Tick(ctx()))
}

// ---------------------------------------------------------------------------
// Action tests
// ---------------------------------------------------------------------------

func TestActionSuccess(t *testing.T) {
	t.Parallel()
	a := NewAction(func(_ context.Context) (bool, error) { return true, nil })
	require.Equal(t, Success, a.Tick(ctx()))
}

func TestActionFailure(t *testing.T) {
	t.Parallel()
	a := NewAction(func(_ context.Context) (bool, error) { return false, nil })
	require.Equal(t, Failure, a.Tick(ctx()))
}

func TestActionError(t *testing.T) {
	t.Parallel()
	a := NewAction(func(_ context.Context) (bool, error) { return false, errors.New("boom") })
	require.Equal(t, Failure, a.Tick(ctx()))
	require.EqualError(t, a.Err(), "boom")
}

// ---------------------------------------------------------------------------
// Inverter tests
// ---------------------------------------------------------------------------

func TestInverter(t *testing.T) {
	t.Parallel()
	inv := NewInverter(NewCondition(func(_ context.Context) bool { return false }))
	require.Equal(t, Success, inv.Tick(ctx()))
}

// ---------------------------------------------------------------------------
// RetryUntilSuccessful tests
// ---------------------------------------------------------------------------

func TestRetrySucceedsEventually(t *testing.T) {
	t.Parallel()

	attempts := 0
	r := NewRetryUntilSuccessful(3, NodeFunc(func(_ context.Context) Status {
		attempts++
		if attempts >= 2 {
			return Success
		}
		return Failure
	}))
	require.Equal(t, Success, r.Tick(ctx()))
	require.Equal(t, 2, attempts)
}

func TestRetryExhausted(t *testing.T) {
	t.Parallel()

	r := NewRetryUntilSuccessful(2, NodeFunc(func(_ context.Context) Status {
		return Failure
	}))
	require.Equal(t, Failure, r.Tick(ctx()))
}

// ---------------------------------------------------------------------------
// Registry tests
// ---------------------------------------------------------------------------

func TestRegistryRoundTrip(t *testing.T) {
	t.Parallel()

	reg := NewRegistry()
	n := NewSequence(NewCondition(func(_ context.Context) bool { return true }))
	reg.Register("test", n)
	require.Contains(t, reg.List(), "test")

	status, err := reg.Tick(ctx(), "test")
	require.NoError(t, err)
	require.Equal(t, Success, status)
}

func TestRegistryNotFound(t *testing.T) {
	t.Parallel()

	reg := NewRegistry()
	_, err := reg.Tick(ctx(), "nope")
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// Ticker tests
// ---------------------------------------------------------------------------

func TestTickerTickOnce(t *testing.T) {
	t.Parallel()

	reg := NewRegistry()
	reg.Register("test", NewCondition(func(_ context.Context) bool { return true }))
	ticker := NewTicker(reg)
	status, err := ticker.TickOnce(ctx(), "test")
	require.NoError(t, err)
	require.Equal(t, Success, status)
}

func TestTickerTickUntilDone(t *testing.T) {
	t.Parallel()

	// Sequence with 3 nodes that all succeed
	reg := NewRegistry()
	reg.Register("three", NewSequence(
		NewCondition(func(_ context.Context) bool { return true }),
		NewCondition(func(_ context.Context) bool { return true }),
		NewCondition(func(_ context.Context) bool { return true }),
	))
	ticker := NewTicker(reg)
	status, err := ticker.TickUntilDone(ctx(), "three")
	require.NoError(t, err)
	require.Equal(t, Success, status)
}

func TestSequenceResetsOnNewTick(t *testing.T) {
	t.Parallel()

	// After a complete run (all succeed), next tick restarts from beginning
	calls := 0
	s := NewSequence(
		NodeFunc(func(_ context.Context) Status { calls++; return Success }),
		NodeFunc(func(_ context.Context) Status { calls++; return Success }),
	)

	require.Equal(t, Success, s.Tick(ctx()))
	require.Equal(t, 2, calls)

	require.Equal(t, Success, s.Tick(ctx()))
	require.Equal(t, 4, calls)
}

func TestFallbackResetsOnNewTick(t *testing.T) {
	t.Parallel()

	calls := 0
	f := NewFallback(
		NodeFunc(func(_ context.Context) Status { calls++; return Success }),
		NodeFunc(func(_ context.Context) Status { calls++; return Success }),
	)

	require.Equal(t, Success, f.Tick(ctx()))
	require.Equal(t, 1, calls) // only first called

	require.Equal(t, Success, f.Tick(ctx()))
	require.Equal(t, 2, calls) // first called again
}
