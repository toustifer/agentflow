"""Built-in condition and action functions — mirrors Go bt_factory.go.

This iteration makes `refresh_phase` real: it first tries a narrow Go phase
provider, then falls back to prefilled `phase_data` if present.
"""

from bt_service.core.blackboard import Blackboard
from bt_service.factory.registry import FactoryRegistry
from bt_service.server.phase_client import fetch_phase, PhaseProviderError
from bt_service.server.dispatch_client import dispatch_task, DispatchProviderError
from bt_service.server.monitor_client import monitor_task, MonitorProviderError
from bt_service.server.stuck_client import report_stuck, ReportStuckProviderError
from bt_service.server.done_client import report_done, ReportDoneProviderError
from bt_service.server.task_get_client import task_get_confirm, TaskGetConfirmProviderError
from bt_service.server.enter_worktree_client import enter_worktree, EnterWorktreeProviderError
from bt_service.server.implement_client import implement_code, ImplementCodeProviderError
from bt_service.server.git_commit_client import git_commit_changes, GitCommitChangesProviderError
from bt_service.server.doc_write_client import doc_write_record, DocWriteRecordProviderError
from bt_service.server.diary_write_client import diary_write_entry, DiaryWriteEntryProviderError
from bt_service.server.submit_review_client import task_submit_for_review, TaskSubmitForReviewProviderError
from bt_service.server.fetch_diff_client import fetch_work_diff, FetchWorkDiffProviderError
from bt_service.server.review_decision_client import task_review_pass, task_review_rework, TaskReviewPassProviderError, TaskReviewReworkProviderError


def _require_namespace_id(bb: Blackboard) -> str:
    namespace_id = bb.get_string("namespace_id") or bb.get_string("nsID")
    if not namespace_id:
        raise ValueError("namespace_id is required")
    return namespace_id


def _require_task_id(bb: Blackboard, *keys: str) -> str:
    for key in keys:
        task_id = bb.get_string(key)
        if task_id:
            return task_id

    task = bb.get("task", {}) or {}
    if isinstance(task, dict):
        value = task.get("task_id") or task.get("id") or ""
        if isinstance(value, str) and value:
            return value

    raise ValueError("task_id is required")


def _require_worker_id(bb: Blackboard) -> str:
    worker_id = bb.get_string("worker_id")
    if not worker_id:
        raise ValueError("worker_id is required")
    return worker_id


def _require_first_task_id(bb: Blackboard, list_key: str) -> str:
    tasks = bb.get(list_key, [])
    if not isinstance(tasks, list) or not tasks:
        raise ValueError(f"{list_key} requires a non-empty list")

    task = tasks[0]
    if not isinstance(task, dict):
        raise ValueError(f"{list_key}[0] must be an object")

    task_id = task.get("task_id") or task.get("id") or ""
    if not isinstance(task_id, str) or not task_id:
        raise ValueError(f"{list_key} requires {list_key}[0].task_id")
    return task_id


def _require_git_payload(action_name: str, git: object) -> tuple[str, str, str, str]:
    if not isinstance(git, dict):
        raise ValueError(f"{action_name} requires git object")

    git_status = git.get("status", "") or ""
    if git_status == "branch_mismatch":
        raise ValueError(f"{action_name} rejected branch_mismatch status")

    branch = git.get("branch", "") or ""
    repo_path = git.get("repo_path", "") or ""
    worktree_path = git.get("worktree_path", "") or ""
    if not all(isinstance(v, str) and v for v in [branch, repo_path, worktree_path]):
        raise ValueError(f"{action_name} requires branch, repo_path, and worktree_path")

    return git_status, branch, repo_path, worktree_path


def _apply_review_decision(
    bb: Blackboard,
    provider,
    provider_error,
    *,
    expected_state: str,
    decision: str,
    action_name: str,
) -> tuple:
    try:
        namespace_id = _require_namespace_id(bb)
        task_id = _require_task_id(bb, "task_id", "review_fetch_task_id")
        worker_id = _require_worker_id(bb)
        result = provider(namespace_id, task_id, worker_id)
    except ValueError as err:
        return (False, err)
    except provider_error as err:
        return (False, err)

    state = result.get("state", "") or ""
    if state != expected_state:
        return (False, ValueError(f"{action_name} expected {expected_state}, got {state!r}"))

    bb.set("review_decision", decision)
    bb.set("review_decision_task_id", result.get("task_id", "") or "")
    bb.set("review_decision_state", state)
    return (True, None)


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
    reg.register_condition("review_decision_present", lambda bb: bb.has("review_approved"))
    reg.register_condition("review_approved", lambda bb: bb.get_bool("review_approved"))

    reg.register_action("refresh_phase", refresh_phase)
    reg.register_action("dispatch_task", dispatch_task_action)
    reg.register_action("monitor_tasks", monitor_tasks_action)
    reg.register_action("report_stuck", report_stuck_action)
    reg.register_action("report_done", report_done_action)
    reg.register_action("task_get_confirm", task_get_confirm_action)
    reg.register_action("enter_worktree", enter_worktree_action)
    reg.register_action("implement_code", implement_code_action)
    reg.register_action("git_commit_changes", git_commit_changes_action)
    reg.register_action("doc_write_record", doc_write_record_action)
    reg.register_action("diary_write_entry", diary_write_entry_action)
    reg.register_action("task_submit_for_review", task_submit_for_review_action)
    reg.register_action("fetch_work_diff", fetch_work_diff_action)
    reg.register_action("review_decide", review_decide_action)
    reg.register_action("task_review_pass", task_review_pass_action)
    reg.register_action("task_review_rework", task_review_rework_action)

    for name in [
        "setup_actions", "shape_actions", "plan_actions",
    ]:
        reg.register_action(name, _noop_action)

    for name in [
        "doc_search_prepare",
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
    dag_id = bb.get_string("dag_id")
    if namespace_id:
        try:
            phase_data = fetch_phase(namespace_id, workdir, dag_id)
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
    if phase_data.get("focused_dag_id"):
        bb.set("focused_dag_id", phase_data.get("focused_dag_id"))
        if not bb.get_string("dag_id"):
            bb.set("dag_id", phase_data.get("focused_dag_id"))
    if phase_data.get("focus_source"):
        bb.set("focus_source", phase_data.get("focus_source"))
    return (True, None)


def dispatch_task_action(bb: Blackboard) -> tuple:
    """Skill-primary prepare-for-spawn.

    Calls Go dispatch provider which issues launch ticket + worktree only.
    Does NOT start the task (state stays assigned / rework_needed).
    Leader must spawn a real Agent then task_transition(start).
    """
    try:
        namespace_id = _require_namespace_id(bb)
        task_id = _require_first_task_id(bb, "next_tasks")
    except ValueError as err:
        return (False, err)

    if bb.get_string("phase") != "execute":
        return (False, ValueError("dispatch_task requires execute phase"))
    if not bb.get_bool("has_next_tasks"):
        return (False, ValueError("dispatch_task requires has_next_tasks=true"))

    try:
        result = dispatch_task(namespace_id, task_id)
    except DispatchProviderError as err:
        return (False, err)

    # Expect prepare-only: typically "assigned" (not "executing").
    bb.set("last_dispatch_task_id", result.get("task_id", "") or "")
    bb.set("last_dispatch_state", result.get("state", "") or "")
    bb.set("last_dispatch_worker", result.get("assigned_worker", "") or "")
    bb.set("last_dispatch_worktree_path", result.get("worktree_path", "") or "")
    bb.set("last_dispatch_branch", result.get("branch", "") or "")
    worker_launch = result.get("worker_launch") or {}
    if isinstance(worker_launch, dict):
        bb.set("last_dispatch_launch_ticket", worker_launch.get("launch_ticket", "") or "")
        bb.set("last_dispatch_worker_launch", worker_launch)
    return (True, None)


def monitor_tasks_action(bb: Blackboard) -> tuple:
    try:
        namespace_id = _require_namespace_id(bb)
        task_id = _require_first_task_id(bb, "active_tasks")
    except ValueError as err:
        return (False, err)

    if bb.get_string("phase") != "execute":
        return (False, ValueError("monitor_tasks requires execute phase"))
    if not bb.get_bool("has_active_tasks"):
        return (False, ValueError("monitor_tasks requires has_active_tasks=true"))

    try:
        result = monitor_task(namespace_id, task_id)
    except MonitorProviderError as err:
        return (False, err)

    bb.set("last_monitored_task_id", result.get("task_id", "") or "")
    bb.set("last_monitored_state", result.get("state", "") or "")
    bb.set("last_monitored_worker", result.get("assigned_worker", "") or "")
    bb.set("last_monitored_worktree_path", result.get("worktree_path", "") or "")
    bb.set("last_monitored_branch", result.get("branch", "") or "")
    bb.set("last_monitored_transitions", result.get("available_transitions", []) or [])
    return (True, None)


def report_stuck_action(bb: Blackboard) -> tuple:
    try:
        namespace_id = _require_namespace_id(bb)
        task_id = _require_first_task_id(bb, "stuck_tasks")
    except ValueError as err:
        return (False, err)

    if bb.get_string("phase") != "stuck":
        return (False, ValueError("report_stuck requires stuck phase"))
    if not bb.get_bool("has_stuck_tasks"):
        return (False, ValueError("report_stuck requires has_stuck_tasks=true"))

    try:
        result = report_stuck(namespace_id, task_id)
    except ReportStuckProviderError as err:
        return (False, err)

    bb.set("last_stuck_task_id", result.get("task_id", "") or "")
    bb.set("last_stuck_title", result.get("title", "") or "")
    bb.set("last_stuck_state", result.get("state", "") or "")
    bb.set("last_stuck_worker", result.get("assigned_worker", "") or "")
    bb.set("last_stuck_dag_id", result.get("dag_id", "") or "")
    bb.set("last_stuck_transitions", result.get("available_transitions", []) or [])
    bb.set("last_stuck_blockers", result.get("blockers", []) or [])
    bb.set("last_stuck_blocker_summary", result.get("blocker_summary", {}) or {})
    bb.set("last_stuck_suggested_actions", result.get("suggested_actions", []) or [])
    return (True, None)


def report_done_action(bb: Blackboard) -> tuple:
    namespace_id = bb.get_string("namespace_id") or bb.get_string("nsID")
    if not namespace_id:
        return (False, ValueError("namespace_id is required"))

    if bb.get_string("phase") != "done":
        return (False, ValueError("report_done requires done phase"))

    try:
        result = report_done(namespace_id)
    except ReportDoneProviderError as err:
        return (False, err)

    bb.set("last_done_phase", result.get("phase", "") or "")
    bb.set("last_done_phase_name", result.get("phase_name", "") or "")
    bb.set("last_done_progress", result.get("progress", "") or "")
    bb.set("last_done_completed_tasks", result.get("completed_tasks", 0) or 0)
    bb.set("last_done_total_tasks", result.get("total_tasks", 0) or 0)
    bb.set("last_done_completion_pct", result.get("completion_pct", 0) or 0)
    bb.set("last_done_dag", result.get("dag", {}) or {})
    bb.set("last_done_suggested_actions", result.get("suggested_actions", []) or [])
    bb.set("last_done_next_steps", result.get("next_steps", []) or [])
    return (True, None)


def task_get_confirm_action(bb: Blackboard) -> tuple:
    try:
        namespace_id = _require_namespace_id(bb)
        task_id = _require_task_id(bb, "task_id")
    except ValueError as err:
        return (False, err)

    worker_id = bb.get_string("worker_id")
    try:
        result = task_get_confirm(namespace_id, task_id, worker_id)
    except TaskGetConfirmProviderError as err:
        return (False, err)

    state = result.get("state", "") or ""
    if state not in {"assigned", "executing", "rework_needed"}:
        return (False, ValueError(f"task_get_confirm rejected state {state!r}"))

    assigned_worker = result.get("assigned_worker", "") or ""
    if worker_id and assigned_worker and worker_id != assigned_worker:
        return (False, ValueError(f"task_get_confirm worker mismatch: expected {worker_id!r}, got {assigned_worker!r}"))

    git = result.get("git", {}) or {}
    try:
        git_status, branch, repo_path, worktree_path = _require_git_payload("task_get_confirm", git)
    except ValueError as err:
        return (False, err)
    if git_status not in {"clean", "dirty", "missing"}:
        return (False, ValueError(f"task_get_confirm rejected git status {git_status!r}"))

    dag = result.get("dag", {}) or {}
    task = result.get("task", {}) or {}
    if not isinstance(dag, dict):
        dag = {}
    if not isinstance(task, dict):
        task = {}

    bb.set("task_confirmed", True)
    bb.set("confirmed_task", task)
    bb.set("confirmed_task_id", result.get("task_id", "") or "")
    bb.set("confirmed_task_title", result.get("title", "") or "")
    bb.set("confirmed_task_state", state)
    bb.set("confirmed_assigned_worker", assigned_worker)
    bb.set("confirmed_dag_id", dag.get("id", "") or "")
    bb.set("confirmed_dag_title", dag.get("title", "") or "")
    bb.set("confirmed_branch", branch)
    bb.set("confirmed_base_branch", git.get("base_branch", "") or "")
    bb.set("confirmed_repo_path", repo_path)
    bb.set("confirmed_worktree_path", worktree_path)
    bb.set("confirmed_head_sha", git.get("head_sha", "") or "")
    bb.set("confirmed_git_status", git_status)
    bb.set("confirmed_suggested_actions", result.get("suggested_actions", []) or [])
    return (True, None)


def enter_worktree_action(bb: Blackboard) -> tuple:
    try:
        namespace_id = _require_namespace_id(bb)
        task_id = _require_task_id(bb, "confirmed_task_id", "task_id")
    except ValueError as err:
        return (False, err)

    worker_id = bb.get_string("worker_id")
    try:
        result = enter_worktree(namespace_id, task_id, worker_id)
    except EnterWorktreeProviderError as err:
        return (False, err)

    assigned_worker = result.get("assigned_worker", "") or ""
    if worker_id and assigned_worker and worker_id != assigned_worker:
        return (False, ValueError(f"enter_worktree worker mismatch: expected {worker_id!r}, got {assigned_worker!r}"))

    git = result.get("git", {}) or {}
    try:
        git_status, branch, repo_path, worktree_path = _require_git_payload("enter_worktree", git)
    except ValueError as err:
        return (False, err)

    dag = result.get("dag", {}) or {}
    if not isinstance(dag, dict):
        dag = {}

    bb.set("worktree_ready", True)
    bb.set("entered_task_id", result.get("task_id", "") or "")
    bb.set("entered_assigned_worker", assigned_worker)
    bb.set("entered_branch", branch)
    bb.set("entered_base_branch", git.get("base_branch", "") or "")
    bb.set("entered_repo_path", repo_path)
    bb.set("entered_worktree_path", worktree_path)
    bb.set("entered_head_sha", git.get("head_sha", "") or "")
    bb.set("entered_git_status", git_status)
    bb.set("entered_dag_id", dag.get("id", "") or "")
    bb.set("entered_dag_title", dag.get("title", "") or "")
    return (True, None)


def implement_code_action(bb: Blackboard) -> tuple:
    try:
        namespace_id = _require_namespace_id(bb)
        task_id = _require_task_id(bb, "confirmed_task_id", "entered_task_id", "task_id")
        worker_id = _require_worker_id(bb)
    except ValueError as err:
        return (False, err)

    try:
        result = implement_code(namespace_id, task_id, worker_id)
    except ImplementCodeProviderError as err:
        return (False, err)

    prompt = result.get("prompt", "") or ""
    if not isinstance(prompt, str) or not prompt:
        return (False, ValueError("implement_code requires non-empty prompt"))

    git = result.get("git", {}) or {}
    worker = result.get("worker", {}) or {}
    task = result.get("task", {}) or {}
    dag = result.get("dag", {}) or {}
    try:
        _, branch, repo_path, worktree_path = _require_git_payload("implement_code", git)
    except ValueError as err:
        return (False, err)
    if not isinstance(worker, dict):
        return (False, ValueError("implement_code requires worker object"))
    if not isinstance(task, dict):
        task = {}
    if not isinstance(dag, dict):
        dag = {}

    returned_worker_id = worker.get("id", "") or ""
    if returned_worker_id and returned_worker_id != worker_id:
        return (False, ValueError(f"implement_code worker mismatch: expected {worker_id!r}, got {returned_worker_id!r}"))

    bb.set("implementation_ready", True)
    bb.set("implementation_task", task)
    bb.set("implementation_dag", dag)
    bb.set("implementation_worker", worker)
    bb.set("implementation_prompt", prompt)
    bb.set("implementation_branch", branch)
    bb.set("implementation_repo_path", repo_path)
    bb.set("implementation_worktree_path", worktree_path)
    bb.set("implementation_base_branch", git.get("base_branch", "") or "")
    bb.set("implementation_head_sha", git.get("head_sha", "") or "")
    bb.set("implementation_git_status", git.get("status", "") or "")
    bb.set("implementation_worker_id", returned_worker_id or worker_id)
    bb.set("implementation_worker_name", worker.get("name", "") or "")
    bb.set("implementation_scope", worker.get("scope", "") or "")
    bb.set("implementation_suggested_actions", result.get("suggested_actions", []) or [])
    return (True, None)


def git_commit_changes_action(bb: Blackboard) -> tuple:
    try:
        namespace_id = _require_namespace_id(bb)
        task_id = _require_task_id(bb, "confirmed_task_id", "entered_task_id", "task_id")
        worker_id = _require_worker_id(bb)
    except ValueError as err:
        return (False, err)

    try:
        result = git_commit_changes(namespace_id, task_id, worker_id)
    except GitCommitChangesProviderError as err:
        return (False, err)

    review = result.get("review", {}) or {}
    git = result.get("git", {}) or {}
    if not isinstance(review, dict):
        return (False, ValueError("git_commit_changes requires review object"))
    if not isinstance(git, dict):
        return (False, ValueError("git_commit_changes requires git object"))

    commit = review.get("commit", "") or ""
    if not isinstance(commit, str) or not commit:
        return (False, ValueError("git_commit_changes requires non-empty review commit"))

    bb.set("review_ready", True)
    bb.set("review_commit", commit)
    bb.set("review_diff", review.get("diff", "") or "")
    bb.set("commit_branch", git.get("branch", "") or "")
    bb.set("commit_worktree_path", git.get("worktree_path", "") or "")
    bb.set("commit_repo_path", git.get("repo_path", "") or "")
    if not bb.get_string("doc_record_content") and not bb.get_string("doc_content"):
        title = bb.get_string("confirmed_task_title") or bb.get_string("task_id") or task_id
        bb.set("doc_record_content", f"Completed task: {title}\n\nReview commit: {commit}")
    if not bb.get_string("diary_entry_content") and not bb.get_string("diary_content"):
        title = bb.get_string("confirmed_task_title") or bb.get_string("task_id") or task_id
        bb.set("diary_entry_content", f"Completed {title} at commit {commit}")
    return (True, None)


def task_submit_for_review_action(bb: Blackboard) -> tuple:
    try:
        namespace_id = _require_namespace_id(bb)
        task_id = _require_task_id(bb, "confirmed_task_id", "entered_task_id", "task_id")
        worker_id = _require_worker_id(bb)
    except ValueError as err:
        return (False, err)

    try:
        result = task_submit_for_review(namespace_id, task_id, worker_id)
    except TaskSubmitForReviewProviderError as err:
        return (False, err)

    state = result.get("state", "") or ""
    if state != "review_pending":
        return (False, ValueError(f"task_submit_for_review expected review_pending, got {state!r}"))

    review = result.get("review", {}) or {}
    if not isinstance(review, dict):
        return (False, ValueError("task_submit_for_review requires review object"))

    bb.set("submitted_for_review", True)
    bb.set("submitted_task_id", result.get("task_id", "") or "")
    bb.set("submitted_state", state)
    bb.set("submitted_review_commit", review.get("commit", "") or "")
    bb.set("submitted_review_diff", review.get("diff", "") or "")
    return (True, None)


def doc_write_record_action(bb: Blackboard) -> tuple:
    try:
        namespace_id = _require_namespace_id(bb)
        task_id = _require_task_id(bb, "confirmed_task_id", "entered_task_id", "task_id")
        worker_id = _require_worker_id(bb)
    except ValueError as err:
        return (False, err)

    content = bb.get_string("doc_record_content") or bb.get_string("doc_content")
    if not content:
        return (False, ValueError("doc_write_record requires doc_record_content or doc_content"))

    title = bb.get_string("doc_record_title") or bb.get_string("doc_title")
    path = bb.get_string("doc_record_path") or bb.get_string("doc_path")
    section = bb.get_string("doc_record_section") or bb.get_string("doc_section")
    tags = bb.get("doc_record_tags") or bb.get("doc_tags") or []
    if not isinstance(tags, list):
        return (False, ValueError("doc_write_record tags must be a list"))

    doc_id = bb.get("doc_record_id") or 0
    if not isinstance(doc_id, int):
        doc_id = 0

    try:
        result = doc_write_record(
            namespace_id,
            task_id,
            worker_id,
            content,
            title=title,
            path=path,
            section=section,
            tags=tags,
            doc_id=doc_id,
        )
    except DocWriteRecordProviderError as err:
        return (False, err)

    recorded_doc_id = result.get("doc_id", 0)
    if not isinstance(recorded_doc_id, int) or recorded_doc_id <= 0:
        return (False, ValueError("doc_write_record requires positive doc_id"))

    bb.set("doc_recorded", True)
    bb.set("recorded_doc_id", recorded_doc_id)
    bb.set("recorded_doc_title", result.get("title", "") or "")
    bb.set("recorded_doc_path", result.get("path", "") or "")
    bb.set("recorded_doc_section", result.get("section", "") or "")
    bb.set("recorded_doc_tags", result.get("tags", []) or [])
    return (True, None)


def diary_write_entry_action(bb: Blackboard) -> tuple:
    try:
        namespace_id = _require_namespace_id(bb)
        worker_id = _require_worker_id(bb)
    except ValueError as err:
        return (False, err)

    task_id = _require_task_id(bb, "confirmed_task_id", "entered_task_id", "task_id") if bb.get_string("confirmed_task_id") or bb.get_string("entered_task_id") or bb.get_string("task_id") else ""
    content = bb.get_string("diary_entry_content") or bb.get_string("diary_content")
    if not content:
        return (False, ValueError("diary_write_entry requires diary_entry_content or diary_content"))

    date = bb.get_string("diary_date")
    tags = bb.get("diary_entry_tags") or bb.get("diary_tags") or []
    if not isinstance(tags, list):
        return (False, ValueError("diary_write_entry tags must be a list"))

    try:
        result = diary_write_entry(
            namespace_id,
            worker_id,
            content,
            task_id=task_id,
            date=date,
            tags=tags,
        )
    except DiaryWriteEntryProviderError as err:
        return (False, err)

    returned_date = result.get("date", "") or ""
    if not isinstance(returned_date, str) or not returned_date:
        return (False, ValueError("diary_write_entry requires non-empty date"))

    bb.set("diary_written", True)
    bb.set("diary_date", returned_date)
    bb.set("diary_worker_id", result.get("worker_id", "") or worker_id)
    bb.set("diary_task_id", result.get("task_id", "") or task_id)
    bb.set("diary_tags", result.get("tags", []) or [])
    return (True, None)


def fetch_work_diff_action(bb: Blackboard) -> tuple:
    try:
        namespace_id = _require_namespace_id(bb)
        task_id = _require_task_id(bb, "task_id", "submitted_task_id")
        worker_id = _require_worker_id(bb)
    except ValueError as err:
        return (False, err)

    try:
        result = fetch_work_diff(namespace_id, task_id, worker_id)
    except FetchWorkDiffProviderError as err:
        return (False, err)

    state = result.get("state", "") or ""
    if state != "review_pending":
        return (False, ValueError(f"fetch_work_diff expected review_pending, got {state!r}"))

    review = result.get("review", {}) or {}
    if not isinstance(review, dict):
        return (False, ValueError("fetch_work_diff requires review object"))

    commit = review.get("commit", "") or ""
    if not isinstance(commit, str) or not commit:
        return (False, ValueError("fetch_work_diff requires non-empty review commit"))

    prompt = result.get("prompt", "") or ""
    if not isinstance(prompt, str) or not prompt:
        return (False, ValueError("fetch_work_diff requires non-empty prompt"))

    bb.set("review_context_ready", True)
    bb.set("review_fetch_task_id", result.get("task_id", "") or "")
    bb.set("review_fetch_state", state)
    bb.set("review_fetch_commit", commit)
    bb.set("review_fetch_diff", review.get("diff", "") or "")
    bb.set("review_prompt", prompt)
    return (True, None)


def review_decide_action(bb: Blackboard) -> tuple:
    if not bb.get_bool("review_context_ready"):
        return (False, ValueError("review_decide requires review_context_ready"))

    raw = bb.get("review_decision_input", None)
    if raw is None:
        raw = bb.get("review_decision", None)
    if raw is None:
        return (False, ValueError("review_decide requires review_decision_input or review_decision"))
    if not isinstance(raw, str):
        return (False, ValueError("review_decide decision input must be a string"))

    decision = raw.strip().lower()
    if decision in {"approve", "approved", "pass", "passed", "accept", "accepted", "true", "yes"}:
        bb.set("review_approved", True)
        bb.set("review_decision", "pass")
        return (True, None)
    if decision in {"rework", "reject", "rejected", "fail", "failed", "changes_requested", "false", "no"}:
        bb.set("review_approved", False)
        bb.set("review_decision", "rework")
        return (True, None)

    return (False, ValueError(f"review_decide unsupported decision {raw!r}"))


def task_review_pass_action(bb: Blackboard) -> tuple:
    return _apply_review_decision(
        bb,
        task_review_pass,
        TaskReviewPassProviderError,
        expected_state="done",
        decision="pass",
        action_name="task_review_pass",
    )


def task_review_rework_action(bb: Blackboard) -> tuple:
    return _apply_review_decision(
        bb,
        task_review_rework,
        TaskReviewReworkProviderError,
        expected_state="rework_needed",
        decision="rework",
        action_name="task_review_rework",
    )


def _noop_action(bb: Blackboard) -> tuple:
    return (True, None)
