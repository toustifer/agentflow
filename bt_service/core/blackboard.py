"""Thread-safe Blackboard with parent chain for SubTree data passing."""

from __future__ import annotations

import threading
from typing import Any


class Blackboard:
    """Thread-safe key-value store with parent chain traversal.

    Set/Get/Has traverse up the parent chain so SubTree children can
    transparently read parent data.
    """

    def __init__(self):
        self._lock = threading.Lock()
        self._data: dict[str, Any] = {}
        self._parent: Blackboard | None = None

    @classmethod
    def from_json_map(cls, data: dict | None) -> "Blackboard":
        """Build a blackboard from a plain JSON object map."""
        bb = cls()
        for k, v in (data or {}).items():
            bb.set(k, v)
        return bb

    # === write ===

    def set(self, key: str, value) -> None:
        self._validate_json_safe(value)
        with self._lock:
            self._data[key] = value

    # === read (with parent chain) ===

    def get(self, key: str, default=None):
        with self._lock:
            if key in self._data:
                return self._data[key]
        if self._parent is not None:
            return self._parent.get(key, default)
        return default

    def has(self, key: str) -> bool:
        with self._lock:
            if key in self._data:
                return True
        if self._parent is not None:
            return self._parent.has(key)
        return False

    def get_string(self, key: str, default: str = "") -> str:
        v = self.get(key, default)
        if not isinstance(v, str):
            return default
        return v

    def get_bool(self, key: str, default: bool = False) -> bool:
        v = self.get(key, default)
        if not isinstance(v, bool):
            return default
        return v

    # === parent chain ===

    def set_parent(self, parent: "Blackboard | None") -> None:
        with self._lock:
            self._parent = parent

    @property
    def parent(self) -> "Blackboard | None":
        with self._lock:
            return self._parent

    # === serialization ===

    def to_json_map(self) -> dict[str, Any]:
        """Export only the local data as a plain JSON-safe map.

        Parent pointers and locks are intentionally omitted.
        """
        with self._lock:
            return dict(self._data)

    def snapshot(self) -> dict[str, Any]:
        """Alias for to_json_map()."""
        return self.to_json_map()

    # === validation ===

    @classmethod
    def _validate_json_safe(cls, value: Any) -> None:
        if value is None:
            return
        if isinstance(value, (str, int, float, bool)):
            return
        if isinstance(value, list):
            for item in value:
                cls._validate_json_safe(item)
            return
        if isinstance(value, dict):
            for k, v in value.items():
                if not isinstance(k, str):
                    raise TypeError(f"blackboard keys must be strings, got {type(k).__name__}")
                cls._validate_json_safe(v)
            return
        raise TypeError(f"blackboard value is not JSON-safe: {type(value).__name__}")
