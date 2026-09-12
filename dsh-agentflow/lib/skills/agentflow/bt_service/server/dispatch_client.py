"""Dispatch provider client for leader dispatch_task.

Uses a narrow loopback HTTP endpoint exposed by the Go bridge.
This is intentionally specific to dispatching a single ready task.
"""

from __future__ import annotations

import json
import os
import urllib.request


class DispatchProviderError(RuntimeError):
    pass


def dispatch_task(namespace_id: str, task_id: str) -> dict:
    url = os.getenv("AGENTFLOW_BT_DISPATCH_URL", "")
    token = os.getenv("AGENTFLOW_BT_DISPATCH_TOKEN", "")
    if not url:
        raise DispatchProviderError("dispatch provider URL not configured")
    if not namespace_id:
        raise DispatchProviderError("namespace_id is required")
    if not task_id:
        raise DispatchProviderError("task_id is required")

    req = urllib.request.Request(
        url,
        data=json.dumps({"namespace_id": namespace_id, "task_id": task_id}, ensure_ascii=False).encode("utf-8"),
        headers={
            "Content-Type": "application/json",
            "X-Agentflow-BT-Token": token,
        },
        method="POST",
    )

    try:
        with urllib.request.urlopen(req, timeout=5) as resp:
            body = resp.read().decode("utf-8")
            data = json.loads(body)
    except Exception as e:
        raise DispatchProviderError(f"dispatch provider request failed: {e}") from e

    if not isinstance(data, dict):
        raise DispatchProviderError("dispatch provider returned non-object")
    return data
