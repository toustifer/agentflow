"""Git commit changes provider client for worker git_commit_changes.

Uses a narrow loopback HTTP endpoint exposed by the Go bridge.
This is intentionally specific to capturing review commit/diff metadata.
"""

from __future__ import annotations

import json
import os
import urllib.request


class GitCommitChangesProviderError(RuntimeError):
    pass


def git_commit_changes(namespace_id: str, task_id: str, worker_id: str) -> dict:
    url = os.getenv("AGENTFLOW_BT_GIT_COMMIT_URL", "")
    token = os.getenv("AGENTFLOW_BT_GIT_COMMIT_TOKEN", "")
    if not url:
        raise GitCommitChangesProviderError("git_commit_changes provider URL not configured")
    if not namespace_id:
        raise GitCommitChangesProviderError("namespace_id is required")
    if not task_id:
        raise GitCommitChangesProviderError("task_id is required")
    if not worker_id:
        raise GitCommitChangesProviderError("worker_id is required")

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
        raise GitCommitChangesProviderError(f"git_commit_changes provider request failed: {e}") from e

    if not isinstance(data, dict):
        raise GitCommitChangesProviderError("git_commit_changes provider returned non-object")
    return data
