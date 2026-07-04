"""Built-in condition and action functions — mirrors Go bt_factory.go.

This iteration makes `refresh_phase` real: it first tries a narrow Go phase
provider, then falls back to prefilled `phase_data` if present.
"""

from bt_service.core.blackboard import Blackboard
from bt_service.factory.registry import FactoryRegistry
from bt_service.server.phase_client import fetch_phase, PhaseProviderError
from bt_service.server.dispatch_client import dispatch_task, DispatchProviderError


def register_builtin_nodes(reg: FactoryRegistry) -> None:
    """Register all built-in condition and action functions."""

    reg.register_condition("phase_is_setup", lambda bb: bb.get_string("phase") == "setup")
    reg.register_condition("phase_is_shape", lambda bb: bb.get_string("phase") == "shape")
    reg.register_condition("phase_is_plan", lambda bb: bb.get_string("phase") == "plan")
    reg.register_condition("phase_is_execute", lambda bb: bb.get_string("phase") == "execute")
    reg.register_condition("phase_is_stuck", lambda bb: bb.get_string("phase") == "stuck")
    reg.register_condition("phase_is_done", lambda bb: bb.get_string("phase") == "done")

    reg.register_condition("has_next_tasks", lambda bb: bb.get_bool("has_next_tasks"))
    reg.register_condition("has_active_tasks", lambda bb: bb.get_bool("has_active_tasks"))
    reg.register_condition("has_stuck_tasks", lambda bb: bb.get_bool("has_stuck_tasks"))
    reg.register_condition("review_approved", lambda bb: bb.get_bool("review_approved"))

    reg.register_action("refresh_phase", refresh_phase)
    reg.register_action("dispatch_task", dispatch_task_action)

    for name in [
        "setup_actions", "shape_actions", "plan_actions",
        "monitor_tasks", "report_stuck", "report_done",
    ]:
        reg.register_action(name, _noop_action)

    for name in [
        "doc_search_prepare", "task_get_confirm", "enter_worktree",
        "implement_code", "git_commit_changes", "doc_write_record",
        "diary_write_entry", "task_submit_for_review",
    ]:
        reg.register_action(name, _noop_action)

    for name in [
        "fetch_work_diff", "task_review_pass", "task_review_rework",
    ]:
        reg.register_action(name, _noop_action)


def refresh_phase(bb: Blackboard) -> tuple:
    """Fetch or consume phase payload, then normalize leader keys.

    Preferred path: call the narrow Go phase provider using namespace_id.
    Fallback path: consume prefilled `phase_data` from blackboard.
    """
    phase_data = bb.get("phase_data", {}) or {}

    # Preferred path: fetch from Go via narrow provider
    namespace_id = bb.get_string("namespace_id") or bb.get_string("nsID")
    workdir = bb.get_string("workdir")
    if namespace_id:
        try:
            phase_data = fetch_phase(namespace_id, workdir)
        except PhaseProviderError:
            # Fall back to phase_data if already pre-populated
            pass

    if not isinstance(phase_data, dict):
        return (False, ValueError("phase_data must be an object"))

    phase = phase_data.get("phase", "") or ""
    phase_name = phase_data.get("phase_name", "") or ""
    progress = phase_data.get("progress", "") or ""
    actions = phase_data.get("actions", []) or []
    next_tasks = phase_data.get("next_tasks", []) or []
    active_tasks = phase_data.get("active_tasks", []) or []
    stuck_tasks = phase_data.get("stuck_tasks", []) or []

    bb.set("phase", phase)
    bb.set("phase_name", phase_name)
    bb.set("progress", progress)
    bb.set("actions", actions)
    bb.set("next_tasks", next_tasks)
    bb.set("active_tasks", active_tasks)
    bb.set("stuck_tasks", stuck_tasks)
    bb.set("has_next_tasks", len(next_tasks) > 0)
    bb.set("has_active_tasks", len(active_tasks) > 0)
    bb.set("has_stuck_tasks", len(stuck_tasks) > 0)
    return (True, None)


def dispatch_task_action(bb: Blackboard) -> tuple:
    namespace_id = bb.get_string("namespace_id") or bb.get_string("nsID")
    if not namespace_id:
        return (False, ValueError("namespace_id is required"))

    if bb.get_string("phase") != "execute":
        return (False, ValueError("dispatch_task requires execute phase"))
    if not bb.get_bool("has_next_tasks"):
        return (False, ValueError("dispatch_task requires has_next_tasks=true"))

    next_tasks = bb.get("next_tasks", [])
    if not isinstance(next_tasks, list) or not next_tasks:
        return (False, ValueError("dispatch_task requires a non-empty next_tasks list"))
    task = next_tasks[0]
    if not isinstance(task, dict):
        return (False, ValueError("next_tasks[0] must be an object"))

    task_id = task.get("task_id") or task.get("id") or ""
    if not isinstance(task_id, str) or not task_id:
        return (False, ValueError("dispatch_task requires next_tasks[0].task_id"))

    try:
        result = dispatch_task(namespace_id, task_id)
    except DispatchProviderError as err:
        return (False, err)

    bb.set("last_dispatch_task_id", result.get("task_id", "") or "")
    bb.set("last_dispatch_state", result.get("state", "") or "")
    bb.set("last_dispatch_worker", result.get("assigned_worker", "") or "")
    bb.set("last_dispatch_worktree_path", result.get("worktree_path", "") or "")
    bb.set("last_dispatch_branch", result.get("branch", "") or "")
    return (True, None)


def _noop_action(bb: Blackboard) -> tuple:
    return (True, None)
