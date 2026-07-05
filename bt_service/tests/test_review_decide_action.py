from bt_service.core.blackboard import Blackboard
from bt_service.server.builtin import review_decide_action


def test_review_decide_requires_review_context_ready():
    bb = Blackboard.from_json_map({"review_decision_input": "approve"})
    ok, err = review_decide_action(bb)
    assert ok is False
    assert err is not None
    assert "review_context_ready" in str(err)


def test_review_decide_requires_input():
    bb = Blackboard.from_json_map({"review_context_ready": True})
    ok, err = review_decide_action(bb)
    assert ok is False
    assert err is not None
    assert "review_decision_input or review_decision" in str(err)


def test_review_decide_sets_pass(monkeypatch):
    bb = Blackboard.from_json_map({"review_context_ready": True, "review_decision_input": "approve"})
    ok, err = review_decide_action(bb)
    assert ok is True
    assert err is None
    assert bb.get("review_approved") is True
    assert bb.get("review_decision") == "pass"


def test_review_decide_sets_rework():
    bb = Blackboard.from_json_map({"review_context_ready": True, "review_decision_input": "rework"})
    ok, err = review_decide_action(bb)
    assert ok is True
    assert err is None
    assert bb.get("review_approved") is False
    assert bb.get("review_decision") == "rework"


def test_review_decide_rejects_unknown_value():
    bb = Blackboard.from_json_map({"review_context_ready": True, "review_decision_input": "maybe"})
    ok, err = review_decide_action(bb)
    assert ok is False
    assert err is not None
    assert "unsupported decision" in str(err)
