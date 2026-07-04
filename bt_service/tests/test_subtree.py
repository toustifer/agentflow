"""Tests for SubTree and Registry."""

from bt_service.core.node import Status
from bt_service.core.blackboard import Blackboard
from bt_service.core.subtree import SubTree
from bt_service.core.registry import Registry
from bt_service.core.leaf import Condition, Wait
from bt_service.factory.registry import FactoryRegistry, register_default_nodes


def bb():
    return Blackboard()


class TestRegistry:
    def test_round_trip(self):
        reg = Registry()
        reg.register("test", Condition(lambda b: True))
        assert "test" in reg.list()
        status, err = reg.tick("test", bb())
        assert status == Status.SUCCESS
        assert err == ""

    def test_not_found(self):
        reg = Registry()
        status, err = reg.tick("nope", bb())
        assert status == Status.FAILURE
        assert "nope" in err


class TestSubTree:
    def test_basic(self):
        tree_reg = Registry()
        tree_reg.register("inner", Condition(lambda b: True))

        st = SubTree("inner", tree_reg)
        assert st.tick(bb()) == Status.SUCCESS

    def test_with_blackboard(self):
        tree_reg = Registry()
        tree_reg.register("inner", Condition(lambda b: b.get_string("x") != ""))

        st = SubTree("inner", tree_reg)

        # Without blackboard
        assert st.tick(bb()) == Status.FAILURE

        # With blackboard
        b = Blackboard()
        b.set("x", "hello")
        assert st.tick(b) == Status.SUCCESS

    def test_parent_chain(self):
        tree_reg = Registry()

        def check_parent(b):
            return b.get_string("parent_val") != ""

        tree_reg.register("inner", Condition(check_parent))

        # Wrap in a Sequence equivalent: tick subtree directly
        st = SubTree("inner", tree_reg)
        b = Blackboard()
        b.set("parent_val", "exists")
        assert st.tick(b) == Status.SUCCESS

    def test_not_found(self):
        st = SubTree("nonexistent", Registry())
        assert st.tick(bb()) == Status.FAILURE

    def test_running_state(self):
        tree_reg = Registry()
        wait_node = Wait(3)
        tree_reg.register("waiter", wait_node)

        st = SubTree("waiter", tree_reg)
        assert st.tick(bb()) == Status.RUNNING
        assert st.tick(bb()) == Status.RUNNING
        assert st.tick(bb()) == Status.SUCCESS

        # After completion, next tick starts fresh
        assert st.tick(bb()) == Status.RUNNING

    def test_halt(self):
        tree_reg = Registry()
        wait_node = Wait(10)
        tree_reg.register("waiter", wait_node)

        st = SubTree("waiter", tree_reg)
        assert st.tick(bb()) == Status.RUNNING
        st.halt()
        assert st.tick(bb()) == Status.RUNNING
