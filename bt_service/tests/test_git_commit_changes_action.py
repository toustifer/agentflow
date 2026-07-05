from bt_service.core.blackboard import Blackboard
from bt_service.server.builtin import git_commit_changes_action
from bt_service.server.git_commit_client import GitCommitChangesProviderError


def test_git_commit_changes_requires_namespace_id():
    bb = Blackboard.from_json_map({"task_id": "T1", "worker_id": "worker-a"})
    ok, err = git_commit_changes_action(bb)
    assert ok is False
    assert err is not None
    assert "namespace_id" in str(err)


def test_git_commit_changes_requires_task_id():
    bb = Blackboard.from_json_map({"namespace_id": "ns-1", "worker_id": "worker-a"})
    ok, err = git_commit_changes_action(bb)
    assert ok is False
    assert err is not None
    assert "task_id" in str(err)


def test_git_commit_changes_requires_worker_id():
    bb = Blackboard.from_json_map({"namespace_id": "ns-1", "task_id": "T1"})
    ok, err = git_commit_changes_action(bb)
    assert ok is False
    assert err is not None
    assert "worker_id" in str(err)


def test_git_commit_changes_provider_success_writes_blackboard(monkeypatch):
    def fake_commit(namespace_id: str, task_id: str, worker_id: str) -> dict:
        return {
            "task_id": "T1",
            "state": "executing",
            "assigned_worker": "worker-a",
            "git": {"repo_path": "D:/repo", "worktree_path": "D:/wt/T1", "branch": "feat/test", "base_branch": "main", "head_sha": "abc", "status": "clean"},
            "review": {"commit": "abc", "diff": "diff --git a/x b/x"},
        }

    monkeypatch.setattr("bt_service.server.builtin.git_commit_changes", fake_commit)
    bb = Blackboard.from_json_map({"namespace_id": "ns-1", "task_id": "T1", "worker_id": "worker-a"})
    ok, err = git_commit_changes_action(bb)
    assert ok is True
    assert err is None
    assert bb.get("review_ready") is True
    assert bb.get("review_commit") == "abc"
    assert bb.get("review_diff") == "diff --git a/x b/x"
    assert bb.get("commit_branch") == "feat/test"
    assert bb.get("commit_worktree_path") == "D:/wt/T1"


def test_git_commit_changes_provider_error_returns_failure(monkeypatch):
    def fake_commit(namespace_id: str, task_id: str, worker_id: str) -> dict:
        raise GitCommitChangesProviderError("boom")

    monkeypatch.setattr("bt_service.server.builtin.git_commit_changes", fake_commit)
    bb = Blackboard.from_json_map({"namespace_id": "ns-1", "task_id": "T1", "worker_id": "worker-a"})
    ok, err = git_commit_changes_action(bb)
    assert ok is False
    assert err is not None
    assert "boom" in str(err)
