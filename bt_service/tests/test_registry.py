"""Tests for FactoryRegistry listing and type registration."""

from bt_service.factory.registry import FactoryRegistry, register_default_nodes


class TestFactoryList:
    def test_default_types(self):
        reg = FactoryRegistry()
        register_default_nodes(reg)
        types = reg.list_types()
        for name in ["Sequence", "Fallback", "ReactiveSequence", "ReactiveFallback",
                       "Inverter", "Retry", "Condition", "Action", "SubTree", "Wait", "Log"]:
            assert name in types, f"{name} not registered"
