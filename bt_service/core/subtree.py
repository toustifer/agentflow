"""SubTree — embeds another registered tree with child blackboard."""
from __future__ import annotations

from bt_service.core.node import Node, Haltable, Status, append_trace
from bt_service.core.blackboard import Blackboard
from bt_service.core.registry import Registry


class SubTree(Node, Haltable):
    """Embeds a registered tree.

    A child blackboard linked to the parent is created on first tick.
    It persists while the subtree is RUNNING and resets on completion.
    The parent chain allows transparent read access to parent data.
    """

    def __init__(self, tree_name: str, registry: Registry):
        self._tree_name = tree_name
        self._registry = registry
        self._child_bb: Blackboard | None = None

    def tick(self, bb: Blackboard) -> Status:
        if self._child_bb is None:
            self._child_bb = Blackboard()
        self._child_bb.set_parent(bb)

        # Thread the active trace list into the child blackboard so inner
        # nodes land in the same traces array as the outer tree.
        parent_trace = getattr(bb, "_trace", None)
        if parent_trace is not None:
            self._child_bb._trace = parent_trace

        tree = self._registry.get(self._tree_name)
        if tree is None:
            append_trace(bb, f"subtree:{self._tree_name}", "subtree", Status.FAILURE)
            return Status.FAILURE

        status = tree.tick(self._child_bb)
        append_trace(bb, f"subtree:{self._tree_name}", "subtree", status)
        if status != Status.RUNNING:
            self._child_bb = None
        return status

    def halt(self) -> None:
        self._child_bb = None
