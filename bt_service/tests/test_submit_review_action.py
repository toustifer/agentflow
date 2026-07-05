from bt_service.core.blackboard import Blackboard
from bt_service.server.builtin import task_submit_for_review_action
from bt_service.server.submit_review_client import TaskSubmitForReviewProviderError


def test_task_submit_for_review_requires_namespace_id():
    bb = Blackboard.from_json_map({"task_id": "T1", "worker_id": "worker-a"})
    ok, err = task_submit_for_review_action(bb)
    assert ok is False
    assert err is not None
    assert "namespace_id" in str(err)


def test_task_submit_for_review_requires_task_id():
    bb = Blackboard.from_json_map({"namespace_id": "ns-1", "worker_id": "worker-a"})
    ok, err = task_submit_for_review_action(bb)
    assert ok is False
    assert err is not None
    assert "task_id" in str(err)


def test_task_submit_for_review_requires_worker_id():
    bb = Blackboard.from_json_map({"namespace_id": "ns-1", "task_id": "T1"})
    ok, err = task_submit_for_review_action(bb)
    assert ok is False
    assert err is not None
    assert "worker_id" in str(err)


def test_task_submit_for_review_provider_success_writes_blackboard(monkeypatch):
    def fake_submit(namespace_id: str, task_id: str, worker_id: str) -> dict:
        return {
            "task_id": "T1",
            "state": "review_pending",
            "assigned_worker": "worker-a",
            "review": {"commit": "abc", "diff": "diff --git a/x b/x"},
        }

    monkeypatch.setattr("bt_service.server.builtin.task_submit_for_review", fake_submit)
    bb = Blackboard.from_json_map({"namespace_id": "ns-1", "task_id": "T1", "worker_id": "worker-a"})
    ok, err = task_submit_for_review_action(bb)
    assert ok is True
    assert err is None
    assert bb.get("submitted_for_review") is True
    assert bb.get("submitted_task_id") == "T1"
    assert bb.get("submitted_state") == "review_pending"
    assert bb.get("submitted_review_commit") == "abc"
    assert bb.get("submitted_review_diff") == "diff --git a/x b/x"


def test_task_submit_for_review_provider_error_returns_failure(monkeypatch):
    def fake_submit(namespace_id: str, task_id: str, worker_id: str) -> dict:
        raise TaskSubmitForReviewProviderError("boom")

    monkeypatch.setattr("bt_service.server.builtin.task_submit_for_review", fake_submit)
    bb = Blackboard.from_json_map({"namespace_id": "ns-1", "task_id": "T1", "worker_id": "worker-a"})
    ok, err = task_submit_for_review_action(bb)
    assert ok is False
    assert err is not None
    assert "boom" in str(err)
