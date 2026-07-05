"""Task submit for review provider client for worker task_submit_for_review.

Uses a narrow loopback HTTP endpoint exposed by the Go bridge.
This is intentionally specific to advancing the task into review_pending.
"""

from __future__ import annotations

import json
import os
import urllib.request


class TaskSubmitForReviewProviderError(RuntimeError):
    pass


def task_submit_for_review(namespace_id: str, task_id: str, worker_id: str) -> dict:
    url = os.getenv("AGENTFLOW_BT_SUBMIT_REVIEW_URL", "")
    token = os.getenv("AGENTFLOW_BT_SUBMIT_REVIEW_TOKEN", "")
    if not url:
        raise TaskSubmitForReviewProviderError("task_submit_for_review provider URL not configured")
    if not namespace_id:
        raise TaskSubmitForReviewProviderError("namespace_id is required")
    if not task_id:
        raise TaskSubmitForReviewProviderError("task_id is required")
    if not worker_id:
        raise TaskSubmitForReviewProviderError("worker_id is required")

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
        raise TaskSubmitForReviewProviderError(f"task_submit_for_review provider request failed: {e}") from e

    if not isinstance(data, dict):
        raise TaskSubmitForReviewProviderError("task_submit_for_review provider returned non-object")
    return data
