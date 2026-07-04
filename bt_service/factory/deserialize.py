"""JSON deserialization — parses tree JSON into a Node tree.

Mirrors Go pkg/bt/deserialize.go.
"""

import json
from typing import Any

from bt_service.core.node import Node, Status
from bt_service.factory.registry import FactoryRegistry
from bt_service.factory.config import NodeConfig, TreeFile


def deserialize_tree(data: str | bytes, reg: FactoryRegistry) -> tuple[Node, dict[str, Any] | None]:
    """Parse a TreeFile JSON string and build the node tree.

    Returns (root Node, initial_blackboard_data).
    The caller should create a Blackboard from initial data before ticking.
    """
    if isinstance(data, bytes):
        data = data.decode("utf-8")

    d = json.loads(data)
    tf = TreeFile.from_dict(d)

    root_cfg = tf.tree or {}
    if not root_cfg:
        raise ValueError("tree file has no 'tree' key")

    # Merge top-level blackboard into root config
    if tf.blackboard:
        root_bb = root_cfg.get("blackboard") or {}
        merged = dict(tf.blackboard)
        merged.update(root_bb)  # root keys override top-level
        root_cfg["blackboard"] = merged

    node = reg.build(root_cfg)
    return node, tf.blackboard


def deserialize_node(data: str | bytes, reg: FactoryRegistry) -> Node:
    """Parse a single NodeConfig JSON string and build the node."""
    if isinstance(data, bytes):
        data = data.decode("utf-8")
    cfg = json.loads(data)
    return reg.build(cfg)
