"""Fetch work diff provider client for reviewer fetch_work_diff.

Uses a narrow loopback HTTP endpoint exposed by the Go bridge.
This is intentionally specific to fetching review commit/diff context.
"""

from __future__ import annotations

import json
import os
import urllib.request


class FetchWorkDiffProviderError(RuntimeError):
    pass


def fetch_work_diff(namespace_id: str, task_id: str, worker_id: str) -> dict:
    url = os.getenv("AGENTFLOW_BT_FETCH_DIFF_URL", "")
    token = os.getenv("AGENTFLOW_BT_FETCH_DIFF_TOKEN", "")
    if not url:
        raise FetchWorkDiffProviderError("fetch_work_diff provider URL not configured")
    if not namespace_id:
        raise FetchWorkDiffProviderError("namespace_id is required")
    if not task_id:
        raise FetchWorkDiffProviderError("task_id is required")
    if not worker_id:
        raise FetchWorkDiffProviderError("worker_id is required")

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
        raise FetchWorkDiffProviderError(f"fetch_work_diff provider request failed: {e}") from e

    if not isinstance(data, dict):
        raise FetchWorkDiffProviderError("fetch_work_diff provider returned non-object")
    return data
