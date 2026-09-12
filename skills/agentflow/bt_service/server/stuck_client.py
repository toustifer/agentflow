"""Stuck provider client for leader report_stuck.

Uses a narrow loopback HTTP endpoint exposed by the Go bridge.
This is intentionally specific to reporting one anchor stuck task.
"""

from __future__ import annotations

import json
import os
import urllib.request


class ReportStuckProviderError(RuntimeError):
    pass


def report_stuck(namespace_id: str, task_id: str) -> dict:
    url = os.getenv("AGENTFLOW_BT_STUCK_URL", "")
    token = os.getenv("AGENTFLOW_BT_STUCK_TOKEN", "")
    if not url:
        raise ReportStuckProviderError("stuck provider URL not configured")
    if not namespace_id:
        raise ReportStuckProviderError("namespace_id is required")
    if not task_id:
        raise ReportStuckProviderError("task_id is required")

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
        raise ReportStuckProviderError(f"stuck provider request failed: {e}") from e

    if not isinstance(data, dict):
        raise ReportStuckProviderError("stuck provider returned non-object")
    return data
