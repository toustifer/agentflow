"""Enter worktree provider client for worker enter_worktree.

Uses a narrow loopback HTTP endpoint exposed by the Go bridge.
This is intentionally specific to ensuring one worker task worktree exists.
"""

from __future__ import annotations

import json
import os
import urllib.request


class EnterWorktreeProviderError(RuntimeError):
    pass


def enter_worktree(namespace_id: str, task_id: str, worker_id: str = "") -> dict:
    url = os.getenv("AGENTFLOW_BT_ENTER_WORKTREE_URL", "")
    token = os.getenv("AGENTFLOW_BT_ENTER_WORKTREE_TOKEN", "")
    if not url:
        raise EnterWorktreeProviderError("enter_worktree provider URL not configured")
    if not namespace_id:
        raise EnterWorktreeProviderError("namespace_id is required")
    if not task_id:
        raise EnterWorktreeProviderError("task_id is required")

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
        raise EnterWorktreeProviderError(f"enter_worktree provider request failed: {e}") from e

    if not isinstance(data, dict):
        raise EnterWorktreeProviderError("enter_worktree provider returned non-object")
    return data
