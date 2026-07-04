"""Tests for control nodes: Sequence, Fallback, ReactiveSequence, ReactiveFallback."""

from bt_service.core.node import Node, Status, NodeFunc
from bt_service.core.control import Sequence, Fallback, ReactiveSequence, ReactiveFallback
from bt_service.core.blackboard import Blackboard


def bb():
    return Blackboard()


class TestSequence:
    def test_all_success(self):
        order = []

        def a(b):
            order.append("A")
            return Status.SUCCESS

        def b(b):
            order.append("B")
            return Status.SUCCESS

        s = Sequence([NodeFunc(a), NodeFunc(b)])
        assert s.tick(bb()) == Status.SUCCESS
        assert order == ["A", "B"]

    def test_failure_stops(self):
        order = []

        def a(b):
            order.append("A")
            return Status.SUCCESS

        def b(b):
            order.append("B")
            return Status.FAILURE

        def c(b):
            order.append("C")
            return Status.SUCCESS

        s = Sequence([NodeFunc(a), NodeFunc(b), NodeFunc(c)])
        assert s.tick(bb()) == Status.FAILURE
        assert order == ["A", "B"]

    def test_running(self):
        order = []

        def a(b):
            order.append("A")
            return Status.SUCCESS

        def b(b):
            order.append("B")
            return Status.RUNNING

        s = Sequence([NodeFunc(a), NodeFunc(b)])
        assert s.tick(bb()) == Status.RUNNING
        assert order == ["A", "B"]

        # Second tick resumes at B
        assert s.tick(bb()) == Status.RUNNING
        assert order == ["A", "B", "B"]

    def test_resets_after_complete(self):
        calls = []

        def a(b):
            calls.append("A")
            return Status.SUCCESS

        def b(b):
            calls.append("B")
            return Status.SUCCESS

        s = Sequence([NodeFunc(a), NodeFunc(b)])
        assert s.tick(bb()) == Status.SUCCESS
        assert len(calls) == 2
        assert s.tick(bb()) == Status.SUCCESS
        assert len(calls) == 4


class TestFallback:
    def test_first_success(self):
        def a(b):
            return Status.SUCCESS

        s = Fallback([NodeFunc(a), NodeFunc(lambda b: 1 / 0)])  # should not tick
        assert s.tick(bb()) == Status.SUCCESS

    def test_all_fail(self):
        s = Fallback([
            NodeFunc(lambda b: Status.FAILURE),
            NodeFunc(lambda b: Status.FAILURE),
        ])
        assert s.tick(bb()) == Status.FAILURE

    def test_fallthrough_to_second(self):
        order = []

        def a(b):
            order.append("1-fail")
            return Status.FAILURE

        def b(b):
            order.append("2-ok")
            return Status.SUCCESS

        s = Fallback([NodeFunc(a), NodeFunc(b)])
        assert s.tick(bb()) == Status.SUCCESS
        assert order == ["1-fail", "2-ok"]

    def test_running(self):
        s = Fallback([
            NodeFunc(lambda b: Status.FAILURE),
            NodeFunc(lambda b: Status.RUNNING),
        ])
        assert s.tick(bb()) == Status.RUNNING
        assert s.tick(bb()) == Status.RUNNING

    def test_resets_on_new_tick(self):
        calls = []

        def a(b):
            calls.append("A")
            return Status.SUCCESS

        s = Fallback([NodeFunc(a), NodeFunc(lambda b: Status.SUCCESS)])
        assert s.tick(bb()) == Status.SUCCESS
        assert len(calls) == 1
        assert s.tick(bb()) == Status.SUCCESS
        assert len(calls) == 2


class TestReactiveSequence:
    def test_all_success(self):
        rs = ReactiveSequence([
            NodeFunc(lambda b: Status.SUCCESS),
            NodeFunc(lambda b: Status.SUCCESS),
        ])
        assert rs.tick(bb()) == Status.SUCCESS

    def test_failure(self):
        rs = ReactiveSequence([
            NodeFunc(lambda b: Status.SUCCESS),
            NodeFunc(lambda b: Status.FAILURE),
        ])
        assert rs.tick(bb()) == Status.FAILURE

    def test_running_with_failure(self):
        """If one child is Running and another fails, failure wins."""
        rs = ReactiveSequence([
            NodeFunc(lambda b: Status.RUNNING),
            NodeFunc(lambda b: Status.FAILURE),
        ])
        assert rs.tick(bb()) == Status.FAILURE

    def test_running_only(self):
        rs = ReactiveSequence([
            NodeFunc(lambda b: Status.RUNNING),
            NodeFunc(lambda b: Status.RUNNING),
        ])
        assert rs.tick(bb()) == Status.RUNNING


class TestReactiveFallback:
    def test_success(self):
        rf = ReactiveFallback([
            NodeFunc(lambda b: Status.FAILURE),
            NodeFunc(lambda b: Status.SUCCESS),
        ])
        assert rf.tick(bb()) == Status.SUCCESS

    def test_all_fail(self):
        rf = ReactiveFallback([
            NodeFunc(lambda b: Status.FAILURE),
            NodeFunc(lambda b: Status.FAILURE),
        ])
        assert rf.tick(bb()) == Status.FAILURE

    def test_running(self):
        rf = ReactiveFallback([
            NodeFunc(lambda b: Status.RUNNING),
            NodeFunc(lambda b: Status.SUCCESS),
        ])
        assert rf.tick(bb()) == Status.SUCCESS

    def test_running_only(self):
        rf = ReactiveFallback([
            NodeFunc(lambda b: Status.RUNNING),
            NodeFunc(lambda b: Status.RUNNING),
        ])
        assert rf.tick(bb()) == Status.RUNNING
