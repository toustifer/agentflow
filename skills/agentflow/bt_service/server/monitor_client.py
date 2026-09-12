"""Monitor provider client for leader monitor_tasks.

Uses a narrow loopback HTTP endpoint exposed by the Go bridge.
This is intentionally specific to fetching the current state of one active task.
"""

from __future__ import annotations

import json
import os
import urllib.request


class MonitorProviderError(RuntimeError):
    pass


def monitor_task(namespace_id: str, task_id: str) -> dict:
    url = os.getenv("AGENTFLOW_BT_MONITOR_URL", "")
    token = os.getenv("AGENTFLOW_BT_MONITOR_TOKEN", "")
    if not url:
        raise MonitorProviderError("monitor provider URL not configured")
    if not namespace_id:
        raise MonitorProviderError("namespace_id is required")
    if not task_id:
        raise MonitorProviderError("task_id is required")

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
        raise MonitorProviderError(f"monitor provider request failed: {e}") from e

    if not isinstance(data, dict):
        raise MonitorProviderError("monitor provider returned non-object")
    return data
