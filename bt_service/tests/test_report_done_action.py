from bt_service.core.blackboard import Blackboard
from bt_service.server.builtin import report_done_action
from bt_service.server.done_client import ReportDoneProviderError


def test_report_done_requires_namespace_id():
    bb = Blackboard.from_json_map({
        "phase": "done",
    })
    ok, err = report_done_action(bb)
    assert ok is False
    assert err is not None
    assert "namespace_id" in str(err)


def test_report_done_requires_done_phase():
    bb = Blackboard.from_json_map({
        "namespace_id": "ns-1",
        "phase": "execute",
    })
    ok, err = report_done_action(bb)
    assert ok is False
    assert err is not None
    assert "done phase" in str(err)


def test_report_done_provider_success_writes_blackboard(monkeypatch):
    def fake_report(namespace_id: str) -> dict:
        assert namespace_id == "ns-1"
        return {
            "phase": "done",
            "phase_name": "已完成（2/2）",
            "progress": "100%",
            "completed_tasks": 2,
            "total_tasks": 2,
            "completion_pct": 100,
            "dag": {"id": "dag-1", "title": "DAG 1", "branch": "feat/test", "status": "done"},
            "suggested_actions": ["goal", "doc_list"],
            "next_steps": ["项目已完成，可添加新功能（/agentflow goal + 目标）", "查看项目文档（doc_list）"],
        }

    monkeypatch.setattr("bt_service.server.builtin.report_done", fake_report)
    bb = Blackboard.from_json_map({
        "namespace_id": "ns-1",
        "phase": "done",
    })
    ok, err = report_done_action(bb)
    assert ok is True
    assert err is None
    assert bb.get("last_done_phase") == "done"
    assert bb.get("last_done_phase_name") == "已完成（2/2）"
    assert bb.get("last_done_progress") == "100%"
    assert bb.get("last_done_completed_tasks") == 2
    assert bb.get("last_done_total_tasks") == 2
    assert bb.get("last_done_completion_pct") == 100
    assert bb.get("last_done_dag") == {"id": "dag-1", "title": "DAG 1", "branch": "feat/test", "status": "done"}
    assert bb.get("last_done_suggested_actions") == ["goal", "doc_list"]
    assert bb.get("last_done_next_steps") == ["项目已完成，可添加新功能（/agentflow goal + 目标）", "查看项目文档（doc_list）"]


def test_report_done_provider_error_returns_failure(monkeypatch):
    def fake_report(namespace_id: str) -> dict:
        raise ReportDoneProviderError("boom")

    monkeypatch.setattr("bt_service.server.builtin.report_done", fake_report)
    bb = Blackboard.from_json_map({
        "namespace_id": "ns-1",
        "phase": "done",
    })
    ok, err = report_done_action(bb)
    assert ok is False
    assert err is not None
    assert "boom" in str(err)
