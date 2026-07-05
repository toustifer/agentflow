from bt_service.core.blackboard import Blackboard
from bt_service.server.builtin import implement_code_action
from bt_service.server.implement_client import ImplementCodeProviderError


def test_implement_code_requires_namespace_id():
    bb = Blackboard.from_json_map({"task_id": "T1", "worker_id": "worker-a"})
    ok, err = implement_code_action(bb)
    assert ok is False
    assert err is not None
    assert "namespace_id" in str(err)


def test_implement_code_requires_task_id():
    bb = Blackboard.from_json_map({"namespace_id": "ns-1", "worker_id": "worker-a"})
    ok, err = implement_code_action(bb)
    assert ok is False
    assert err is not None
    assert "task_id" in str(err)


def test_implement_code_requires_worker_id():
    bb = Blackboard.from_json_map({"namespace_id": "ns-1", "task_id": "T1"})
    ok, err = implement_code_action(bb)
    assert ok is False
    assert err is not None
    assert "worker_id" in str(err)


def test_implement_code_rejects_empty_prompt(monkeypatch):
    def fake_implement(namespace_id: str, task_id: str, worker_id: str) -> dict:
        return {
            "task": {"id": "T1"},
            "dag": {"id": "dag-1", "title": "DAG 1", "branch": "feat/test"},
            "git": {"repo_path": "D:/repo", "worktree_path": "D:/wt/T1", "branch": "feat/test", "base_branch": "main", "head_sha": "abc", "status": "clean"},
            "worker": {"id": "worker-a", "name": "Worker A", "scope": "backend"},
            "prompt": "",
            "suggested_actions": ["worker_prompt_get", "worktree_get", "git_status"],
        }

    monkeypatch.setattr("bt_service.server.builtin.implement_code", fake_implement)
    bb = Blackboard.from_json_map({"namespace_id": "ns-1", "task_id": "T1", "worker_id": "worker-a"})
    ok, err = implement_code_action(bb)
    assert ok is False
    assert err is not None
    assert "prompt" in str(err)


def test_implement_code_provider_success_writes_blackboard(monkeypatch):
    def fake_implement(namespace_id: str, task_id: str, worker_id: str) -> dict:
        return {
            "task": {"id": "T1", "title": "task 1", "state": "executing"},
            "dag": {"id": "dag-1", "title": "DAG 1", "branch": "feat/test"},
            "git": {"repo_path": "D:/repo", "worktree_path": "D:/wt/T1", "branch": "feat/test", "base_branch": "main", "head_sha": "abc", "status": "clean"},
            "worker": {"id": "worker-a", "name": "Worker A", "scope": "backend"},
            "prompt": "Implement the task in the worktree.",
            "suggested_actions": ["worker_prompt_get", "worktree_get", "git_status"],
        }

    monkeypatch.setattr("bt_service.server.builtin.implement_code", fake_implement)
    bb = Blackboard.from_json_map({"namespace_id": "ns-1", "task_id": "T1", "worker_id": "worker-a"})
    ok, err = implement_code_action(bb)
    assert ok is True
    assert err is None
    assert bb.get("implementation_ready") is True
    assert bb.get("implementation_prompt") == "Implement the task in the worktree."
    assert bb.get("implementation_branch") == "feat/test"
    assert bb.get("implementation_repo_path") == "D:/repo"
    assert bb.get("implementation_worktree_path") == "D:/wt/T1"
    assert bb.get("implementation_base_branch") == "main"
    assert bb.get("implementation_head_sha") == "abc"
    assert bb.get("implementation_git_status") == "clean"
    assert bb.get("implementation_worker_id") == "worker-a"
    assert bb.get("implementation_worker_name") == "Worker A"
    assert bb.get("implementation_scope") == "backend"
    assert bb.get("implementation_suggested_actions") == ["worker_prompt_get", "worktree_get", "git_status"]


def test_implement_code_provider_error_returns_failure(monkeypatch):
    def fake_implement(namespace_id: str, task_id: str, worker_id: str) -> dict:
        raise ImplementCodeProviderError("boom")

    monkeypatch.setattr("bt_service.server.builtin.implement_code", fake_implement)
    bb = Blackboard.from_json_map({"namespace_id": "ns-1", "task_id": "T1", "worker_id": "worker-a"})
    ok, err = implement_code_action(bb)
    assert ok is False
    assert err is not None
    assert "boom" in str(err)
