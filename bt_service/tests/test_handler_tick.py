"""Additional handler/phase provider related tests."""

import os

from bt_service.server.handler import BTServer
from bt_service.server.phase_client import PhaseProviderError, fetch_phase


class TestPhaseClient:
    def test_missing_url_raises(self):
        old = os.environ.pop("AGENTFLOW_BT_PHASE_URL", None)
        try:
            try:
                fetch_phase("ns-x")
                assert False, "should have raised"
            except PhaseProviderError:
                pass
        finally:
            if old is not None:
                os.environ["AGENTFLOW_BT_PHASE_URL"] = old


class TestHandlerOutputs:
    def test_tick_returns_outputs_from_prefilled_phase_data(self):
        srv = BTServer(trees_dir="trees")
        result = srv.handle("tick", {
            "tree_name": "leader-default",
            "blackboard": {
                "namespace_id": "ns_demo",
                "phase_data": {
                    "phase": "shape",
                    "phase_name": "等待出形态书",
                    "progress": "0%",
                    "actions": ["brainstorm", "worker_register"],
                    "next_tasks": [],
                    "active_tasks": [],
                    "stuck_tasks": [],
                    "has_next_tasks": False,
                    "has_active_tasks": False,
                    "has_stuck_tasks": False,
                },
            },
            "options": {"return_blackboard": True},
        })
        assert result["status"] == "success"
        assert result["outputs"]["phase"] == "shape"
        assert result["outputs"]["actions"] == ["brainstorm", "worker_register"]
        assert result["blackboard"]["phase_name"] == "等待出形态书"

    def test_refresh_phase_falls_back_to_prefilled_data_when_provider_missing(self):
        srv = BTServer(trees_dir="trees")
        result = srv.handle("tick", {
            "tree_name": "leader-default",
            "blackboard": {
                "namespace_id": "ns_demo",
                "phase_data": {
                    "phase": "plan",
                    "phase_name": "等待拆解 DAG",
                    "progress": "0%",
                    "actions": ["dag_create", "task_create_batch"],
                    "next_tasks": [],
                    "active_tasks": [],
                    "stuck_tasks": [],
                    "has_next_tasks": False,
                    "has_active_tasks": False,
                    "has_stuck_tasks": False,
                },
            },
            "options": {"return_blackboard": True},
        })
        assert result["outputs"]["phase"] == "plan"
        assert result["blackboard"]["has_next_tasks"] is False
