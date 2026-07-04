"""Tests for Blackboard."""

import threading
from bt_service.core.blackboard import Blackboard


class TestBlackboard:
    def test_set_get(self):
        bb = Blackboard()
        bb.set("key1", "value1")
        bb.set("key2", 42)
        bb.set("key3", True)

        assert bb.get("key1") == "value1"
        assert bb.get("key2") == 42
        assert bb.get("key3") is True
        assert bb.get("nope") is None

    def test_has(self):
        bb = Blackboard()
        bb.set("a", 1)
        assert bb.has("a") is True
        assert bb.has("b") is False

    def test_get_string(self):
        bb = Blackboard()
        bb.set("s", "hello")
        bb.set("n", 42)
        assert bb.get_string("s") == "hello"
        assert bb.get_string("n") == ""
        assert bb.get_string("missing") == ""

    def test_get_bool(self):
        bb = Blackboard()
        bb.set("t", True)
        bb.set("f", False)
        bb.set("s", "yes")
        assert bb.get_bool("t") is True
        assert bb.get_bool("f") is False
        assert bb.get_bool("s") is False
        assert bb.get_bool("missing") is False

    def test_parent_chain(self):
        parent = Blackboard()
        parent.set("from_parent", "pval")
        parent.set("override", "original")

        child = Blackboard()
        child.set("from_child", "cval")
        child.set("override", "child_val")
        child.set_parent(parent)

        assert child.get("from_parent") == "pval"
        assert child.get("override") == "child_val"
        assert child.get("from_child") == "cval"
        assert parent.get("from_child") is None

    def test_deep_parent_chain(self):
        root = Blackboard()
        root.set("a", "root_a")

        mid = Blackboard()
        mid.set("b", "mid_b")
        mid.set_parent(root)

        leaf = Blackboard()
        leaf.set("c", "leaf_c")
        leaf.set_parent(mid)

        assert leaf.get("a") == "root_a"
        assert leaf.get("b") == "mid_b"
        assert leaf.get("c") == "leaf_c"
        assert leaf.has("a") is True
        assert leaf.has("b") is True
        assert mid.has("c") is False

    def test_concurrent_safe(self):
        bb = Blackboard()
        errors = []

        def worker():
            try:
                for _ in range(100):
                    bb.set("k", 1)
                    bb.get("k")
                    bb.has("k")
            except Exception as e:
                errors.append(e)

        threads = [threading.Thread(target=worker) for _ in range(10)]
        for t in threads:
            t.start()
        for t in threads:
            t.join()
        assert len(errors) == 0

    def test_set_parent_nil(self):
        child = Blackboard()
        child.set("x", 1)
        child.set_parent(None)
        assert child.get("x") == 1
        assert child.get("missing") is None

    def test_parent_property(self):
        parent = Blackboard()
        child = Blackboard()
        child.set_parent(parent)
        assert child.parent is parent
        assert parent.parent is None
