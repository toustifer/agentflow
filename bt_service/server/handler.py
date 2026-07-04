"""MCP-style handler for BT RPC methods."""

from __future__ import annotations

import json
import os
from typing import Any

from bt_service.core.blackboard import Blackboard
from bt_service.core.registry import Registry
from bt_service.factory.registry import FactoryRegistry, register_default_nodes
from bt_service.factory.deserialize import deserialize_tree
from bt_service.loader.template import load_tree_dir
from bt_service.loader.nodes import discover_nodes
from bt_service.server.builtin import register_builtin_nodes

DEFAULT_TREES_DIR = os.path.join(os.path.dirname(__file__), "..", "..", "trees")


class BTServer:
    """Holds registries and tree state, handles RPC methods."""

    def __init__(self, trees_dir: str = ""):
        self.trees_dir = trees_dir or DEFAULT_TREES_DIR

        self.factory_reg = FactoryRegistry()
        register_default_nodes(self.factory_reg)
        register_builtin_nodes(self.factory_reg)

        self.tree_reg = Registry()
        self.factory_reg.set_tree_registry(self.tree_reg)

        self._tree_sources: dict[str, str] = {}

        sources = load_tree_dir(self.trees_dir, self.factory_reg, self.tree_reg)
        for s in sources:
            self._tree_sources[s["name"]] = s["source"]

        nodes_dir = os.path.join(self.trees_dir, "nodes")
        self._discovered = discover_nodes(nodes_dir, self.factory_reg)

    def handle(self, method: str, params: dict) -> dict:
        handler = getattr(self, f"do_{method}", None)
        if handler is None:
            raise ValueError(f"unknown method: {method}")
        return handler(params)

    # ===== RPC handlers =====

    def do_ping(self, params: dict) -> dict:
        return {
            "ok": True,
            "service": "bt_service",
            "version": "dev",
            "capabilities": {
                "trees": True,
                "validate": True,
                "tick": True,
                "phase_provider": True,
            },
        }

    def do_list_trees(self, params: dict) -> dict:
        names = sorted(self.tree_reg.list())
        return {
            "trees": [{"name": name, "source": "runtime", "loaded": True} for name in names],
            "count": len(names),
        }

    def do_show_tree(self, params: dict) -> dict:
        name = params.get("tree_name") or params.get("tree") or ""
        source = self._tree_sources.get(name)
        if source is None:
            raise ValueError(f"tree {name!r} not found")
        parsed = json.loads(source)
        pretty = json.dumps(parsed, indent=2, ensure_ascii=False)
        return {"tree_name": name, "tree_json": parsed, "source_pretty": pretty}

    def do_validate(self, params: dict) -> dict:
        tree_name = params.get("tree_name", "")
        tree_json = params.get("tree_json")

        if tree_name:
            source = self._tree_sources.get(tree_name)
            if source is None:
                return {"valid": False, "tree_name": tree_name, "errors": [{"path": "$", "code": "not_found", "message": f"tree {tree_name!r} not found"}]}
            tree_json = json.loads(source)

        if tree_json is None:
            raise ValueError("tree_name or tree_json is required")

        try:
            deserialize_tree(json.dumps(tree_json), self.factory_reg)
            return {"valid": True, "tree_name": tree_name or None, "errors": []}
        except (ValueError, KeyError, TypeError) as e:
            return {
                "valid": False,
                "tree_name": tree_name or None,
                "errors": [{"path": "$.tree", "code": "validation_error", "message": str(e)}],
            }

    def do_tick(self, params: dict) -> dict:
        """Tick a tree by name or inline JSON.

        Params:
            tree_name?: str
            tree_json?: dict
            blackboard?: dict
            options?: { return_blackboard?: bool }
        """
        tree_name = params.get("tree_name", "")
        tree_json = params.get("tree_json")
        bb_data = params.get("blackboard", {}) or {}
        options = params.get("options", {}) or {}
        return_blackboard = bool(options.get("return_blackboard", True))

        if tree_json is not None:
            node, _ = deserialize_tree(json.dumps(tree_json), self.factory_reg)
        elif tree_name:
            node = self.tree_reg.get(tree_name)
            if node is None:
                raise ValueError(f"tree {tree_name!r} not found")
        else:
            raise ValueError("either tree_name or tree_json is required")

        bb = Blackboard.from_json_map(bb_data)
        status = node.tick(bb)

        outputs = self._extract_outputs(bb)
        result = {
            "status": status.name.lower(),
            "tree_name": tree_name or None,
            "outputs": outputs,
        }
        if return_blackboard:
            result["blackboard"] = bb.to_json_map()
        return result

    def do_list_nodes(self, params: dict) -> dict:
        return {
            "node_types": self.factory_reg.list_types(),
            "conditions": sorted(list(self.factory_reg._cond_fns.keys())),
            "actions": sorted(list(self.factory_reg._act_fns.keys())),
        }

    def do_load_tree(self, params: dict) -> dict:
        tree_name = params.get("tree_name", "")
        tree_json = params.get("tree_json", {})
        if not tree_name or not tree_json:
            raise ValueError("tree_name and tree_json are required")

        node, _ = deserialize_tree(json.dumps(tree_json), self.factory_reg)
        self.tree_reg.register(tree_name, node)
        self._tree_sources[tree_name] = json.dumps(tree_json, ensure_ascii=False)
        return {"ok": True, "tree_name": tree_name}

    def do_discover(self, params: dict) -> dict:
        nodes_dir = params.get("dir", os.path.join(self.trees_dir, "nodes"))
        registered = discover_nodes(nodes_dir, self.factory_reg)
        return {"registered": registered}

    @staticmethod
    def _extract_outputs(bb: Blackboard) -> dict:
        out = {}
        for key in [
            "phase",
            "phase_name",
            "progress",
            "actions",
            "next_tasks",
            "active_tasks",
            "stuck_tasks",
        ]:
            if bb.has(key):
                out[key] = bb.get(key)
        return out
