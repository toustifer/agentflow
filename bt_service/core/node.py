"""Node interface, Status enum, and Haltable ABC for the BehaviorTree runtime."""

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
