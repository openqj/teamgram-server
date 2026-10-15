import sys
from pathlib import Path
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parent))

import messaging_online_capacity_probe as probe


class OnlineCapacityEvaluationTest(unittest.TestCase):
    def _evidence(self, **overrides):
        online = {
            "ok": True,
            "connect": {"connected": 1000},
            "hold": {"minConnected": 1000},
            "reconnect": {"connected": 10},
            "serviceGauge": {"observed": True, "min": 1000, "cleanupObserved": True},
        }
        online.update(overrides)
        return {"online": online}

    def test_accepts_complete_online_evidence(self):
        result = probe.evaluate(self._evidence(), 1000, 10)
        self.assertEqual(result["status"], "PASS")

    def test_rejects_client_drop_during_hold(self):
        result = probe.evaluate(self._evidence(hold={"minConnected": 999}), 1000, 10)
        self.assertEqual(result["status"], "FAIL")
        self.assertFalse(result["checks"]["allConnectionsHeld"])

    def test_rejects_missing_server_gauge(self):
        result = probe.evaluate(self._evidence(serviceGauge={"observed": False, "min": None, "cleanupObserved": False}), 1000, 10)
        self.assertEqual(result["status"], "FAIL")
        self.assertFalse(result["checks"]["serverActiveGauge"])

    def test_rejects_connection_leak(self):
        result = probe.evaluate(self._evidence(serviceGauge={"observed": True, "min": 1000, "cleanupObserved": False}), 1000, 10)
        self.assertEqual(result["status"], "FAIL")
        self.assertFalse(result["checks"]["connectionCleanup"])

    def test_mixed_acceptance_requires_both_online_and_message_budgets(self):
        load = {"schemaVersion": 1, "phase": "capacity", "evidenceClass": "local_probe",
                "productionSloEvidence": False, "clock": "performance.now", "ok": True,
                "processExitCode": 0, "cleanup": {"ok": True, "remaining": 0},
                "checks": {key: True for key in ("ptsContinuity", "historiesUnique", "liveMatchesHistory", "outboxDrainedBeforeCleanup")},
                "capacity": {"rate": 1, "durationMs": 1000, "planned": 1, "samples": [
                    {"slot": 0, "direction": "A-to-B", "scheduledAtMs": 0, "dispatchedAtMs": 1,
                     "ackAtMs": 20, "liveAtMs": 100, "ack": {"id": 1}, "live": {"id": 1}}]}}
        result = probe.evaluate_mixed(self._evidence()["online"], load, 1000, 10)
        self.assertEqual(result["status"], "PASS")
        load["capacity"]["samples"][0]["liveAtMs"] = 3000
        result = probe.evaluate_mixed(self._evidence()["online"], load, 1000, 10)
        self.assertEqual(result["status"], "FAIL")


if __name__ == "__main__":
    unittest.main()
