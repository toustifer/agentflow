"""Leaf nodes: Condition, Action, ActionWithRunning, Inverter, Retry, Wait, Log."""
from __future__ import annotations

from bt_service.core.node import Node, Haltable, Status, append_trace


class Condition(Node, Haltable):
    """Pure check. Returns SUCCESS or FAILURE, never RUNNING."""

    def __init__(self, fn):
        self._fn = fn

    def tick(self, bb) -> Status:
        status = Status.SUCCESS if self._fn(bb) else Status.FAILURE
        append_trace(bb, getattr(self, "_trace_name", None) or getattr(self._fn, "__name__", "condition"), "condition", status)
        return status

    def halt(self) -> None:
        pass


class Action(Node, Haltable):
    """Side effect returning SUCCESS/FAILURE.

    fn(bb) -> (bool success, Exception | None)
    """

    def __init__(self, fn):
        self._fn = fn
        self._error = None

    def _trace_name_of(self) -> str:
        return getattr(self, "_trace_name", None) or getattr(self._fn, "__name__", "action")

    def tick(self, bb) -> Status:
        try:
            ok, err = self._fn(bb)
            self._error = err
            if err:
                append_trace(bb, self._trace_name_of(), "action", Status.FAILURE)
                return Status.FAILURE
            status = Status.SUCCESS if ok else Status.FAILURE
            append_trace(bb, self._trace_name_of(), "action", status)
            return status
        except Exception as e:
            self._error = e
            append_trace(bb, self._trace_name_of(), "action", Status.FAILURE)
            return Status.FAILURE

    @property
    def error(self):
        return self._error

    def halt(self) -> None:
        pass


class ActionWithRunning(Node, Haltable):
    """Side effect that can return RUNNING.

    fn(bb) -> (Status, Exception | None)
    """

    def __init__(self, fn):
        self._fn = fn
        self._error = None

    def tick(self, bb) -> Status:
        name = getattr(self, "_trace_name", None) or getattr(self._fn, "__name__", "action")
        try:
            status, err = self._fn(bb)
            self._error = err
            if err:
                append_trace(bb, name, "action", Status.FAILURE)
                return Status.FAILURE
            append_trace(bb, name, "action", status)
            return status
        except Exception as e:
            self._error = e
            append_trace(bb, name, "action", Status.FAILURE)
            return Status.FAILURE

    @property
    def error(self):
        return self._error

    def halt(self) -> None:
        pass


class Inverter(Node, Haltable):
    """Inverts SUCCESS <-> FAILURE. RUNNING passes through."""

    def __init__(self, child: Node):
        self._child = child

    def tick(self, bb) -> Status:
        status = self._child.tick(bb)
        append_trace(bb, "Inverter", "decorator", status)
        if status == Status.SUCCESS:
            return Status.FAILURE
        elif status == Status.FAILURE:
            return Status.SUCCESS
        return status

    def halt(self) -> None:
        if isinstance(self._child, Haltable):
            self._child.halt()


class Retry(Node, Haltable):
    """Retries child N times until SUCCESS."""

    def __init__(self, max_retry: int, child: Node):
        self._child = child
        self._max_retry = max_retry
        self._attempts = 0

    def tick(self, bb) -> Status:
        while self._attempts < self._max_retry:
            status = self._child.tick(bb)
            append_trace(bb, f"Retry:{self._max_retry}", "decorator", status)
            if status == Status.RUNNING:
                return Status.RUNNING
            elif status == Status.SUCCESS:
                self._attempts = 0
                return Status.SUCCESS
            elif status == Status.FAILURE:
                self._attempts += 1
        self._attempts = 0
        return Status.FAILURE

    def halt(self) -> None:
        if isinstance(self._child, Haltable):
            self._child.halt()
        self._attempts = 0


class Wait(Node, Haltable):
    """Returns RUNNING for N ticks, then SUCCESS."""

    def __init__(self, ticks: int = 1):
        self._max_ticks = ticks
        self._count = 0

    def tick(self, bb) -> Status:
        self._count += 1
        status = Status.SUCCESS if self._count >= self._max_ticks else Status.RUNNING
        if status == Status.SUCCESS:
            self._count = 0
        append_trace(bb, "Wait", "wait", status)
        return status

    def halt(self) -> None:
        self._count = 0


class Log(Node, Haltable):
    """Decorator that logs a message on each tick (passthrough)."""

    def __init__(self, message: str, child: Node):
        self._message = message
        self._child = child

    def tick(self, bb) -> Status:
        status = self._child.tick(bb)
        append_trace(bb, "Log", "decorator", status)
        return status

    def halt(self) -> None:
        if isinstance(self._child, Haltable):
            self._child.halt()
