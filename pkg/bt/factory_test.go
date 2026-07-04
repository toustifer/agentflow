package bt

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFactoryRegisterAndBuildSequence(t *testing.T) {
	reg := NewFactoryRegistry()
	RegisterDefaultNodes(reg)
	reg.RegisterCondition("yes", func(ctx context.Context) bool { return true })

	node, err := reg.Build(NodeConfig{
		Type: "Sequence",
		Children: rawMessages(
			`{"type": "Condition", "properties": {"fn": "yes"}}`,
			`{"type": "Condition", "properties": {"fn": "yes"}}`,
		),
	})
	require.NoError(t, err)
	require.Equal(t, Success, node.Tick(context.Background()))
}

func TestFactoryUnknownType(t *testing.T) {
	reg := NewFactoryRegistry()
	_, err := reg.Build(NodeConfig{Type: "NonExistent"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown node type")
}

func TestFactoryInverterWrongChildCount(t *testing.T) {
	reg := NewFactoryRegistry()
	RegisterDefaultNodes(reg)
	reg.RegisterCondition("yes", func(ctx context.Context) bool { return true })

	_, err := reg.Build(NodeConfig{
		Type: "Inverter",
		Children: rawMessages(
			`{"type": "Condition", "properties": {"fn": "yes"}}`,
			`{"type": "Condition", "properties": {"fn": "yes"}}`,
		),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "requires exactly 1 child")
}

func TestFactoryConditionMissingFn(t *testing.T) {
	reg := NewFactoryRegistry()
	RegisterDefaultNodes(reg)
	_, err := reg.Build(NodeConfig{Type: "Condition", Properties: map[string]any{}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "requires 'fn' property")
}

func TestFactoryActionMissingFn(t *testing.T) {
	reg := NewFactoryRegistry()
	RegisterDefaultNodes(reg)
	_, err := reg.Build(NodeConfig{Type: "Action", Properties: map[string]any{}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "requires 'fn' property")
}

func TestFactoryRegisterConditionAndUse(t *testing.T) {
	reg := NewFactoryRegistry()
	RegisterDefaultNodes(reg)

	called := false
	reg.RegisterCondition("custom_cond", func(ctx context.Context) bool {
		called = true
		return true
	})

	node, err := reg.Build(NodeConfig{
		Type:       "Condition",
		Properties: map[string]any{"fn": "custom_cond"},
	})
	require.NoError(t, err)
	require.Equal(t, Success, node.Tick(context.Background()))
	require.True(t, called)
}

func TestFactoryRegisterActionAndUse(t *testing.T) {
	reg := NewFactoryRegistry()
	RegisterDefaultNodes(reg)

	called := false
	reg.RegisterAction("custom_act", func(ctx context.Context) (bool, error) {
		called = true
		return true, nil
	})

	node, err := reg.Build(NodeConfig{
		Type:       "Action",
		Properties: map[string]any{"fn": "custom_act"},
	})
	require.NoError(t, err)
	require.Equal(t, Success, node.Tick(context.Background()))
	require.True(t, called)
}

func TestFactoryList(t *testing.T) {
	reg := NewFactoryRegistry()
	RegisterDefaultNodes(reg)

	names := reg.List()
	require.Contains(t, names, "Sequence")
	require.Contains(t, names, "Fallback")
	require.Contains(t, names, "ReactiveSequence")
	require.Contains(t, names, "ReactiveFallback")
	require.Contains(t, names, "Inverter")
	require.Contains(t, names, "Retry")
	require.Contains(t, names, "Condition")
	require.Contains(t, names, "Action")
}

// rawMessages converts JSON strings to []json.RawMessage
func rawMessages(jsons ...string) []json.RawMessage {
	msgs := make([]json.RawMessage, len(jsons))
	for i, j := range jsons {
		msgs[i] = json.RawMessage(j)
	}
	return msgs
}
