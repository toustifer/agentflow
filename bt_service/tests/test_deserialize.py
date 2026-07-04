"""Tests for deserialization — JSON → Node tree."""

import json
from bt_service.core.node import Status
from bt_service.core.blackboard import Blackboard
from bt_service.factory.registry import FactoryRegistry, register_default_nodes
from bt_service.factory.deserialize import deserialize_tree, deserialize_node


def make_reg():
    reg = FactoryRegistry()
    register_default_nodes(reg)
    reg.register_condition("yes", lambda b: True)
    reg.register_condition("no", lambda b: False)
    return reg


def bb():
    return Blackboard()


class TestDeserializeSequence:
    def test_all_success(self):
        reg = make_reg()
        node, _ = deserialize_tree(json.dumps({
            "tree": {
                "type": "Sequence",
                "children": [
                    {"type": "Condition", "properties": {"fn": "yes"}},
                    {"type": "Condition", "properties": {"fn": "yes"}},
                ],
            }
        }), reg)
        assert node.tick(bb()) == Status.SUCCESS

    def test_failure(self):
        reg = make_reg()
        node, _ = deserialize_tree(json.dumps({
            "tree": {
                "type": "Sequence",
                "children": [
                    {"type": "Condition", "properties": {"fn": "yes"}},
                    {"type": "Condition", "properties": {"fn": "no"}},
                ],
            }
        }), reg)
        assert node.tick(bb()) == Status.FAILURE


class TestDeserializeFallback:
    def test_success(self):
        reg = make_reg()
        node, _ = deserialize_tree(json.dumps({
            "tree": {
                "type": "Fallback",
                "children": [
                    {"type": "Condition", "properties": {"fn": "no"}},
                    {"type": "Condition", "properties": {"fn": "yes"}},
                ],
            }
        }), reg)
        assert node.tick(bb()) == Status.SUCCESS


class TestDeserializeInverter:
    def test_invert(self):
        reg = make_reg()
        node, _ = deserialize_tree(json.dumps({
            "tree": {
                "type": "Inverter",
                "children": [
                    {"type": "Condition", "properties": {"fn": "no"}},
                ],
            }
        }), reg)
        assert node.tick(bb()) == Status.SUCCESS


class TestDeserializeReactive:
    def test_sequence(self):
        reg = make_reg()
        node, _ = deserialize_tree(json.dumps({
            "tree": {
                "type": "ReactiveSequence",
                "children": [
                    {"type": "Condition", "properties": {"fn": "yes"}},
                    {"type": "Condition", "properties": {"fn": "yes"}},
                ],
            }
        }), reg)
        assert node.tick(bb()) == Status.SUCCESS

    def test_fallback(self):
        reg = make_reg()
        node, _ = deserialize_tree(json.dumps({
            "tree": {
                "type": "ReactiveFallback",
                "children": [
                    {"type": "Condition", "properties": {"fn": "no"}},
                    {"type": "Condition", "properties": {"fn": "yes"}},
                ],
            }
        }), reg)
        assert node.tick(bb()) == Status.SUCCESS


class TestDeserializeRetry:
    def test_exhausted(self):
        reg = make_reg()
        node, _ = deserialize_tree(json.dumps({
            "tree": {
                "type": "Retry",
                "properties": {"max_retry": 2},
                "children": [
                    {"type": "Condition", "properties": {"fn": "no"}},
                ],
            }
        }), reg)
        assert node.tick(bb()) == Status.FAILURE


class TestDeserializeAction:
    def test_action(self):
        reg = make_reg()
        calls = [0]
        reg.register_action("increment", lambda b: (calls.__setitem__(0, calls[0] + 1) or True, None))
        node, _ = deserialize_tree(json.dumps({
            "tree": {"type": "Action", "properties": {"fn": "increment"}}
        }), reg)
        assert node.tick(bb()) == Status.SUCCESS
        assert calls[0] == 1

    def test_action_reads_blackboard(self):
        reg = make_reg()
        captured = [""]
        reg.register_action("read_bb", lambda b: (captured.__setitem__(0, b.get_string("val")) or True, None))
        node, _ = deserialize_tree(json.dumps({
            "tree": {"type": "Action", "properties": {"fn": "read_bb"}}
        }), reg)

        b = Blackboard()
        b.set("val", "from-bb")
        assert node.tick(b) == Status.SUCCESS
        assert captured[0] == "from-bb"


class TestDeserializeWithBlackboard:
    def test_blackboard_data(self):
        reg = FactoryRegistry()
        register_default_nodes(reg)
        reg.register_condition("has_ns", lambda b: b.get_string("nsID") != "")

        node, bb_data = deserialize_tree(json.dumps({
            "name": "test-tree",
            "blackboard": {"nsID": "ns-123"},
            "tree": {"type": "Condition", "properties": {"fn": "has_ns"}},
        }), reg)
        assert bb_data == {"nsID": "ns-123"}

        b = Blackboard()
        for k, v in (bb_data or {}).items():
            b.set(k, v)
        assert node.tick(b) == Status.SUCCESS


class TestDeserializeErrors:
    def test_unknown_condition_fn(self):
        reg = make_reg()
        try:
            deserialize_tree(json.dumps({
                "tree": {"type": "Condition", "properties": {"fn": "does_not_exist"}}
            }), reg)
            assert False, "should have raised"
        except ValueError as e:
            assert "does_not_exist" in str(e)

    def test_invalid_json(self):
        reg = make_reg()
        try:
            deserialize_tree("{invalid", reg)
            assert False, "should have raised"
        except (json.JSONDecodeError, ValueError):
            pass

    def test_unknown_type(self):
        reg = make_reg()
        try:
            deserialize_tree(json.dumps({
                "tree": {"type": "NonExistent"}
            }), reg)
            assert False, "should have raised"
        except ValueError as e:
            assert "NonExistent" in str(e)


class TestDeserializeNodeDirect:
    def test_direct(self):
        reg = make_reg()
        node = deserialize_node(json.dumps({
            "type": "Condition", "properties": {"fn": "yes"}
        }), reg)
        assert node.tick(bb()) == Status.SUCCESS
