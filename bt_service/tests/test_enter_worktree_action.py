from bt_service.core.blackboard import Blackboard
from bt_service.server.builtin import enter_worktree_action
from bt_service.server.enter_worktree_client import EnterWorktreeProviderError


def test_enter_worktree_requires_namespace_id():
    bb = Blackboard.from_json_map({"task_id": "T1"})
    ok, err = enter_worktree_action(bb)
    assert ok is False
    assert err is not None
    assert "namespace_id" in str(err)


def test_enter_worktree_requires_task_id():
    bb = Blackboard.from_json_map({"namespace_id": "ns-1"})
    ok, err = enter_worktree_action(bb)
    assert ok is False
    assert err is not None
    assert "task_id" in str(err)


def test_enter_worktree_rejects_worker_mismatch(monkeypatch):
    def fake_enter(namespace_id: str, task_id: str, worker_id: str = "") -> dict:
        return {
            "task_id": "T1",
            "state": "executing",
            "assigned_worker": "worker-b",
            "dag": {"id": "dag-1", "title": "DAG 1", "branch": "feat/test"},
            "git": {"repo_path": "D:/repo", "worktree_path": "D:/wt/T1", "branch": "feat/test", "base_branch": "main", "head_sha": "abc", "status": "clean"},
        }

    monkeypatch.setattr("bt_service.server.builtin.enter_worktree", fake_enter)
    bb = Blackboard.from_json_map({"namespace_id": "ns-1", "task_id": "T1", "worker_id": "worker-a"})
    ok, err = enter_worktree_action(bb)
    assert ok is False
    assert err is not None
    assert "worker" in str(err)


def test_enter_worktree_rejects_branch_mismatch(monkeypatch):
    def fake_enter(namespace_id: str, task_id: str, worker_id: str = "") -> dict:
        return {
            "task_id": "T1",
            "state": "executing",
            "assigned_worker": "worker-a",
            "dag": {"id": "dag-1", "title": "DAG 1", "branch": "feat/test"},
            "git": {"repo_path": "D:/repo", "worktree_path": "D:/wt/T1", "branch": "feat/test", "base_branch": "main", "head_sha": "abc", "status": "branch_mismatch"},
        }

    monkeypatch.setattr("bt_service.server.builtin.enter_worktree", fake_enter)
    bb = Blackboard.from_json_map({"namespace_id": "ns-1", "task_id": "T1", "worker_id": "worker-a"})
    ok, err = enter_worktree_action(bb)
    assert ok is False
    assert err is not None
    assert "branch_mismatch" in str(err)


def test_enter_worktree_provider_success_writes_blackboard(monkeypatch):
    def fake_enter(namespace_id: str, task_id: str, worker_id: str = "") -> dict:
        assert namespace_id == "ns-1"
        assert task_id == "T1"
        assert worker_id == "worker-a"
        return {
            "task_id": "T1",
            "state": "executing",
            "assigned_worker": "worker-a",
            "dag": {"id": "dag-1", "title": "DAG 1", "branch": "feat/test"},
            "git": {"repo_path": "D:/repo", "worktree_path": "D:/wt/T1", "branch": "feat/test", "base_branch": "main", "head_sha": "abc", "status": "clean"},
        }

    monkeypatch.setattr("bt_service.server.builtin.enter_worktree", fake_enter)
    bb = Blackboard.from_json_map({"namespace_id": "ns-1", "task_id": "T1", "worker_id": "worker-a"})
    ok, err = enter_worktree_action(bb)
    assert ok is True
    assert err is None
    assert bb.get("worktree_ready") is True
    assert bb.get("entered_task_id") == "T1"
    assert bb.get("entered_assigned_worker") == "worker-a"
    assert bb.get("entered_branch") == "feat/test"
    assert bb.get("entered_base_branch") == "main"
    assert bb.get("entered_repo_path") == "D:/repo"
    assert bb.get("entered_worktree_path") == "D:/wt/T1"
    assert bb.get("entered_head_sha") == "abc"
    assert bb.get("entered_git_status") == "clean"


def test_enter_worktree_provider_error_returns_failure(monkeypatch):
    def fake_enter(namespace_id: str, task_id: str, worker_id: str = "") -> dict:
        raise EnterWorktreeProviderError("boom")

    monkeypatch.setattr("bt_service.server.builtin.enter_worktree", fake_enter)
    bb = Blackboard.from_json_map({"namespace_id": "ns-1", "task_id": "T1"})
    ok, err = enter_worktree_action(bb)
    assert ok is False
    assert err is not None
    assert "boom" in str(err)
