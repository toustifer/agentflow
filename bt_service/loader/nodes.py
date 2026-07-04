"""Node auto-discovery — scans trees/nodes/*.py for user-defined nodes.

Each .py file should export a NODES list:
    NODES = [
        {"type": "condition", "name": "my_cond", "fn": callable},
        {"type": "action", "name": "my_action", "fn": callable},
    ]
"""

import importlib
import importlib.util
import os
import sys

from bt_service.factory.registry import FactoryRegistry
from bt_service.core.blackboard import Blackboard


def discover_nodes(nodes_dir: str, registry: FactoryRegistry) -> list[str]:
    """Scan nodes_dir for .py files, import each, register NODES entries.

    Returns list of registered node names.
    """
    registered = []
    if not os.path.isdir(nodes_dir):
        return registered

    sys.path.insert(0, nodes_dir)
    for entry in sorted(os.listdir(nodes_dir)):
        if not entry.endswith(".py") or entry == "__init__.py":
            continue
        module_name = entry[:-3]
        filepath = os.path.join(nodes_dir, entry)

        try:
            spec = importlib.util.spec_from_file_location(module_name, filepath)
            if spec is None or spec.loader is None:
                continue
            module = importlib.util.module_from_spec(spec)
            spec.loader.exec_module(module)

            if not hasattr(module, "NODES"):
                continue

            for node_def in module.NODES:
                _register_one(node_def, registry, module_name)
                registered.append(f"{module_name}.{node_def['name']}")
        except Exception:
            continue

    sys.path.pop(0)
    return registered


def _register_one(node_def: dict, registry: FactoryRegistry, module_name: str) -> None:
    node_type = node_def.get("type")
    name = node_def.get("name")
    fn = node_def.get("fn")

    if not name or not fn:
        return

    if node_type == "condition":
        registry.register_condition(name, fn)
    elif node_type == "action":
        registry.register_action(name, fn)
