"""Implement code provider client for worker implement_code.

Uses a narrow loopback HTTP endpoint exposed by the Go bridge.
This is intentionally specific to preparing implementation context.
"""

from __future__ import annotations

import json
import os
import urllib.request


class ImplementCodeProviderError(RuntimeError):
    pass


def implement_code(namespace_id: str, task_id: str, worker_id: str) -> dict:
    url = os.getenv("AGENTFLOW_BT_IMPLEMENT_CODE_URL", "")
    token = os.getenv("AGENTFLOW_BT_IMPLEMENT_CODE_TOKEN", "")
    if not url:
        raise ImplementCodeProviderError("implement_code provider URL not configured")
    if not namespace_id:
        raise ImplementCodeProviderError("namespace_id is required")
    if not task_id:
        raise ImplementCodeProviderError("task_id is required")
    if not worker_id:
        raise ImplementCodeProviderError("worker_id is required")

    req = urllib.request.Request(
        url,
        data=json.dumps({"namespace_id": namespace_id, "task_id": task_id, "worker_id": worker_id}, ensure_ascii=False).encode("utf-8"),
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
        raise ImplementCodeProviderError(f"implement_code provider request failed: {e}") from e

    if not isinstance(data, dict):
        raise ImplementCodeProviderError("implement_code provider returned non-object")
    return data
