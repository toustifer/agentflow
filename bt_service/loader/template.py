"""Tree template loader — scans directories for .json tree files."""

import json
import os

from bt_service.core.registry import Registry
from bt_service.factory.registry import FactoryRegistry
from bt_service.factory.deserialize import deserialize_tree


def load_tree_dir(dir_path: str, reg: FactoryRegistry, tree_reg: Registry) -> list[dict]:
    """Scan dir_path for .json files, deserialize each, register in tree_reg.

    Returns list of {name, source} for inspection tools.
    """
    sources = []
    if not os.path.isdir(dir_path):
        return sources

    for entry in sorted(os.listdir(dir_path)):
        if not entry.endswith(".json"):
            continue
        filepath = os.path.join(dir_path, entry)
        with open(filepath, "r", encoding="utf-8") as f:
            data = f.read()

        node, _ = deserialize_tree(data, reg)
        name = entry[:-5]  # strip .json
        tree_reg.register(name, node)
        sources.append({"name": name, "source": data})
    return sources


def validate_tree_json(data: str, reg: FactoryRegistry) -> tuple[bool, str]:
    """Validate JSON tree without registering. Returns (valid, error_msg)."""
    try:
        json.loads(data)  # ensure valid JSON
    except json.JSONDecodeError as e:
        return False, str(e)
    try:
        deserialize_tree(data, reg)
        return True, ""
    except (ValueError, KeyError, TypeError) as e:
        return False, str(e)
