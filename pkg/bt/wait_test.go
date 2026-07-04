package bt

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWaitReturnsRunning(t *testing.T) {
	w := NewWait(3)
	require.Equal(t, Running, w.Tick(context.Background()))
	require.Equal(t, Running, w.Tick(context.Background()))
	require.Equal(t, Success, w.Tick(context.Background()))
}

func TestWaitSingleTick(t *testing.T) {
	w := NewWait(1)
	require.Equal(t, Success, w.Tick(context.Background()))
}

func TestWaitResetsAfterSuccess(t *testing.T) {
	w := NewWait(2)
	require.Equal(t, Running, w.Tick(context.Background()))
	require.Equal(t, Success, w.Tick(context.Background()))

	// Next tick starts over
	require.Equal(t, Running, w.Tick(context.Background()))
}

func TestWaitHaltResets(t *testing.T) {
	w := NewWait(5)
	require.Equal(t, Running, w.Tick(context.Background()))
	w.Halt()

	// After halt, resets to 0
	require.Equal(t, Running, w.Tick(context.Background()))
	require.Equal(t, Running, w.Tick(context.Background()))
	require.Equal(t, Running, w.Tick(context.Background()))
	require.Equal(t, Running, w.Tick(context.Background()))
	require.Equal(t, Success, w.Tick(context.Background()))
}

func TestWaitJSONDeserialize(t *testing.T) {
	reg := NewFactoryRegistry()
	RegisterDefaultNodes(reg)

	node, err := DeserializeNode([]byte(`{"type": "Wait", "properties": {"ticks": 3}}`), reg)
	require.NoError(t, err)

	ctx := context.Background()
	require.Equal(t, Running, node.Tick(ctx))
	require.Equal(t, Running, node.Tick(ctx))
	require.Equal(t, Success, node.Tick(ctx))
}

func TestWaitJSONDefaultTicks(t *testing.T) {
	reg := NewFactoryRegistry()
	RegisterDefaultNodes(reg)

	node, err := DeserializeNode([]byte(`{"type": "Wait"}`), reg)
	require.NoError(t, err)

	require.Equal(t, Success, node.Tick(context.Background()))
}
