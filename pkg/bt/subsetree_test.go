package bt

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSubTreeBasic(t *testing.T) {
	treeReg := NewRegistry()
	condReg := NewFactoryRegistry()
	RegisterDefaultNodes(condReg)
	condReg.RegisterCondition("yes", func(ctx context.Context) bool { return true })

	// Register a simple tree in the tree registry
	innerNode, err := condReg.Build(NodeConfig{
		Type:       "Condition",
		Properties: map[string]any{"fn": "yes"},
	})
	require.NoError(t, err)
	treeReg.Register("inner", innerNode)

	// Create SubTree
	st := NewSubTree("inner", treeReg)
	require.Equal(t, Success, st.Tick(context.Background()))
}

func TestSubTreeWithBlackboard(t *testing.T) {
	treeReg := NewRegistry()
	condReg := NewFactoryRegistry()
	RegisterDefaultNodes(condReg)

	condReg.RegisterCondition("has_val", func(ctx context.Context) bool {
		bb := BlackboardFromContext(ctx)
		return bb != nil && bb.GetString("x") != ""
	})

	innerNode, err := condReg.Build(NodeConfig{
		Type:       "Condition",
		Properties: map[string]any{"fn": "has_val"},
	})
	require.NoError(t, err)
	treeReg.Register("inner", innerNode)

	st := NewSubTree("inner", treeReg)

	// Tick without blackboard — should fail
	require.Equal(t, Failure, st.Tick(context.Background()))

	// Tick with blackboard — should succeed
	bb := NewBlackboard()
	bb.Set("x", "hello")
	ctx := ContextWithBlackboard(context.Background(), bb)
	require.Equal(t, Success, st.Tick(ctx))
}

func TestSubTreeParentChain(t *testing.T) {
	treeReg := NewRegistry()
	condReg := NewFactoryRegistry()
	RegisterDefaultNodes(condReg)

	// Subtree condition reads from parent chain
	condReg.RegisterCondition("from_parent", func(ctx context.Context) bool {
		bb := BlackboardFromContext(ctx)
		return bb != nil && bb.GetString("parent_val") != ""
	})

	innerNode, err := condReg.Build(NodeConfig{
		Type:       "Condition",
		Properties: map[string]any{"fn": "from_parent"},
	})
	require.NoError(t, err)
	treeReg.Register("inner", innerNode)

	// Wrap in a Sequence to test multiple subtree interactions
	reg := NewFactoryRegistry()
	RegisterDefaultNodes(reg)
	reg.SetTreeRegistry(treeReg)

	node, err := reg.Build(NodeConfig{
		Type: "Sequence",
		Children: rawMessages(
			`{"type": "SubTree", "properties": {"tree": "inner"}}`,
		),
	})
	require.NoError(t, err)

	bb := NewBlackboard()
	bb.Set("parent_val", "exists")
	ctx := ContextWithBlackboard(context.Background(), bb)
	require.Equal(t, Success, node.Tick(ctx))
}

func TestSubTreeJSON(t *testing.T) {
	treeReg := NewRegistry()
	condReg := NewFactoryRegistry()
	RegisterDefaultNodes(condReg)
	condReg.RegisterCondition("ok", func(ctx context.Context) bool { return true })

	innerNode, err := condReg.Build(NodeConfig{
		Type:       "Condition",
		Properties: map[string]any{"fn": "ok"},
	})
	require.NoError(t, err)
	treeReg.Register("inner-cond", innerNode)

	reg := NewFactoryRegistry()
	RegisterDefaultNodes(reg)
	reg.SetTreeRegistry(treeReg)

	node, _, err := DeserializeTree([]byte(`{
		"tree": {
			"type": "SubTree",
			"properties": {"tree": "inner-cond"}
		}
	}`), reg)
	require.NoError(t, err)

	require.Equal(t, Success, node.Tick(context.Background()))
}

func TestSubTreeRequiresTreeProperty(t *testing.T) {
	reg := NewFactoryRegistry()
	RegisterDefaultNodes(reg)

	_, err := reg.Build(NodeConfig{Type: "SubTree", Properties: map[string]any{}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "requires 'tree' property")
}

func TestSubTreeRequiresTreeRegistry(t *testing.T) {
	reg := NewFactoryRegistry()
	RegisterDefaultNodes(reg)
	// No treeRegistry set

	_, err := reg.Build(NodeConfig{
		Type:       "SubTree",
		Properties: map[string]any{"tree": "anything"},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "no tree registry set")
}

func TestSubTreeNotFound(t *testing.T) {
	treeReg := NewRegistry()
	reg := NewFactoryRegistry()
	RegisterDefaultNodes(reg)
	reg.SetTreeRegistry(treeReg)

	node, err := reg.Build(NodeConfig{
		Type:       "SubTree",
		Properties: map[string]any{"tree": "nonexistent"},
	})
	require.NoError(t, err)

	status := node.Tick(context.Background())
	require.Equal(t, Failure, status)
}

func TestSubTreeRunningState(t *testing.T) {
	treeReg := NewRegistry()
	waitNode := NewWait(3)
	treeReg.Register("waiter", waitNode)

	st := NewSubTree("waiter", treeReg)
	ctx := context.Background()

	require.Equal(t, Running, st.Tick(ctx))
	require.Equal(t, Running, st.Tick(ctx))
	require.Equal(t, Success, st.Tick(ctx))

	// After completion, next tick starts fresh
	require.Equal(t, Running, st.Tick(ctx))
}

func TestSubTreeHalt(t *testing.T) {
	treeReg := NewRegistry()
	waitNode := NewWait(10)
	treeReg.Register("waiter", waitNode)

	st := NewSubTree("waiter", treeReg)
	ctx := context.Background()

	require.Equal(t, Running, st.Tick(ctx))
	st.Halt()

	// After halt, should restart from beginning
	require.Equal(t, Running, st.Tick(ctx))
}

func TestLogDecoratorPassthrough(t *testing.T) {
	cond := NewCondition(func(ctx context.Context) bool { return true })
	log := NewLogDecorator("test", cond)
	require.Equal(t, Success, log.Tick(context.Background()))

	cond2 := NewCondition(func(ctx context.Context) bool { return false })
	log2 := NewLogDecorator("test-fail", cond2)
	require.Equal(t, Failure, log2.Tick(context.Background()))
}

func TestLogJSON(t *testing.T) {
	reg := NewFactoryRegistry()
	RegisterDefaultNodes(reg)
	reg.RegisterCondition("yes", func(ctx context.Context) bool { return true })

	node, _, err := DeserializeTree([]byte(`{
		"tree": {
			"type": "Log",
			"properties": {"message": "hello"},
			"children": [
				{"type": "Condition", "properties": {"fn": "yes"}}
			]
		}
	}`), reg)
	require.NoError(t, err)
	require.Equal(t, Success, node.Tick(context.Background()))
}

func TestLogRequiresOneChild(t *testing.T) {
	reg := NewFactoryRegistry()
	RegisterDefaultNodes(reg)

	_, err := reg.Build(NodeConfig{Type: "Log"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "requires exactly 1 child")
}
