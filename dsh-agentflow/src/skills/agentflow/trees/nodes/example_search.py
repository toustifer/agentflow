"""Example custom node — demonstrates how to add custom conditions/actions.

Place this file in trees/nodes/ and the BT service will auto-discover it.
"""

from bt_service.core.blackboard import Blackboard


def search_web(bb: Blackboard) -> tuple:
    """Action: search the web and store results in blackboard."""
    query = bb.get_string("query")
    results = f"mock results for: {query}"
    bb.set("search_results", results)
    return (True, None)


def has_results(bb: Blackboard) -> bool:
    """Condition: check if search results exist in blackboard."""
    return bb.has("search_results")


# Convention: export a NODES list for auto-discovery.
NODES = [
    {"type": "action", "name": "search_web", "fn": search_web},
    {"type": "condition", "name": "has_results", "fn": has_results},
]
