"""Node interface, Status enum, and Haltable ABC for the BehaviorTree runtime."""
from __future__ import annotations

from abc import ABC, abstractmethod
from enum import IntEnum


class Status(IntEnum):
    """Tick return status matching BehaviorTree.CPP convention."""

    SUCCESS = 0
    FAILURE = 1
    RUNNING = 2


class Node(ABC):
    """Core interface. Every node must implement tick()."""

    @abstractmethod
    def tick(self, bb: "Blackboard") -> Status:
        ...


class Haltable(ABC):
    """Implemented by nodes that need cleanup when interrupted."""

    @abstractmethod
    def halt(self) -> None:
        ...


class NodeFunc(Node):
    """Wraps a plain function as a Node."""

    def __init__(self, fn):
        self._fn = fn

    def tick(self, bb: "Blackboard") -> Status:
        return self._fn(bb)


def append_trace(bb: "Blackboard", name: str, node_type: str, status: Status) -> None:
    """Record one executed node into ``bb._trace`` when tracing is enabled.

    The trace list lives as a plain attribute on the Blackboard (never in its
    JSON data map), so enabling tracing never leaks into ``outputs`` or
    ``blackboard`` payloads. ``status.name.lower()`` yields success/failure/running.
    """
    trace = getattr(bb, "_trace", None)
    if trace is not None:
        trace.append({"name": name, "type": node_type, "status": status.name.lower()})


def child_label(child: Node, idx: int) -> str:
    """Human label for one child: leaf fn name, else ``Type#idx``."""
    fn = getattr(child, "_fn", None)
    if fn is not None and getattr(fn, "__name__", None):
        return getattr(fn, "__name__")
    return f"{type(child).__name__}#{idx}"


def is_control_node(node: Node) -> bool:
    """True for the composite control nodes (Sequence/Fallback family).

    Leaves and decorators record themselves; control loops only record
    control children, so each executed node appears exactly once in traces.
    """
    return bool(getattr(type(node), "_is_control", False))
