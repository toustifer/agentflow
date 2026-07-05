from bt_service.core.blackboard import Blackboard
from bt_service.server.builtin import doc_write_record_action
from bt_service.server.doc_write_client import DocWriteRecordProviderError


def test_doc_write_record_requires_namespace_id():
    bb = Blackboard.from_json_map({"task_id": "T1", "worker_id": "worker-a", "doc_record_content": "body"})
    ok, err = doc_write_record_action(bb)
    assert ok is False
    assert err is not None
    assert "namespace_id" in str(err)


def test_doc_write_record_requires_task_id():
    bb = Blackboard.from_json_map({"namespace_id": "ns-1", "worker_id": "worker-a", "doc_record_content": "body"})
    ok, err = doc_write_record_action(bb)
    assert ok is False
    assert err is not None
    assert "task_id" in str(err)


def test_doc_write_record_requires_worker_id():
    bb = Blackboard.from_json_map({"namespace_id": "ns-1", "task_id": "T1", "doc_record_content": "body"})
    ok, err = doc_write_record_action(bb)
    assert ok is False
    assert err is not None
    assert "worker_id" in str(err)


def test_doc_write_record_requires_content():
    bb = Blackboard.from_json_map({"namespace_id": "ns-1", "task_id": "T1", "worker_id": "worker-a"})
    ok, err = doc_write_record_action(bb)
    assert ok is False
    assert err is not None
    assert "doc_record_content" in str(err)


def test_doc_write_record_provider_success_writes_blackboard(monkeypatch):
    def fake_write(namespace_id: str, task_id: str, worker_id: str, content: str, **kwargs) -> dict:
        return {
            "doc_id": 7,
            "title": kwargs.get("title") or "Implementation Note",
            "path": kwargs.get("path") or "tasks/T1.md",
            "section": kwargs.get("section") or "tasks",
            "tags": kwargs.get("tags") or ["task"],
        }

    monkeypatch.setattr("bt_service.server.builtin.doc_write_record", fake_write)
    bb = Blackboard.from_json_map({
        "namespace_id": "ns-1",
        "task_id": "T1",
        "worker_id": "worker-a",
        "doc_record_content": "body",
        "doc_record_title": "Implementation Note",
        "doc_record_path": "tasks/T1.md",
        "doc_record_section": "tasks",
        "doc_record_tags": ["task"],
    })
    ok, err = doc_write_record_action(bb)
    assert ok is True
    assert err is None
    assert bb.get("doc_recorded") is True
    assert bb.get("recorded_doc_id") == 7
    assert bb.get("recorded_doc_title") == "Implementation Note"
    assert bb.get("recorded_doc_path") == "tasks/T1.md"
    assert bb.get("recorded_doc_section") == "tasks"
    assert bb.get("recorded_doc_tags") == ["task"]


def test_doc_write_record_provider_error_returns_failure(monkeypatch):
    def fake_write(namespace_id: str, task_id: str, worker_id: str, content: str, **kwargs) -> dict:
        raise DocWriteRecordProviderError("boom")

    monkeypatch.setattr("bt_service.server.builtin.doc_write_record", fake_write)
    bb = Blackboard.from_json_map({"namespace_id": "ns-1", "task_id": "T1", "worker_id": "worker-a", "doc_record_content": "body"})
    ok, err = doc_write_record_action(bb)
    assert ok is False
    assert err is not None
    assert "boom" in str(err)
