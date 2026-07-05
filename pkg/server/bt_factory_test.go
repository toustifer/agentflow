package server

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/toustifer/agentflow/pkg/bt"
)

func TestRegisterBuiltinNodesReviewDecisionPresentReadsBlackboardPresence(t *testing.T) {
	reg := bt.NewFactoryRegistry()
	bt.RegisterDefaultNodes(reg)
	RegisterBuiltinNodes(reg)

	node, err := reg.Build(bt.NodeConfig{
		Type:       "Condition",
		Properties: map[string]any{"fn": "review_decision_present"},
	})
	require.NoError(t, err)

	bb := bt.NewBlackboard()
	ctx := bt.ContextWithBlackboard(context.Background(), bb)
	require.Equal(t, bt.Failure, node.Tick(ctx))

	bb.Set("review_approved", false)
	require.Equal(t, bt.Success, node.Tick(ctx))
}

func TestRegisterBuiltinNodesValidatesWorkerAndReviewerTrees(t *testing.T) {
	reg := bt.NewFactoryRegistry()
	bt.RegisterDefaultNodes(reg)
	RegisterBuiltinNodes(reg)

	require.NoError(t, bt.ValidateTreeJSON([]byte(`{
  "name": "worker-default",
  "blackboard": {},
  "tree": {
    "type": "Sequence",
    "children": [
      {"type": "Action", "properties": {"fn": "task_get_confirm"}},
      {"type": "Action", "properties": {"fn": "enter_worktree"}},
      {"type": "Action", "properties": {"fn": "implement_code"}},
      {"type": "Action", "properties": {"fn": "git_commit_changes"}},
      {"type": "Action", "properties": {"fn": "doc_write_record"}},
      {"type": "Action", "properties": {"fn": "diary_write_entry"}},
      {"type": "Action", "properties": {"fn": "task_submit_for_review"}}
    ]
  }
}`), reg))

	require.NoError(t, bt.ValidateTreeJSON([]byte(`{
  "name": "reviewer-default",
  "blackboard": {},
  "tree": {
    "type": "Sequence",
    "children": [
      {"type": "Action", "properties": {"fn": "fetch_work_diff"}},
      {"type": "Action", "properties": {"fn": "review_decide"}},
      {
        "type": "Sequence",
        "children": [
          {"type": "Condition", "properties": {"fn": "review_decision_present"}},
          {
            "type": "Fallback",
            "children": [
              {
                "type": "Sequence",
                "children": [
                  {"type": "Condition", "properties": {"fn": "review_approved"}},
                  {"type": "Action", "properties": {"fn": "task_review_pass"}}
                ]
              },
              {
                "type": "Sequence",
                "children": [
                  {"type": "Inverter", "children": [{"type": "Condition", "properties": {"fn": "review_approved"}}]},
                  {"type": "Action", "properties": {"fn": "task_review_rework"}}
                ]
              }
            ]
          }
        ]
      }
    ]
  }
}`), reg))
}

func TestPlaceholderActionFailsExplicitly(t *testing.T) {
	reg := bt.NewFactoryRegistry()
	bt.RegisterDefaultNodes(reg)
	RegisterBuiltinNodes(reg)

	node, err := reg.Build(bt.NodeConfig{
		Type:       "Action",
		Properties: map[string]any{"fn": "task_get_confirm"},
	})
	require.NoError(t, err)
	require.Equal(t, bt.Failure, node.Tick(context.Background()))
}

func TestRegisterBuiltinNodesReviewApprovedReadsBlackboard(t *testing.T) {
	reg := bt.NewFactoryRegistry()
	bt.RegisterDefaultNodes(reg)
	RegisterBuiltinNodes(reg)

	node, err := reg.Build(bt.NodeConfig{
		Type:       "Condition",
		Properties: map[string]any{"fn": "review_approved"},
	})
	require.NoError(t, err)

	bb := bt.NewBlackboard()
	ctx := bt.ContextWithBlackboard(context.Background(), bb)
	require.Equal(t, bt.Failure, node.Tick(ctx))

	bb.Set("review_approved", true)
	require.Equal(t, bt.Success, node.Tick(ctx))
}
