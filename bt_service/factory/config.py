"""NodeConfig and TreeFile dataclass-style tuples."""

from __future__ import annotations
from typing import Any


class NodeConfig:
    """JSON-deserialized node configuration.

    Fields map directly to the JSON keys used in tree definitions.
    """

    def __init__(
        self,
        type_name: str,
        name: str = "",
        children: list[dict] | None = None,
        properties: dict[str, Any] | None = None,
        blackboard: dict[str, Any] | None = None,
    ):
        self.type = type_name
        self.name = name
        self.children = children or []
        self.properties = properties or {}
        self.blackboard = blackboard or {}

    @classmethod
    def from_dict(cls, d: dict) -> "NodeConfig":
        return cls(
            type_name=d.get("type", ""),
            name=d.get("name", ""),
            children=d.get("children"),
            properties=d.get("properties"),
            blackboard=d.get("blackboard"),
        )


class TreeFile:
    """Top-level tree file format."""

    def __init__(
        self,
        name: str = "",
        blackboard: dict[str, Any] | None = None,
        tree: dict | None = None,
    ):
        self.name = name
        self.blackboard = blackboard or {}
        self.tree = tree

    @classmethod
    def from_dict(cls, d: dict) -> "TreeFile":
        return cls(
            name=d.get("name", ""),
            blackboard=d.get("blackboard"),
            tree=d.get("tree"),
        )
