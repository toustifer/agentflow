package server

import (
	"context"

	"github.com/toustifer/agentflow/pkg/bt"
)

// RegisterBuiltinNodes registers all leader/worker/reviewer condition and action
// functions into the factory registry. Action functions read the Server and nsID
// from the blackboard (set by handleLeaderTick before each tick).
func RegisterBuiltinNodes(reg *bt.FactoryRegistry) {
	// ===== Phase condition checks =====
	reg.RegisterCondition("phase_is_setup", func(ctx context.Context) bool {
		bb := bt.BlackboardFromContext(ctx)
		return bb != nil && bb.GetString("phase") == "setup"
	})
	reg.RegisterCondition("phase_is_shape", func(ctx context.Context) bool {
		bb := bt.BlackboardFromContext(ctx)
		return bb != nil && bb.GetString("phase") == "shape"
	})
	reg.RegisterCondition("phase_is_plan", func(ctx context.Context) bool {
		bb := bt.BlackboardFromContext(ctx)
		return bb != nil && bb.GetString("phase") == "plan"
	})
	reg.RegisterCondition("phase_is_execute", func(ctx context.Context) bool {
		bb := bt.BlackboardFromContext(ctx)
		return bb != nil && bb.GetString("phase") == "execute"
	})
	reg.RegisterCondition("phase_is_stuck", func(ctx context.Context) bool {
		bb := bt.BlackboardFromContext(ctx)
		return bb != nil && bb.GetString("phase") == "stuck"
	})
	reg.RegisterCondition("phase_is_done", func(ctx context.Context) bool {
		bb := bt.BlackboardFromContext(ctx)
		return bb != nil && bb.GetString("phase") == "done"
	})

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

	// Phase action placeholders
	reg.RegisterAction("setup_actions", placeholderAction("setup_actions"))
	reg.RegisterAction("shape_actions", placeholderAction("shape_actions"))
	reg.RegisterAction("plan_actions", placeholderAction("plan_actions"))
	reg.RegisterAction("dispatch_task", placeholderAction("dispatch_task"))
	reg.RegisterAction("monitor_tasks", placeholderAction("monitor_tasks"))
	reg.RegisterAction("report_stuck", placeholderAction("report_stuck"))
	reg.RegisterAction("report_done", placeholderAction("report_done"))

	// ===== Worker placeholder functions =====
	// Allow worker-default.json to deserialize without error.
	reg.RegisterAction("doc_search_prepare", placeholderAction("doc_search_prepare"))
	reg.RegisterAction("task_get_confirm", placeholderAction("task_get_confirm"))
	reg.RegisterAction("enter_worktree", placeholderAction("enter_worktree"))
	reg.RegisterAction("implement_code", placeholderAction("implement_code"))
	reg.RegisterAction("git_commit_changes", placeholderAction("git_commit_changes"))
	reg.RegisterAction("doc_write_record", placeholderAction("doc_write_record"))
	reg.RegisterAction("diary_write_entry", placeholderAction("diary_write_entry"))
	reg.RegisterAction("task_submit_for_review", placeholderAction("task_submit_for_review"))

	// ===== Reviewer placeholder functions =====
	reg.RegisterCondition("review_approved", func(ctx context.Context) bool {
		return false
	})
	reg.RegisterAction("fetch_work_diff", placeholderAction("fetch_work_diff"))
	reg.RegisterAction("task_review_pass", placeholderAction("task_review_pass"))
	reg.RegisterAction("task_review_rework", placeholderAction("task_review_rework"))
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

// placeholderAction returns a no-op action for worker/reviewer templates.
func placeholderAction(name string) func(ctx context.Context) (bool, error) {
	return func(ctx context.Context) (bool, error) {
		return true, nil
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
                  },
                  {
                    "type": "Sequence",
                    "children": [
                      { "type": "Condition", "properties": { "fn": "has_stuck_tasks" } },
                      { "type": "Action", "properties": { "fn": "report_stuck" } }
                    ]
                  }
                ]
              }
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
