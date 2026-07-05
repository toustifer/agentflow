"""Task get confirm provider client for worker task_get_confirm.

Uses a narrow loopback HTTP endpoint exposed by the Go bridge.
This is intentionally specific to confirming one worker task context.
"""

from __future__ import annotations

import json
import os
import urllib.request


class TaskGetConfirmProviderError(RuntimeError):
    pass


def task_get_confirm(namespace_id: str, task_id: str, worker_id: str = "") -> dict:
    url = os.getenv("AGENTFLOW_BT_TASK_GET_URL", "")
    token = os.getenv("AGENTFLOW_BT_TASK_GET_TOKEN", "")
    if not url:
        raise TaskGetConfirmProviderError("task_get_confirm provider URL not configured")
    if not namespace_id:
        raise TaskGetConfirmProviderError("namespace_id is required")
    if not task_id:
        raise TaskGetConfirmProviderError("task_id is required")

    payload = {"namespace_id": namespace_id, "task_id": task_id}
    if worker_id:
        payload["worker_id"] = worker_id

    req = urllib.request.Request(
        url,
        data=json.dumps(payload, ensure_ascii=False).encode("utf-8"),
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
        raise TaskGetConfirmProviderError(f"task_get_confirm provider request failed: {e}") from e

    if not isinstance(data, dict):
        raise TaskGetConfirmProviderError("task_get_confirm provider returned non-object")
    return data
