from bt_service.core.blackboard import Blackboard
from bt_service.server.builtin import task_review_pass_action, task_review_rework_action
from bt_service.server.review_decision_client import TaskReviewPassProviderError, TaskReviewReworkProviderError


def test_task_review_pass_success_writes_blackboard(monkeypatch):
    monkeypatch.setattr("bt_service.server.builtin.task_review_pass", lambda ns, tid, wid: {"task_id": "T1", "state": "done"})
    bb = Blackboard.from_json_map({"namespace_id": "ns-1", "task_id": "T1", "worker_id": "reviewer-a"})
    ok, err = task_review_pass_action(bb)
    assert ok is True
    assert err is None
    assert bb.get("review_decision") == "pass"
    assert bb.get("review_decision_task_id") == "T1"
    assert bb.get("review_decision_state") == "done"


def test_task_review_rework_success_writes_blackboard(monkeypatch):
    monkeypatch.setattr("bt_service.server.builtin.task_review_rework", lambda ns, tid, wid: {"task_id": "T1", "state": "rework_needed"})
    bb = Blackboard.from_json_map({"namespace_id": "ns-1", "task_id": "T1", "worker_id": "reviewer-a"})
    ok, err = task_review_rework_action(bb)
    assert ok is True
    assert err is None
    assert bb.get("review_decision") == "rework"
    assert bb.get("review_decision_task_id") == "T1"
    assert bb.get("review_decision_state") == "rework_needed"


def test_task_review_pass_provider_error(monkeypatch):
    def fake(ns, tid, wid):
        raise TaskReviewPassProviderError("boom")

    monkeypatch.setattr("bt_service.server.builtin.task_review_pass", fake)
    bb = Blackboard.from_json_map({"namespace_id": "ns-1", "task_id": "T1", "worker_id": "reviewer-a"})
    ok, err = task_review_pass_action(bb)
    assert ok is False
    assert err is not None
    assert "boom" in str(err)


def test_task_review_rework_provider_error(monkeypatch):
    def fake(ns, tid, wid):
        raise TaskReviewReworkProviderError("boom")

    monkeypatch.setattr("bt_service.server.builtin.task_review_rework", fake)
    bb = Blackboard.from_json_map({"namespace_id": "ns-1", "task_id": "T1", "worker_id": "reviewer-a"})
    ok, err = task_review_rework_action(bb)
    assert ok is False
    assert err is not None
    assert "boom" in str(err)
