"""Named tree registry for SubTree lookups."""

from bt_service.core.node import Node, Status


class Registry:
    """Holds named behavior trees for SubTree resolution and top-level ticks."""

    def __init__(self):
        self._trees: dict[str, Node] = {}

    def register(self, name: str, root: Node) -> None:
        self._trees[name] = root

    def get(self, name: str) -> Node | None:
        return self._trees.get(name)

    def tick(self, name: str, bb) -> tuple[Status, str]:
        tree = self._trees.get(name)
        if tree is None:
            return Status.FAILURE, f"tree {name!r} not found"
        try:
            status = tree.tick(bb)
            return status, ""
        except Exception as e:
            return Status.FAILURE, str(e)

    def list(self) -> list[str]:
        return list(self._trees.keys())
