from bt_service.core.blackboard import Blackboard
from bt_service.server.builtin import report_stuck_action
from bt_service.server.stuck_client import ReportStuckProviderError


def test_report_stuck_requires_namespace_id():
    bb = Blackboard.from_json_map({
        "phase": "stuck",
        "has_stuck_tasks": True,
        "stuck_tasks": [{"task_id": "T1"}],
    })
    ok, err = report_stuck_action(bb)
    assert ok is False
    assert err is not None
    assert "namespace_id" in str(err)


def test_report_stuck_requires_stuck_phase():
    bb = Blackboard.from_json_map({
        "namespace_id": "ns-1",
        "phase": "execute",
        "has_stuck_tasks": True,
        "stuck_tasks": [{"task_id": "T1"}],
    })
    ok, err = report_stuck_action(bb)
    assert ok is False
    assert err is not None
    assert "stuck phase" in str(err)


def test_report_stuck_requires_stuck_tasks():
    bb = Blackboard.from_json_map({
        "namespace_id": "ns-1",
        "phase": "stuck",
        "has_stuck_tasks": False,
        "stuck_tasks": [],
    })
    ok, err = report_stuck_action(bb)
    assert ok is False
    assert err is not None
    assert "stuck_tasks" in str(err)


def test_report_stuck_provider_success_writes_blackboard(monkeypatch):
    def fake_report(namespace_id: str, task_id: str) -> dict:
        assert namespace_id == "ns-1"
        assert task_id == "T1"
        return {
            "task_id": "T1",
            "title": "task 1",
            "state": "rework_needed",
            "assigned_worker": "worker-a",
            "dag_id": "dag-1",
            "available_transitions": ["resume", "reassign", "cancel"],
            "blockers": [{"task_id": "T1", "type": "dependency", "blocked_by": "T0"}],
            "blocker_summary": {"total": 1, "dependency": 1, "worker": 0},
            "suggested_actions": ["task_get", "project_blockers"],
        }

    monkeypatch.setattr("bt_service.server.builtin.report_stuck", fake_report)
    bb = Blackboard.from_json_map({
        "namespace_id": "ns-1",
        "phase": "stuck",
        "has_stuck_tasks": True,
        "stuck_tasks": [{"task_id": "T1"}],
    })
    ok, err = report_stuck_action(bb)
    assert ok is True
    assert err is None
    assert bb.get("last_stuck_task_id") == "T1"
    assert bb.get("last_stuck_title") == "task 1"
    assert bb.get("last_stuck_state") == "rework_needed"
    assert bb.get("last_stuck_worker") == "worker-a"
    assert bb.get("last_stuck_dag_id") == "dag-1"
    assert bb.get("last_stuck_transitions") == ["resume", "reassign", "cancel"]
    assert bb.get("last_stuck_blockers") == [{"task_id": "T1", "type": "dependency", "blocked_by": "T0"}]
    assert bb.get("last_stuck_blocker_summary") == {"total": 1, "dependency": 1, "worker": 0}
    assert bb.get("last_stuck_suggested_actions") == ["task_get", "project_blockers"]


def test_report_stuck_provider_error_returns_failure(monkeypatch):
    def fake_report(namespace_id: str, task_id: str) -> dict:
        raise ReportStuckProviderError("boom")

    monkeypatch.setattr("bt_service.server.builtin.report_stuck", fake_report)
    bb = Blackboard.from_json_map({
        "namespace_id": "ns-1",
        "phase": "stuck",
        "has_stuck_tasks": True,
        "stuck_tasks": [{"task_id": "T1"}],
    })
    ok, err = report_stuck_action(bb)
    assert ok is False
    assert err is not None
    assert "boom" in str(err)
