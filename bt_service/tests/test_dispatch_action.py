from bt_service.core.blackboard import Blackboard
from bt_service.server.builtin import dispatch_task_action
from bt_service.server.dispatch_client import DispatchProviderError


def test_dispatch_task_requires_namespace_id():
    bb = Blackboard.from_json_map({
        "phase": "execute",
        "has_next_tasks": True,
        "next_tasks": [{"task_id": "T1"}],
    })
    ok, err = dispatch_task_action(bb)
    assert ok is False
    assert err is not None
    assert "namespace_id" in str(err)


def test_dispatch_task_requires_execute_phase():
    bb = Blackboard.from_json_map({
        "namespace_id": "ns-1",
        "phase": "shape",
        "has_next_tasks": True,
        "next_tasks": [{"task_id": "T1"}],
    })
    ok, err = dispatch_task_action(bb)
    assert ok is False
    assert err is not None
    assert "execute phase" in str(err)


def test_dispatch_task_requires_next_tasks():
    bb = Blackboard.from_json_map({
        "namespace_id": "ns-1",
        "phase": "execute",
        "has_next_tasks": False,
        "next_tasks": [],
    })
    ok, err = dispatch_task_action(bb)
    assert ok is False
    assert err is not None
    assert "next_tasks" in str(err)


def test_dispatch_task_provider_success_writes_blackboard(monkeypatch):
    def fake_dispatch(namespace_id: str, task_id: str) -> dict:
        assert namespace_id == "ns-1"
        assert task_id == "T1"
        return {
            "task_id": "T1",
            "state": "executing",
            "assigned_worker": "worker-a",
            "worktree_path": "D:/tmp/worktree/T1",
            "branch": "feat/test",
        }

    monkeypatch.setattr("bt_service.server.builtin.dispatch_task", fake_dispatch)
    bb = Blackboard.from_json_map({
        "namespace_id": "ns-1",
        "phase": "execute",
        "has_next_tasks": True,
        "next_tasks": [{"task_id": "T1"}],
    })
    ok, err = dispatch_task_action(bb)
    assert ok is True
    assert err is None
    assert bb.get("last_dispatch_task_id") == "T1"
    assert bb.get("last_dispatch_state") == "executing"
    assert bb.get("last_dispatch_worker") == "worker-a"
    assert bb.get("last_dispatch_worktree_path") == "D:/tmp/worktree/T1"
    assert bb.get("last_dispatch_branch") == "feat/test"


def test_dispatch_task_provider_error_returns_failure(monkeypatch):
    def fake_dispatch(namespace_id: str, task_id: str) -> dict:
        raise DispatchProviderError("boom")

    monkeypatch.setattr("bt_service.server.builtin.dispatch_task", fake_dispatch)
    bb = Blackboard.from_json_map({
        "namespace_id": "ns-1",
        "phase": "execute",
        "has_next_tasks": True,
        "next_tasks": [{"task_id": "T1"}],
    })
    ok, err = dispatch_task_action(bb)
    assert ok is False
    assert err is not None
    assert "boom" in str(err)
