from bt_service.core.blackboard import Blackboard
from bt_service.server.builtin import monitor_tasks_action
from bt_service.server.monitor_client import MonitorProviderError


def test_monitor_tasks_requires_namespace_id():
    bb = Blackboard.from_json_map({
        "phase": "execute",
        "has_active_tasks": True,
        "active_tasks": [{"task_id": "T1"}],
    })
    ok, err = monitor_tasks_action(bb)
    assert ok is False
    assert err is not None
    assert "namespace_id" in str(err)


def test_monitor_tasks_requires_active_tasks():
    bb = Blackboard.from_json_map({
        "namespace_id": "ns-1",
        "phase": "execute",
        "has_active_tasks": False,
        "active_tasks": [],
    })
    ok, err = monitor_tasks_action(bb)
    assert ok is False
    assert err is not None
    assert "active_tasks" in str(err)


def test_monitor_tasks_provider_success_writes_blackboard(monkeypatch):
    def fake_monitor(namespace_id: str, task_id: str) -> dict:
        assert namespace_id == "ns-1"
        assert task_id == "T1"
        return {
            "task_id": "T1",
            "state": "executing",
            "assigned_worker": "worker-a",
            "worktree_path": "D:/tmp/worktree/T1",
            "branch": "feat/test",
            "available_transitions": ["submit", "reassign", "cancel"],
        }

    monkeypatch.setattr("bt_service.server.builtin.monitor_task", fake_monitor)
    bb = Blackboard.from_json_map({
        "namespace_id": "ns-1",
        "phase": "execute",
        "has_active_tasks": True,
        "active_tasks": [{"task_id": "T1"}],
    })
    ok, err = monitor_tasks_action(bb)
    assert ok is True
    assert err is None
    assert bb.get("last_monitored_task_id") == "T1"
    assert bb.get("last_monitored_state") == "executing"
    assert bb.get("last_monitored_worker") == "worker-a"
    assert bb.get("last_monitored_worktree_path") == "D:/tmp/worktree/T1"
    assert bb.get("last_monitored_branch") == "feat/test"
    assert bb.get("last_monitored_transitions") == ["submit", "reassign", "cancel"]


def test_monitor_tasks_provider_error_returns_failure(monkeypatch):
    def fake_monitor(namespace_id: str, task_id: str) -> dict:
        raise MonitorProviderError("boom")

    monkeypatch.setattr("bt_service.server.builtin.monitor_task", fake_monitor)
    bb = Blackboard.from_json_map({
        "namespace_id": "ns-1",
        "phase": "execute",
        "has_active_tasks": True,
        "active_tasks": [{"task_id": "T1"}],
    })
    ok, err = monitor_tasks_action(bb)
    assert ok is False
    assert err is not None
    assert "boom" in str(err)
