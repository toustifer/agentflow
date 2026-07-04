"""Control flow nodes: Sequence, Fallback, ReactiveSequence, ReactiveFallback."""

from bt_service.core.node import Node, Haltable, Status


class Sequence(Node, Haltable):
    """Ticks children in order. Remembers position across ticks.

    FAILURE or RUNNING → return immediately, remember position.
    All children SUCCESS → SUCCESS.
    """

    def __init__(self, children=None):
        self.children = children or []
        self._index = 0

    def tick(self, bb) -> Status:
        while self._index < len(self.children):
            status = self.children[self._index].tick(bb)
            if status == Status.RUNNING:
                return Status.RUNNING
            elif status == Status.FAILURE:
                self._index = 0
                return Status.FAILURE
            elif status == Status.SUCCESS:
                self._index += 1
        self._index = 0
        return Status.SUCCESS

    def halt(self) -> None:
        if self._index < len(self.children):
            child = self.children[self._index]
            if isinstance(child, Haltable):
                child.halt()
        self._index = 0


class Fallback(Node, Haltable):
    """Tries each child until one succeeds. Remembers position.

    SUCCESS or RUNNING → return immediately, remember position.
    All children FAILURE → FAILURE.
    """

    def __init__(self, children=None):
        self.children = children or []
        self._index = 0

    def tick(self, bb) -> Status:
        while self._index < len(self.children):
            status = self.children[self._index].tick(bb)
            if status == Status.RUNNING:
                return Status.RUNNING
            elif status == Status.SUCCESS:
                self._index = 0
                return Status.SUCCESS
            elif status == Status.FAILURE:
                self._index += 1
        self._index = 0
        return Status.FAILURE

    def halt(self) -> None:
        if self._index < len(self.children):
            child = self.children[self._index]
            if isinstance(child, Haltable):
                child.halt()
        self._index = 0


class ReactiveSequence(Node, Haltable):
    """Ticks ALL children every tick. Resets non-RUNNING children each tick.

    First FAILURE → return FAILURE.
    All children SUCCESS → SUCCESS.
    """

    def __init__(self, children=None):
        self.children = children or []

    def tick(self, bb) -> Status:
        any_running = False
        for child in self.children:
            status = child.tick(bb)
            if status == Status.RUNNING:
                any_running = True
            elif status == Status.FAILURE:
                return Status.FAILURE
        return Status.RUNNING if any_running else Status.SUCCESS

    def halt(self) -> None:
        for child in self.children:
            if isinstance(child, Haltable):
                child.halt()


class ReactiveFallback(Node, Haltable):
    """Ticks ALL children every tick. First SUCCESS returns SUCCESS.

    All children FAILURE → FAILURE.
    """

    def __init__(self, children=None):
        self.children = children or []

    def tick(self, bb) -> Status:
        any_running = False
        all_failed = True
        for child in self.children:
            status = child.tick(bb)
            if status == Status.RUNNING:
                any_running = True
                all_failed = False
            elif status == Status.SUCCESS:
                return Status.SUCCESS
            elif status == Status.FAILURE:
                pass
        if any_running:
            return Status.RUNNING
        return Status.FAILURE

    def halt(self) -> None:
        for child in self.children:
            if isinstance(child, Haltable):
                child.halt()
