"""Project doc write provider client for worker doc_write_record.

Uses a narrow loopback HTTP endpoint exposed by the Go bridge.
This is intentionally specific to persisting one project doc record.
"""

from __future__ import annotations

import json
import os
import urllib.request


class DocWriteRecordProviderError(RuntimeError):
    pass


def doc_write_record(
    namespace_id: str,
    task_id: str,
    worker_id: str,
    content: str,
    *,
    title: str = "",
    path: str = "",
    section: str = "",
    tags: list[str] | None = None,
    doc_id: int = 0,
) -> dict:
    url = os.getenv("AGENTFLOW_BT_DOC_WRITE_URL", "")
    token = os.getenv("AGENTFLOW_BT_DOC_WRITE_TOKEN", "")
    if not url:
        raise DocWriteRecordProviderError("doc_write_record provider URL not configured")
    if not namespace_id:
        raise DocWriteRecordProviderError("namespace_id is required")
    if not task_id:
        raise DocWriteRecordProviderError("task_id is required")
    if not worker_id:
        raise DocWriteRecordProviderError("worker_id is required")
    if not content:
        raise DocWriteRecordProviderError("content is required")

    payload = {
        "namespace_id": namespace_id,
        "task_id": task_id,
        "worker_id": worker_id,
        "content": content,
    }
    if title:
        payload["title"] = title
    if path:
        payload["path"] = path
    if section:
        payload["section"] = section
    if tags:
        payload["tags"] = tags
    if doc_id:
        payload["doc_id"] = doc_id

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
        raise DocWriteRecordProviderError(f"doc_write_record provider request failed: {e}") from e

    if not isinstance(data, dict):
        raise DocWriteRecordProviderError("doc_write_record provider returned non-object")
    return data
