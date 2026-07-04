"""Shared RPC contract helpers for bt_service."""

JSONRPC_VERSION = "2.0"
MAX_FRAME_BYTES = 4 * 1024 * 1024


def success_response(req_id, result: dict) -> dict:
    return {
        "jsonrpc": JSONRPC_VERSION,
        "id": req_id,
        "result": result,
    }


def error_response(req_id, code: int, message: str, data: dict | None = None) -> dict:
    resp = {
        "jsonrpc": JSONRPC_VERSION,
        "id": req_id,
        "error": {
            "code": code,
            "message": message,
        },
    }
    if data:
        resp["error"]["data"] = data
    return resp
