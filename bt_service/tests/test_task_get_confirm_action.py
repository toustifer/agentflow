from bt_service.core.blackboard import Blackboard
from bt_service.server.builtin import task_get_confirm_action
from bt_service.server.task_get_client import TaskGetConfirmProviderError


def test_task_get_confirm_requires_namespace_id():
    bb = Blackboard.from_json_map({"task_id": "T1"})
    ok, err = task_get_confirm_action(bb)
    assert ok is False
    assert err is not None
    assert "namespace_id" in str(err)


def test_task_get_confirm_requires_task_id():
    bb = Blackboard.from_json_map({"namespace_id": "ns-1"})
    ok, err = task_get_confirm_action(bb)
    assert ok is False
    assert err is not None
    assert "task_id" in str(err)


def test_task_get_confirm_rejects_invalid_state(monkeypatch):
    def fake_confirm(namespace_id: str, task_id: str, worker_id: str = "") -> dict:
        return {
            "task": {"id": "T1", "state": "done", "metadata": {}},
            "task_id": "T1",
            "title": "task 1",
            "state": "done",
            "assigned_worker": "worker-a",
            "dag": {"id": "dag-1", "title": "DAG 1", "branch": "feat/test"},
            "git": {"repo_path": "D:/repo", "worktree_path": "D:/wt/T1", "branch": "feat/test", "base_branch": "main", "head_sha": "abc", "status": "clean"},
            "suggested_actions": ["worktree_get", "worker_prompt_get"],
        }

    monkeypatch.setattr("bt_service.server.builtin.task_get_confirm", fake_confirm)
    bb = Blackboard.from_json_map({"namespace_id": "ns-1", "task_id": "T1"})
    ok, err = task_get_confirm_action(bb)
    assert ok is False
    assert err is not None
    assert "state" in str(err)


def test_task_get_confirm_accepts_missing_worktree_and_writes_blackboard(monkeypatch):
    def fake_confirm(namespace_id: str, task_id: str, worker_id: str = "") -> dict:
        assert namespace_id == "ns-1"
        assert task_id == "T1"
        assert worker_id == "worker-a"
        return {
            "task": {"id": "T1", "state": "assigned", "metadata": {}},
            "task_id": "T1",
            "title": "task 1",
            "state": "assigned",
            "assigned_worker": "worker-a",
            "dag": {"id": "dag-1", "title": "DAG 1", "branch": "feat/test"},
            "git": {"repo_path": "D:/repo", "worktree_path": "D:/wt/T1", "branch": "feat/test", "base_branch": "main", "head_sha": "", "status": "missing"},
            "suggested_actions": ["worktree_get", "worker_prompt_get"],
        }

    monkeypatch.setattr("bt_service.server.builtin.task_get_confirm", fake_confirm)
    bb = Blackboard.from_json_map({"namespace_id": "ns-1", "task_id": "T1", "worker_id": "worker-a"})
    ok, err = task_get_confirm_action(bb)
    assert ok is True
    assert err is None
    assert bb.get("task_confirmed") is True
    assert bb.get("confirmed_task_id") == "T1"
    assert bb.get("confirmed_task_title") == "task 1"
    assert bb.get("confirmed_task_state") == "assigned"
    assert bb.get("confirmed_assigned_worker") == "worker-a"
    assert bb.get("confirmed_dag_id") == "dag-1"
    assert bb.get("confirmed_dag_title") == "DAG 1"
    assert bb.get("confirmed_branch") == "feat/test"
    assert bb.get("confirmed_base_branch") == "main"
    assert bb.get("confirmed_repo_path") == "D:/repo"
    assert bb.get("confirmed_worktree_path") == "D:/wt/T1"
    assert bb.get("confirmed_head_sha") == ""
    assert bb.get("confirmed_git_status") == "missing"
    assert bb.get("confirmed_suggested_actions") == ["worktree_get", "worker_prompt_get"]


def test_task_get_confirm_rejects_worker_mismatch(monkeypatch):
    def fake_confirm(namespace_id: str, task_id: str, worker_id: str = "") -> dict:
        return {
            "task": {"id": "T1", "state": "assigned", "metadata": {}},
            "task_id": "T1",
            "title": "task 1",
            "state": "assigned",
            "assigned_worker": "worker-b",
            "dag": {"id": "dag-1", "title": "DAG 1", "branch": "feat/test"},
            "git": {"repo_path": "D:/repo", "worktree_path": "D:/wt/T1", "branch": "feat/test", "base_branch": "main", "head_sha": "", "status": "missing"},
            "suggested_actions": ["worktree_get", "worker_prompt_get"],
        }

    monkeypatch.setattr("bt_service.server.builtin.task_get_confirm", fake_confirm)
    bb = Blackboard.from_json_map({"namespace_id": "ns-1", "task_id": "T1", "worker_id": "worker-a"})
    ok, err = task_get_confirm_action(bb)
    assert ok is False
    assert err is not None
    assert "worker" in str(err)


def test_task_get_confirm_provider_error_returns_failure(monkeypatch):
    def fake_confirm(namespace_id: str, task_id: str, worker_id: str = "") -> dict:
        raise TaskGetConfirmProviderError("boom")

    monkeypatch.setattr("bt_service.server.builtin.task_get_confirm", fake_confirm)
    bb = Blackboard.from_json_map({"namespace_id": "ns-1", "task_id": "T1"})
    ok, err = task_get_confirm_action(bb)
    assert ok is False
    assert err is not None
    assert "boom" in str(err)
