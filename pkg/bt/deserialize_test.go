package bt

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeserializeSequenceAllSuccess(t *testing.T) {
	reg := NewFactoryRegistry()
	RegisterDefaultNodes(reg)
	reg.RegisterCondition("yes", func(ctx context.Context) bool { return true })

	node, _, err := DeserializeTree([]byte(`{
		"tree": {
			"type": "Sequence",
			"children": [
				{"type": "Condition", "properties": {"fn": "yes"}},
				{"type": "Condition", "properties": {"fn": "yes"}}
			]
		}
	}`), reg)
	require.NoError(t, err)
	require.Equal(t, Success, node.Tick(context.Background()))
}

func TestDeserializeSequenceFailure(t *testing.T) {
	reg := NewFactoryRegistry()
	RegisterDefaultNodes(reg)
	reg.RegisterCondition("yes", func(ctx context.Context) bool { return true })
	reg.RegisterCondition("no", func(ctx context.Context) bool { return false })

	node, _, err := DeserializeTree([]byte(`{
		"tree": {
			"type": "Sequence",
			"children": [
				{"type": "Condition", "properties": {"fn": "yes"}},
				{"type": "Condition", "properties": {"fn": "no"}}
			]
		}
	}`), reg)
	require.NoError(t, err)
	require.Equal(t, Failure, node.Tick(context.Background()))
}

func TestDeserializeFallback(t *testing.T) {
	reg := NewFactoryRegistry()
	RegisterDefaultNodes(reg)
	reg.RegisterCondition("yes", func(ctx context.Context) bool { return true })
	reg.RegisterCondition("no", func(ctx context.Context) bool { return false })

	node, _, err := DeserializeTree([]byte(`{
		"tree": {
			"type": "Fallback",
			"children": [
				{"type": "Condition", "properties": {"fn": "no"}},
				{"type": "Condition", "properties": {"fn": "yes"}}
			]
		}
	}`), reg)
	require.NoError(t, err)
	require.Equal(t, Success, node.Tick(context.Background()))
}

func TestDeserializeInverter(t *testing.T) {
	reg := NewFactoryRegistry()
	RegisterDefaultNodes(reg)
	reg.RegisterCondition("no", func(ctx context.Context) bool { return false })

	node, _, err := DeserializeTree([]byte(`{
		"tree": {
			"type": "Inverter",
			"children": [
				{"type": "Condition", "properties": {"fn": "no"}}
			]
		}
	}`), reg)
	require.NoError(t, err)
	require.Equal(t, Success, node.Tick(context.Background()))
}

func TestDeserializeReactiveSequence(t *testing.T) {
	reg := NewFactoryRegistry()
	RegisterDefaultNodes(reg)
	reg.RegisterCondition("yes", func(ctx context.Context) bool { return true })

	node, _, err := DeserializeTree([]byte(`{
		"tree": {
			"type": "ReactiveSequence",
			"children": [
				{"type": "Condition", "properties": {"fn": "yes"}},
				{"type": "Condition", "properties": {"fn": "yes"}}
			]
		}
	}`), reg)
	require.NoError(t, err)
	require.Equal(t, Success, node.Tick(context.Background()))
}

func TestDeserializeReactiveFallback(t *testing.T) {
	reg := NewFactoryRegistry()
	RegisterDefaultNodes(reg)
	reg.RegisterCondition("yes", func(ctx context.Context) bool { return true })
	reg.RegisterCondition("no", func(ctx context.Context) bool { return false })

	node, _, err := DeserializeTree([]byte(`{
		"tree": {
			"type": "ReactiveFallback",
			"children": [
				{"type": "Condition", "properties": {"fn": "no"}},
				{"type": "Condition", "properties": {"fn": "yes"}}
			]
		}
	}`), reg)
	require.NoError(t, err)
	require.Equal(t, Success, node.Tick(context.Background()))
}

func TestDeserializeRetry(t *testing.T) {
	reg := NewFactoryRegistry()
	RegisterDefaultNodes(reg)
	reg.RegisterCondition("no", func(ctx context.Context) bool { return false })

	node, _, err := DeserializeTree([]byte(`{
		"tree": {
			"type": "Retry",
			"properties": {"max_retry": 2},
			"children": [
				{"type": "Condition", "properties": {"fn": "no"}}
			]
		}
	}`), reg)
	require.NoError(t, err)
	require.Equal(t, Failure, node.Tick(context.Background()))
}

func TestDeserializeRetryDefaultMax(t *testing.T) {
	reg := NewFactoryRegistry()
	RegisterDefaultNodes(reg)
	reg.RegisterCondition("no", func(ctx context.Context) bool { return false })

	node, _, err := DeserializeTree([]byte(`{
		"tree": {
			"type": "Retry",
			"children": [
				{"type": "Condition", "properties": {"fn": "no"}}
			]
		}
	}`), reg)
	require.NoError(t, err)
	// default max_retry=3, condition returns false 3 times → Failure
	require.Equal(t, Failure, node.Tick(context.Background()))
}

func TestDeserializeWithBlackboardData(t *testing.T) {
	reg := NewFactoryRegistry()
	RegisterDefaultNodes(reg)

	reg.RegisterCondition("has_ns", func(ctx context.Context) bool {
		bb := BlackboardFromContext(ctx)
		return bb != nil && bb.GetString("nsID") != ""
	})

	node, bbData, err := DeserializeTree([]byte(`{
		"name": "test-tree",
		"blackboard": {"nsID": "ns-123"},
		"tree": {
			"type": "Condition",
			"properties": {"fn": "has_ns"}
		}
	}`), reg)
	require.NoError(t, err)
	require.NotNil(t, bbData)
	require.Equal(t, "ns-123", bbData["nsID"])

	bb := NewBlackboard()
	for k, v := range bbData {
		bb.Set(k, v)
	}
	ctx := ContextWithBlackboard(context.Background(), bb)
	require.Equal(t, Success, node.Tick(ctx))
}

func TestDeserializeAction(t *testing.T) {
	reg := NewFactoryRegistry()
	RegisterDefaultNodes(reg)

	callCount := 0
	reg.RegisterAction("increment", func(ctx context.Context) (bool, error) {
		callCount++
		return true, nil
	})

	node, _, err := DeserializeTree([]byte(`{
		"tree": {
			"type": "Action",
			"properties": {"fn": "increment"}
		}
	}`), reg)
	require.NoError(t, err)
	require.Equal(t, Success, node.Tick(context.Background()))
	require.Equal(t, 1, callCount)
}

func TestDeserializeActionReadsFromBlackboard(t *testing.T) {
	reg := NewFactoryRegistry()
	RegisterDefaultNodes(reg)

	var captured string
	reg.RegisterAction("read_bb", func(ctx context.Context) (bool, error) {
		bb := BlackboardFromContext(ctx)
		if bb != nil {
			captured = bb.GetString("val")
		}
		return true, nil
	})

	node, _, err := DeserializeTree([]byte(`{
		"tree": {
			"type": "Action",
			"properties": {"fn": "read_bb"}
		}
	}`), reg)
	require.NoError(t, err)

	bb := NewBlackboard()
	bb.Set("val", "from-bb")
	ctx := ContextWithBlackboard(context.Background(), bb)
	require.Equal(t, Success, node.Tick(ctx))
	require.Equal(t, "from-bb", captured)
}

func TestDeserializeUnknownConditionFn(t *testing.T) {
	reg := NewFactoryRegistry()
	RegisterDefaultNodes(reg)

	_, _, err := DeserializeTree([]byte(`{
		"tree": {
			"type": "Condition",
			"properties": {"fn": "does_not_exist"}
		}
	}`), reg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown condition function")
}

func TestDeserializeInvalidJSON(t *testing.T) {
	reg := NewFactoryRegistry()
	_, _, err := DeserializeTree([]byte("{invalid"), reg)
	require.Error(t, err)
}

func TestDeserializeNodeDirect(t *testing.T) {
	reg := NewFactoryRegistry()
	RegisterDefaultNodes(reg)
	reg.RegisterCondition("ok", func(ctx context.Context) bool { return true })

	node, err := DeserializeNode([]byte(`{"type": "Condition", "properties": {"fn": "ok"}}`), reg)
	require.NoError(t, err)
	require.Equal(t, Success, node.Tick(context.Background()))
}

func TestDeserializeNestedRetryInSequence(t *testing.T) {
	reg := NewFactoryRegistry()
	RegisterDefaultNodes(reg)

	failCount := 0
	reg.RegisterCondition("fail_twice", func(ctx context.Context) bool {
		failCount++
		return failCount >= 3
	})

	node, _, err := DeserializeTree([]byte(`{
		"tree": {
			"type": "Sequence",
			"children": [
				{
					"type": "Retry",
					"properties": {"max_retry": 5},
					"children": [
						{"type": "Condition", "properties": {"fn": "fail_twice"}}
					]
				}
			]
		}
	}`), reg)
	require.NoError(t, err)
	require.Equal(t, Success, node.Tick(context.Background()))
	// failCount starts at 0, fails on tick 0 and 1, succeeds on tick 2
	require.Equal(t, 3, failCount)
}

func TestDeserializeTreeFileWithoutBlackboard(t *testing.T) {
	reg := NewFactoryRegistry()
	RegisterDefaultNodes(reg)
	reg.RegisterCondition("yes", func(ctx context.Context) bool { return true })

	node, bbData, err := DeserializeTree([]byte(`{
		"name": "simple",
		"tree": {"type": "Condition", "properties": {"fn": "yes"}}
	}`), reg)
	require.NoError(t, err)
	require.Nil(t, bbData)
	require.Equal(t, Success, node.Tick(context.Background()))
}
