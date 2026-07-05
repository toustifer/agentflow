from bt_service.core.blackboard import Blackboard
from bt_service.server.builtin import fetch_work_diff_action, review_decide_action
from bt_service.server.fetch_diff_client import FetchWorkDiffProviderError


def test_fetch_work_diff_requires_namespace_id():
    bb = Blackboard.from_json_map({"task_id": "T1", "worker_id": "reviewer-a"})
    ok, err = fetch_work_diff_action(bb)
    assert ok is False
    assert err is not None
    assert "namespace_id" in str(err)


def test_fetch_work_diff_requires_task_id():
    bb = Blackboard.from_json_map({"namespace_id": "ns-1", "worker_id": "reviewer-a"})
    ok, err = fetch_work_diff_action(bb)
    assert ok is False
    assert err is not None
    assert "task_id" in str(err)


def test_fetch_work_diff_requires_worker_id():
    bb = Blackboard.from_json_map({"namespace_id": "ns-1", "task_id": "T1"})
    ok, err = fetch_work_diff_action(bb)
    assert ok is False
    assert err is not None
    assert "worker_id" in str(err)


def test_fetch_work_diff_provider_success_writes_blackboard(monkeypatch):
    def fake_fetch(namespace_id: str, task_id: str, worker_id: str) -> dict:
        return {
            "task_id": "T1",
            "state": "review_pending",
            "assigned_worker": "worker-a",
            "review": {"commit": "abc", "diff": "diff --git a/x b/x"},
            "prompt": "rev_commit=abc rev_diff=diff --git",
        }

    monkeypatch.setattr("bt_service.server.builtin.fetch_work_diff", fake_fetch)
    bb = Blackboard.from_json_map({"namespace_id": "ns-1", "task_id": "T1", "worker_id": "reviewer-a"})
    ok, err = fetch_work_diff_action(bb)
    assert ok is True
    assert err is None
    assert bb.get("review_context_ready") is True
    assert bb.get("review_fetch_task_id") == "T1"
    assert bb.get("review_fetch_state") == "review_pending"
    assert bb.get("review_fetch_commit") == "abc"
    assert bb.get("review_fetch_diff") == "diff --git a/x b/x"
    assert bb.get("review_prompt") == "rev_commit=abc rev_diff=diff --git"
    assert bb.has("review_approved") is False


def test_fetch_work_diff_then_review_decide_sets_approval(monkeypatch):
    def fake_fetch(namespace_id: str, task_id: str, worker_id: str) -> dict:
        return {
            "task_id": "T1",
            "state": "review_pending",
            "assigned_worker": "worker-a",
            "review": {"commit": "abc", "diff": "diff --git a/x b/x"},
            "prompt": "rev_commit=abc rev_diff=diff --git",
        }

    monkeypatch.setattr("bt_service.server.builtin.fetch_work_diff", fake_fetch)
    bb = Blackboard.from_json_map({
        "namespace_id": "ns-1",
        "task_id": "T1",
        "worker_id": "reviewer-a",
        "review_decision_input": "approve",
    })
    ok, err = fetch_work_diff_action(bb)
    assert ok is True
    assert err is None
    ok, err = review_decide_action(bb)
    assert ok is True
    assert err is None
    assert bb.get("review_approved") is True
    assert bb.get("review_decision") == "pass"


    def fake_fetch(namespace_id: str, task_id: str, worker_id: str) -> dict:
        raise FetchWorkDiffProviderError("boom")

    monkeypatch.setattr("bt_service.server.builtin.fetch_work_diff", fake_fetch)
    bb = Blackboard.from_json_map({"namespace_id": "ns-1", "task_id": "T1", "worker_id": "reviewer-a"})
    ok, err = fetch_work_diff_action(bb)
    assert ok is False
    assert err is not None
    assert "boom" in str(err)
