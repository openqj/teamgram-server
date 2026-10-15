import copy
import unittest
from unittest import mock

import messaging_capacity_probe as capacity


def fixture():
    return {"schemaVersion": 1, "phase": "capacity", "evidenceClass": "local_probe", "productionSloEvidence": False,
            "clock": "performance.now", "ok": True, "processExitCode": 0, "cleanup": {"ok": True, "remaining": 0},
            "checks": {key: True for key in ("ptsContinuity", "historiesUnique", "liveMatchesHistory", "outboxDrainedBeforeCleanup")},
            "capacity": {"rate": 2, "durationMs": 1000, "planned": 2, "samples": [
                {"slot": i, "direction": "B-to-A" if i else "A-to-B", "scheduledAtMs": i * 500,
                 "dispatchedAtMs": i * 500 + 1, "ackAtMs": i * 500 + 20, "liveAtMs": i * 500 + 100,
                 "ack": {"id": i+1}, "live": {"id": i+1}} for i in range(2)]}}


class CapacityTests(unittest.TestCase):
    def test_stage_metric_delta_uses_bounded_labels(self):
        before = {"metric": [{"labels": {"queue": "inbox", "stage": "consumer_ack"}, "value": 4}]}
        after = {"metric": [{"labels": {"stage": "consumer_ack", "queue": "inbox"}, "value": 9}]}
        delta = capacity.stage_metric_delta(before, after)
        self.assertEqual(delta["metric"][0]["value"], 5)

    def test_settled_stage_metrics_waits_for_this_profile(self):
        before = {"syncDeliveryCount": [{"labels": {"stage": "update_prepare"}, "value": 7}],
                  "syncResults": [{"labels": {"stage": "update_prepare", "outcome": "ok"}, "value": 7}]}
        stale = copy.deepcopy(before)
        advanced = {"syncDeliveryCount": [{"labels": {"stage": "update_prepare"}, "value": 8}],
                    "syncResults": [{"labels": {"stage": "update_prepare", "outcome": "ok"}, "value": 8}]}
        with mock.patch.object(capacity, "stage_metric_snapshot", side_effect=[stale, advanced, advanced, advanced]), \
                mock.patch.object(capacity.time, "sleep") as sleep:
            result = capacity.settled_stage_metrics(19091, before, timeout=1, quiet_samples=2)
        self.assertEqual(result, advanced)
        self.assertEqual(sleep.call_count, 3)

    def test_every_planned_slot_stays_in_denominator(self):
        value = fixture()
        self.assertEqual(capacity.evaluate(value)["status"], "PASS")
        value["capacity"]["samples"][1] = {"slot": 1, "direction": "B-to-A", "scheduledAtMs": 500, "dropped": "MAX_INFLIGHT"}
        report = capacity.evaluate(value)
        self.assertEqual(report["counts"]["planned"], 2)
        self.assertEqual(report["liveOnTimeRatio"], .5)
        self.assertEqual(report["status"], "FAIL")
        value["capacity"]["samples"].pop()
        with self.assertRaises(ValueError):
            capacity.evaluate(value)

    def test_late_drain_is_not_window_goodput_or_on_time(self):
        value = fixture()
        value["capacity"]["samples"][0]["liveAtMs"] = 2500
        report = capacity.evaluate(value)
        self.assertEqual(report["counts"]["delivered"], 2)
        self.assertEqual(report["counts"]["live_within_window"], 1)
        self.assertEqual(report["counts"]["live_on_time"], 1)
        self.assertEqual(report["status"], "FAIL")

    def test_late_delivery_counts_as_complete_but_misses_latency_budget(self):
        value = fixture()
        value["capacity"]["samples"][0].update(liveAtMs=8500, liveLate=True)
        report = capacity.evaluate(value)
        self.assertTrue(report["completeDelivery"])
        self.assertEqual(report["counts"]["live_late"], 1)
        self.assertEqual(report["counts"]["live_unobserved"], 0)
        self.assertEqual(report["liveSuccessRatio"], 1)
        self.assertEqual(report["liveOnTimeRatio"], .5)
        self.assertEqual(report["status"], "FAIL")

    def test_cleanup_and_consistency_are_required(self):
        for key in ("ptsContinuity", "outboxDrainedBeforeCleanup"):
            value = copy.deepcopy(fixture())
            value["checks"][key] = False
            self.assertFalse(capacity.evaluate(value)["consistency"])


if __name__ == "__main__":
    unittest.main()
