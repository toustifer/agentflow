"""FactoryRegistry — maps type names to node constructors.

Mirrors the Go pkg/bt/factory.go design: register named node types,
then build a node tree from a NodeConfig dict recursively.
"""

from __future__ import annotations
from typing import Any, Callable

from bt_service.core.node import Node, Status
from bt_service.core.control import Sequence, Fallback, ReactiveSequence, ReactiveFallback
from bt_service.core.leaf import Condition, Action, Inverter, Retry, Wait, Log
from bt_service.core.subtree import SubTree
from bt_service.core.registry import Registry
from bt_service.core.blackboard import Blackboard

# Factory: (cfg dict, children list, registry) -> Node
NodeFactory = Callable[[dict, list[Node], "FactoryRegistry"], Node]
CondFn = Callable[[Blackboard], bool]
ActFn = Callable[[Blackboard], tuple[bool, Exception | None]]


class FactoryRegistry:
    """Maps type names to constructors, plus named condition/action functions."""

    def __init__(self):
        self._factories: dict[str, NodeFactory] = {}
        self._cond_fns: dict[str, CondFn] = {}
        self._act_fns: dict[str, ActFn] = {}
        self._tree_registry: Registry | None = None

    # === registration ===

    def register(self, type_name: str, factory: NodeFactory) -> None:
        self._factories[type_name] = factory

    def register_condition(self, name: str, fn: CondFn) -> None:
        self._cond_fns[name] = fn

    def register_action(self, name: str, fn: ActFn) -> None:
        self._act_fns[name] = fn

    def set_tree_registry(self, tr: Registry) -> None:
        self._tree_registry = tr

    # === build ===

    def build(self, cfg: dict) -> Node:
        type_name = cfg.get("type", "")
        factory = self._factories.get(type_name)
        if factory is None:
            raise ValueError(f"unknown node type: {type_name!r}")

        children = []
        for i, child_raw in enumerate(cfg.get("children", [])):
            child = self.build(child_raw)
            children.append(child)

        return factory(cfg, children, self)

    def list_types(self) -> list[str]:
        return list(self._factories.keys())


# ====================================================================
# Built-in node type factories
# ====================================================================


def _sequence_factory(cfg: dict, children: list[Node], reg: FactoryRegistry) -> Node:
    return Sequence(children)


def _fallback_factory(cfg: dict, children: list[Node], reg: FactoryRegistry) -> Node:
    return Fallback(children)


def _reactive_sequence_factory(cfg: dict, children: list[Node], reg: FactoryRegistry) -> Node:
    return ReactiveSequence(children)


def _reactive_fallback_factory(cfg: dict, children: list[Node], reg: FactoryRegistry) -> Node:
    return ReactiveFallback(children)


def _inverter_factory(cfg: dict, children: list[Node], reg: FactoryRegistry) -> Node:
    if len(children) != 1:
        raise ValueError(f"Inverter requires exactly 1 child, got {len(children)}")
    return Inverter(children[0])


def _retry_factory(cfg: dict, children: list[Node], reg: FactoryRegistry) -> Node:
    if len(children) != 1:
        raise ValueError(f"Retry requires exactly 1 child, got {len(children)}")
    max_retry = int(cfg.get("properties", {}).get("max_retry", 3))
    return Retry(max_retry, children[0])


def _condition_factory(cfg: dict, children: list[Node], reg: FactoryRegistry) -> Node:
    fn_name = cfg.get("properties", {}).get("fn", "")
    if not fn_name:
        raise ValueError("Condition requires 'fn' property")
    fn = reg._cond_fns.get(fn_name)
    if fn is None:
        raise ValueError(f"unknown condition function: {fn_name!r}")
    return Condition(fn)


def _action_factory(cfg: dict, children: list[Node], reg: FactoryRegistry) -> Node:
    fn_name = cfg.get("properties", {}).get("fn", "")
    if not fn_name:
        raise ValueError("Action requires 'fn' property")
    fn = reg._act_fns.get(fn_name)
    if fn is None:
        raise ValueError(f"unknown action function: {fn_name!r}")
    return Action(fn)


def _subtree_factory(cfg: dict, children: list[Node], reg: FactoryRegistry) -> Node:
    tree_name = cfg.get("properties", {}).get("tree", "")
    if not tree_name:
        raise ValueError("SubTree requires 'tree' property")
    tr = reg._tree_registry
    if tr is None:
        raise ValueError("SubTree: no tree registry set on FactoryRegistry")
    return SubTree(tree_name, tr)


def _wait_factory(cfg: dict, children: list[Node], reg: FactoryRegistry) -> Node:
    ticks = int(cfg.get("properties", {}).get("ticks", 1))
    return Wait(ticks)


def _log_factory(cfg: dict, children: list[Node], reg: FactoryRegistry) -> Node:
    if len(children) != 1:
        raise ValueError(f"Log requires exactly 1 child, got {len(children)}")
    message = cfg.get("properties", {}).get("message", "")
    return Log(message, children[0])


def register_default_nodes(reg: FactoryRegistry) -> None:
    """Register all built-in node types into the registry."""
    reg.register("Sequence", _sequence_factory)
    reg.register("Fallback", _fallback_factory)
    reg.register("ReactiveSequence", _reactive_sequence_factory)
    reg.register("ReactiveFallback", _reactive_fallback_factory)
    reg.register("Inverter", _inverter_factory)
    reg.register("Retry", _retry_factory)
    reg.register("Condition", _condition_factory)
    reg.register("Action", _action_factory)
    reg.register("SubTree", _subtree_factory)
    reg.register("Wait", _wait_factory)
    reg.register("Log", _log_factory)
