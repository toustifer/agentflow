"""Tests for leaf nodes: Condition, Action, Inverter, Retry, Wait."""

from bt_service.core.node import Status
from bt_service.core.leaf import Condition, Action, Inverter, Retry, Wait, Log
from bt_service.core.blackboard import Blackboard


def bb():
    return Blackboard()


class TestCondition:
    def test_true(self):
        c = Condition(lambda b: True)
        assert c.tick(bb()) == Status.SUCCESS

    def test_false(self):
        c = Condition(lambda b: False)
        assert c.tick(bb()) == Status.FAILURE


class TestAction:
    def test_success(self):
        a = Action(lambda b: (True, None))
        assert a.tick(bb()) == Status.SUCCESS

    def test_failure(self):
        a = Action(lambda b: (False, None))
        assert a.tick(bb()) == Status.FAILURE

    def test_error(self):
        a = Action(lambda b: (False, Exception("boom")))
        assert a.tick(bb()) == Status.FAILURE
        assert a.error is not None
        assert "boom" in str(a.error)

    def test_exception_raised(self):
        def crash(b):
            raise ValueError("crash")
        a = Action(crash)
        assert a.tick(bb()) == Status.FAILURE
        assert a.error is not None


class TestInverter:
    def test_invert_failure_to_success(self):
        inv = Inverter(Condition(lambda b: False))
        assert inv.tick(bb()) == Status.SUCCESS

    def test_invert_success_to_failure(self):
        inv = Inverter(Condition(lambda b: True))
        assert inv.tick(bb()) == Status.FAILURE

    def test_running_passthrough(self):
        from bt_service.core.node import NodeFunc
        inv = Inverter(NodeFunc(lambda b: Status.RUNNING))
        assert inv.tick(bb()) == Status.RUNNING


class TestRetry:
    def test_succeeds_eventually(self):
        attempts = [0]

        def flaky(b):
            attempts[0] += 1
            return Status.SUCCESS if attempts[0] >= 2 else Status.FAILURE

        r = Retry(3, Condition(lambda b: None))
        # Replace with an action-style
        from bt_service.core.node import NodeFunc
        r = Retry(3, NodeFunc(flaky))
        assert r.tick(bb()) == Status.SUCCESS
        assert attempts[0] == 2

    def test_exhausted(self):
        r = Retry(2, Condition(lambda b: False))
        assert r.tick(bb()) == Status.FAILURE


class TestWait:
    def test_returns_running_then_success(self):
        w = Wait(3)
        assert w.tick(bb()) == Status.RUNNING
        assert w.tick(bb()) == Status.RUNNING
        assert w.tick(bb()) == Status.SUCCESS

    def test_single_tick(self):
        w = Wait(1)
        assert w.tick(bb()) == Status.SUCCESS

    def test_resets_after_success(self):
        w = Wait(2)
        assert w.tick(bb()) == Status.RUNNING
        assert w.tick(bb()) == Status.SUCCESS

        # Starts over
        assert w.tick(bb()) == Status.RUNNING

    def test_halt_resets(self):
        w = Wait(5)
        assert w.tick(bb()) == Status.RUNNING
        w.halt()
        assert w.tick(bb()) == Status.RUNNING


class TestLog:
    def test_passthrough_success(self):
        log = Log("test", Condition(lambda b: True))
        assert log.tick(bb()) == Status.SUCCESS

    def test_passthrough_failure(self):
        log = Log("test", Condition(lambda b: False))
        assert log.tick(bb()) == Status.FAILURE
