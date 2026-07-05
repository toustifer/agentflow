package server

import (
	"context"
	"fmt"

	"github.com/toustifer/agentflow/pkg/bt"
)

// RegisterBuiltinNodes registers all leader/worker/reviewer condition and action
// functions into the factory registry. Action functions read the Server and nsID
// from the blackboard (set by handleLeaderTick before each tick).
func RegisterBuiltinNodes(reg *bt.FactoryRegistry) {
	// ===== Phase condition checks =====
	for _, phase := range []string{"setup", "shape", "plan", "execute", "stuck", "done"} {
		phaseName := phase
		reg.RegisterCondition("phase_is_"+phaseName, func(ctx context.Context) bool {
			bb := bt.BlackboardFromContext(ctx)
			return bb != nil && bb.GetString("phase") == phaseName
		})
	}

	// ===== Task state conditions =====
	reg.RegisterCondition("has_next_tasks", func(ctx context.Context) bool {
		bb := bt.BlackboardFromContext(ctx)
		return bb != nil && bb.GetBool("has_next_tasks")
	})
	reg.RegisterCondition("has_active_tasks", func(ctx context.Context) bool {
		bb := bt.BlackboardFromContext(ctx)
		return bb != nil && bb.GetBool("has_active_tasks")
	})
	reg.RegisterCondition("has_stuck_tasks", func(ctx context.Context) bool {
		bb := bt.BlackboardFromContext(ctx)
		return bb != nil && bb.GetBool("has_stuck_tasks")
	})
	reg.RegisterCondition("review_decision_present", func(ctx context.Context) bool {
		bb := bt.BlackboardFromContext(ctx)
		return bb != nil && bb.Has("review_approved")
	})

	// ===== Actions =====

	// refresh_phase calls project_next_steps and stores phase info in blackboard
	reg.RegisterAction("refresh_phase", func(ctx context.Context) (bool, error) {
		bb := bt.BlackboardFromContext(ctx)
		if bb == nil {
			return false, nil
		}
		sVal, _ := bb.Get("server")
		s, _ := sVal.(*Server)
		if s == nil {
			return false, nil
		}
		nsID := bb.GetString("nsID")
		if nsID == "" {
			return false, nil
		}

		result, err := s.handleProjectNextSteps(ctx, map[string]any{"namespace_id": nsID})
		if err != nil {
			return false, err
		}

		bb.Set("phase", toString(result["phase"]))
		bb.Set("phase_name", toString(result["phase_name"]))
		bb.Set("progress", toString(result["progress"]))

		nextTasks := toMapSlice(result["next_tasks"])
		activeTasks := toMapSlice(result["active_tasks"])
		stuckTasks := toMapSlice(result["stuck_tasks"])

		bb.Set("has_next_tasks", len(nextTasks) > 0)
		bb.Set("has_active_tasks", len(activeTasks) > 0)
		bb.Set("has_stuck_tasks", len(stuckTasks) > 0)
		bb.Set("next_tasks", nextTasks)
		bb.Set("active_tasks", activeTasks)
		bb.Set("stuck_tasks", stuckTasks)
		bb.Set("actions", toStringSlice(result["actions"]))

		return true, nil
	})

	for _, name := range []string{
		"setup_actions",
		"shape_actions",
		"plan_actions",
		"dispatch_task",
		"monitor_tasks",
		"report_stuck",
		"report_done",
		"doc_search_prepare",
		"task_get_confirm",
		"enter_worktree",
		"implement_code",
		"git_commit_changes",
		"doc_write_record",
		"diary_write_entry",
		"task_submit_for_review",
		"fetch_work_diff",
		"review_decide",
		"task_review_pass",
		"task_review_rework",
	} {
		reg.RegisterAction(name, placeholderAction(name))
	}

	// ===== Reviewer placeholder functions =====
	reg.RegisterCondition("review_approved", func(ctx context.Context) bool {
		bb := bt.BlackboardFromContext(ctx)
		return bb != nil && bb.GetBool("review_approved")
	})
}

// toMapSlice converts a []any of maps to []map[string]any.
func toMapSlice(v any) []map[string]any {
	if list, ok := v.([]map[string]any); ok {
		return list
	}
	if list, ok := v.([]any); ok {
		out := make([]map[string]any, 0, len(list))
		for _, item := range list {
			if m, ok := item.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	}
	return nil
}

// placeholderAction returns a stub action for trees that are only executable via
// the Python BT sidecar.
func placeholderAction(name string) func(ctx context.Context) (bool, error) {
	return func(ctx context.Context) (bool, error) {
		return false, fmt.Errorf("BT action %q requires the Python sidecar runtime", name)
	}
}

// leaderDefaultJSON is the embedded leader behavior tree definition.
const leaderDefaultJSON = `{
  "name": "leader-default",
  "blackboard": {},
  "tree": {
    "type": "ReactiveSequence",
    "children": [
      {
        "type": "Action",
        "name": "refresh_phase",
        "properties": { "fn": "refresh_phase" }
      },
      {
        "type": "Fallback",
        "children": [
          {
            "type": "Sequence",
            "children": [
              { "type": "Condition", "properties": { "fn": "phase_is_setup" } },
              { "type": "Action", "properties": { "fn": "setup_actions" } }
            ]
          },
          {
            "type": "Sequence",
            "children": [
              { "type": "Condition", "properties": { "fn": "phase_is_shape" } },
              { "type": "Action", "properties": { "fn": "shape_actions" } }
            ]
          },
          {
            "type": "Sequence",
            "children": [
              { "type": "Condition", "properties": { "fn": "phase_is_plan" } },
              { "type": "Action", "properties": { "fn": "plan_actions" } }
            ]
          },
          {
            "type": "Sequence",
            "children": [
              { "type": "Condition", "properties": { "fn": "phase_is_execute" } },
              {
                "type": "Fallback",
                "children": [
                  {
                    "type": "Sequence",
                    "children": [
                      { "type": "Condition", "properties": { "fn": "has_next_tasks" } },
                      { "type": "Action", "properties": { "fn": "dispatch_task" } }
                    ]
                  },
                  {
                    "type": "Sequence",
                    "children": [
                      { "type": "Condition", "properties": { "fn": "has_active_tasks" } },
                      { "type": "Action", "properties": { "fn": "monitor_tasks" } }
                    ]
                  }
                ]
              }
            ]
          },
          {
            "type": "Sequence",
            "children": [
              { "type": "Condition", "properties": { "fn": "phase_is_stuck" } },
              { "type": "Action", "properties": { "fn": "report_stuck" } }
            ]
          },
          {
            "type": "Sequence",
            "children": [
              { "type": "Condition", "properties": { "fn": "phase_is_done" } },
              { "type": "Action", "properties": { "fn": "report_done" } }
            ]
          }
        ]
      }
    ]
  }
}`
