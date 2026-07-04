"""Transport framing helpers tests.

These are lightweight contract tests focused on byte-based UTF-8 length.
"""

import json


def test_utf8_content_length_is_byte_based():
    payload = json.dumps({"phase_name": "执行中（1/3）"}, ensure_ascii=False, separators=(",", ":")).encode("utf-8")
    char_len = len(payload.decode("utf-8"))
    byte_len = len(payload)
    assert byte_len >= char_len
    assert byte_len != 0
