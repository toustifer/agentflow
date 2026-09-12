"""Worker diary write provider client for worker diary_write_entry.

Uses a narrow loopback HTTP endpoint exposed by the Go bridge.
This is intentionally specific to persisting one worker diary entry.
"""

from __future__ import annotations

import json
import os
import urllib.request


class DiaryWriteEntryProviderError(RuntimeError):
    pass


def diary_write_entry(
    namespace_id: str,
    worker_id: str,
    content: str,
    *,
    task_id: str = "",
    date: str = "",
    tags: list[str] | None = None,
) -> dict:
    url = os.getenv("AGENTFLOW_BT_DIARY_WRITE_URL", "")
    token = os.getenv("AGENTFLOW_BT_DIARY_WRITE_TOKEN", "")
    if not url:
        raise DiaryWriteEntryProviderError("diary_write_entry provider URL not configured")
    if not namespace_id:
        raise DiaryWriteEntryProviderError("namespace_id is required")
    if not worker_id:
        raise DiaryWriteEntryProviderError("worker_id is required")
    if not content:
        raise DiaryWriteEntryProviderError("content is required")

    payload = {
        "namespace_id": namespace_id,
        "worker_id": worker_id,
        "content": content,
    }
    if task_id:
        payload["task_id"] = task_id
    if date:
        payload["date"] = date
    if tags:
        payload["tags"] = tags

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
        raise DiaryWriteEntryProviderError(f"diary_write_entry provider request failed: {e}") from e

    if not isinstance(data, dict):
        raise DiaryWriteEntryProviderError("diary_write_entry provider returned non-object")
    return data
