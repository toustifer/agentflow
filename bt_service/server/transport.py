"""JSON-RPC transport — Content-Length framed stdio loop.

This module uses binary stdin/stdout and byte-based Content-Length,
so framing is correct for non-ASCII payloads on Windows and other platforms.
"""

from __future__ import annotations

import json
import sys
from typing import Callable

MAX_FRAME_BYTES = 4 * 1024 * 1024  # 4 MiB
JSONRPC_VERSION = "2.0"


def _readline_ascii() -> bytes:
    return sys.stdin.buffer.readline()


def read_frame() -> dict | None:
    """Read a JSON-RPC request from stdin with Content-Length framing.

    Returns the parsed dict, or None on EOF.
    """
    content_length = 0
    while True:
        line = _readline_ascii()
        if not line:
            return None
        line = line.rstrip(b"\r\n")
        if line == b"":
            break
        lower = line.lower()
        if lower.startswith(b"content-length:"):
            try:
                content_length = int(line.split(b":", 1)[1].strip())
            except (ValueError, IndexError):
                raise ValueError("invalid Content-Length header")

    if content_length <= 0:
        raise ValueError("missing or empty Content-Length")
    if content_length > MAX_FRAME_BYTES:
        raise ValueError(f"frame too large: {content_length}")

    body = sys.stdin.buffer.read(content_length)
    if len(body) != content_length:
        raise ValueError("short read on frame body")
    return json.loads(body.decode("utf-8"))


def write_frame(data: dict) -> None:
    """Write a JSON-RPC response to stdout with Content-Length framing."""
    payload = json.dumps(data, ensure_ascii=False, separators=(",", ":")).encode("utf-8")
    header = f"Content-Length: {len(payload)}\r\n\r\n".encode("ascii")
    sys.stdout.buffer.write(header)
    sys.stdout.buffer.write(payload)
    sys.stdout.buffer.flush()


def run_stdio_loop(handler: Callable[[str, dict], dict]) -> None:
    """Run the JSON-RPC stdio loop.

    Reads requests from stdin, dispatches via handler, writes responses.
    """
    while True:
        req = read_frame()
        if req is None:
            break

        req_id = req.get("id")
        method = req.get("method", "")
        params = req.get("params", {}) or {}

        try:
            if req.get("jsonrpc") != JSONRPC_VERSION:
                raise ValueError("jsonrpc must be '2.0'")
            if not isinstance(method, str) or not method:
                raise ValueError("method is required")
            if not isinstance(params, dict):
                raise ValueError("params must be an object")

            result = handler(method, params)
            write_frame({
                "jsonrpc": JSONRPC_VERSION,
                "id": req_id,
                "result": result,
            })
        except Exception as e:
            write_frame({
                "jsonrpc": JSONRPC_VERSION,
                "id": req_id,
                "error": {"code": -32603, "message": str(e)},
            })
