from bt_service.core.blackboard import Blackboard
from bt_service.server.builtin import diary_write_entry_action
from bt_service.server.diary_write_client import DiaryWriteEntryProviderError


def test_diary_write_entry_requires_namespace_id():
    bb = Blackboard.from_json_map({"worker_id": "worker-a", "diary_entry_content": "done"})
    ok, err = diary_write_entry_action(bb)
    assert ok is False
    assert err is not None
    assert "namespace_id" in str(err)


def test_diary_write_entry_requires_worker_id():
    bb = Blackboard.from_json_map({"namespace_id": "ns-1", "diary_entry_content": "done"})
    ok, err = diary_write_entry_action(bb)
    assert ok is False
    assert err is not None
    assert "worker_id" in str(err)


def test_diary_write_entry_requires_content():
    bb = Blackboard.from_json_map({"namespace_id": "ns-1", "worker_id": "worker-a"})
    ok, err = diary_write_entry_action(bb)
    assert ok is False
    assert err is not None
    assert "diary_entry_content" in str(err)


def test_diary_write_entry_provider_success_writes_blackboard(monkeypatch):
    def fake_write(namespace_id: str, worker_id: str, content: str, **kwargs) -> dict:
        return {
            "worker_id": worker_id,
            "task_id": kwargs.get("task_id") or "T1",
            "date": kwargs.get("date") or "2026-07-05",
            "tags": kwargs.get("tags") or ["task"],
        }

    monkeypatch.setattr("bt_service.server.builtin.diary_write_entry", fake_write)
    bb = Blackboard.from_json_map({
        "namespace_id": "ns-1",
        "task_id": "T1",
        "worker_id": "worker-a",
        "diary_entry_content": "done",
        "diary_entry_tags": ["task"],
    })
    ok, err = diary_write_entry_action(bb)
    assert ok is True
    assert err is None
    assert bb.get("diary_written") is True
    assert bb.get("diary_date") == "2026-07-05"
    assert bb.get("diary_worker_id") == "worker-a"
    assert bb.get("diary_task_id") == "T1"
    assert bb.get("diary_tags") == ["task"]


def test_diary_write_entry_provider_error_returns_failure(monkeypatch):
    def fake_write(namespace_id: str, worker_id: str, content: str, **kwargs) -> dict:
        raise DiaryWriteEntryProviderError("boom")

    monkeypatch.setattr("bt_service.server.builtin.diary_write_entry", fake_write)
    bb = Blackboard.from_json_map({"namespace_id": "ns-1", "task_id": "T1", "worker_id": "worker-a", "diary_entry_content": "done"})
    ok, err = diary_write_entry_action(bb)
    assert ok is False
    assert err is not None
    assert "boom" in str(err)
