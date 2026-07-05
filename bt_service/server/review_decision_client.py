"""Reviewer decision provider clients for reviewer pass/rework actions."""

from __future__ import annotations

import json
import os
import urllib.request


class TaskReviewPassProviderError(RuntimeError):
    pass


class TaskReviewReworkProviderError(RuntimeError):
    pass


def task_review_pass(namespace_id: str, task_id: str, worker_id: str = "") -> dict:
    return _call_reviewer_endpoint(
        os.getenv("AGENTFLOW_BT_REVIEW_PASS_URL", ""),
        os.getenv("AGENTFLOW_BT_REVIEW_PASS_TOKEN", ""),
        namespace_id,
        task_id,
        worker_id,
        TaskReviewPassProviderError,
        "task_review_pass",
    )


def task_review_rework(namespace_id: str, task_id: str, worker_id: str = "") -> dict:
    return _call_reviewer_endpoint(
        os.getenv("AGENTFLOW_BT_REVIEW_REWORK_URL", ""),
        os.getenv("AGENTFLOW_BT_REVIEW_REWORK_TOKEN", ""),
        namespace_id,
        task_id,
        worker_id,
        TaskReviewReworkProviderError,
        "task_review_rework",
    )


def _call_reviewer_endpoint(url: str, token: str, namespace_id: str, task_id: str, worker_id: str, error_cls, label: str) -> dict:
    if not url:
        raise error_cls(f"{label} provider URL not configured")
    if not namespace_id:
        raise error_cls("namespace_id is required")
    if not task_id:
        raise error_cls("task_id is required")

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
        raise error_cls(f"{label} provider request failed: {e}") from e

    if not isinstance(data, dict):
        raise error_cls(f"{label} provider returned non-object")
    return data
