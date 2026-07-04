"""Phase provider client for refresh_phase.

Uses a narrow loopback HTTP endpoint exposed by the Go bridge.
This is intentionally specific to phase fetching and is not a general callback bus.
"""

from __future__ import annotations

import json
import os
import urllib.request


class PhaseProviderError(RuntimeError):
    pass


def fetch_phase(namespace_id: str, workdir: str = "") -> dict:
    url = os.getenv("AGENTFLOW_BT_PHASE_URL", "")
    token = os.getenv("AGENTFLOW_BT_PHASE_TOKEN", "")
    if not url:
        raise PhaseProviderError("phase provider URL not configured")
    if not namespace_id:
        raise PhaseProviderError("namespace_id is required")

    payload = {"namespace_id": namespace_id}
    if workdir:
        payload["workdir"] = workdir

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
        raise PhaseProviderError(f"phase provider request failed: {e}") from e

    if not isinstance(data, dict):
        raise PhaseProviderError("phase provider returned non-object")
    return data
