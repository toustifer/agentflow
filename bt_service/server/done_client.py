"""Done provider client for leader report_done.

Uses a narrow loopback HTTP endpoint exposed by the Go bridge.
This is intentionally specific to reporting namespace-level completion.
"""

from __future__ import annotations

import json
import os
import urllib.request


class ReportDoneProviderError(RuntimeError):
    pass


def report_done(namespace_id: str) -> dict:
    url = os.getenv("AGENTFLOW_BT_DONE_URL", "")
    token = os.getenv("AGENTFLOW_BT_DONE_TOKEN", "")
    if not url:
        raise ReportDoneProviderError("done provider URL not configured")
    if not namespace_id:
        raise ReportDoneProviderError("namespace_id is required")

    req = urllib.request.Request(
        url,
        data=json.dumps({"namespace_id": namespace_id}, ensure_ascii=False).encode("utf-8"),
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
        raise ReportDoneProviderError(f"done provider request failed: {e}") from e

    if not isinstance(data, dict):
        raise ReportDoneProviderError("done provider returned non-object")
    return data
