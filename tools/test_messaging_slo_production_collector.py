import datetime as dt
import json
import os
from pathlib import Path
import tempfile
import unittest
from types import SimpleNamespace

from messaging_slo_production_collector import collect_minute, measurement_row
from messaging_slo_window import UTC


def complete_report(tag):
    return {
        "probe": "messaging.cross-account-live-updates", "phase": "slo", "tag": tag,
        "ok": True, "collectionComplete": True,
        "checks": {"sloCanary": True, "historiesUnique": True, "ptsContinuity": True},
        "cleanup": {"ok": True, "remaining": 0},
        "deliverySamples": [
            {"direction": "A-to-B", "ok": True, "rpcMs": 20.0, "deliveryMs": 40.0},
            {"direction": "B-to-A", "ok": True, "rpcMs": 30.0, "deliveryMs": 50.0},
        ],
        "connectionObservation": {"eligible": 22, "good": 22},
        "reconnectSample": {"eligible": 2, "good": 2},
        "messageIntegrity": {"eligible": 2, "lost": 0, "duplicates": 0, "complete": True},
    }


class Clock:
    def __init__(self, value):
        self.value = value

    def now(self):
        return self.value

    def sleep(self, seconds):
        self.value += seconds


class ProductionSloCollectorTests(unittest.TestCase):
    def test_maps_only_complete_layer229_probe_evidence(self):
        result = measurement_row(complete_report("run-1"))
        self.assertTrue(result["collectorComplete"])
        self.assertEqual(result["ackGood"], 2)
        self.assertEqual(result["liveGood"], 2)
        self.assertEqual(result["measurements"]["connection"],
                         {"complete": True, "eligible": 22, "good": 22})

    def test_missing_measurement_group_never_passes(self):
        report = complete_report("run-1")
        report["collectionComplete"] = False
        result = measurement_row(report)
        self.assertFalse(result["collectorComplete"])
        self.assertFalse(result["measurements"]["messageIntegrity"]["complete"])

    def test_ack_and_live_delivery_failures_are_counted_independently(self):
        report = complete_report("run-1")
        report["ok"] = False
        report["deliverySamples"][0] = {
            "direction": "A-to-B", "ok": False, "rpcMs": 20.0, "error": "LIVE_UPDATE_TIMEOUT",
        }
        report["messageIntegrity"].update(lost=1, duplicates=0)
        result = measurement_row(report)
        self.assertTrue(result["collectorComplete"])
        self.assertEqual(result["ackGood"], 2)
        self.assertEqual(result["liveGood"], 1)
        self.assertEqual(result["measurements"]["messageIntegrity"]["lost"], 1)

    def test_minute_run_appends_private_raw_and_rebuilds_metrics(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            clock = Clock(dt.datetime(2025, 10, 15, 10, 20, 2, tzinfo=UTC).timestamp())

            def runner(command, **kwargs):
                tag = kwargs["env"]["PROBE_TAG"]
                return SimpleNamespace(returncode=0, stdout=json.dumps(complete_report(tag)))

            environment = {key: "test" for key in (
                "A_AUTH_B64", "B_AUTH_B64", "A_USER_ID", "B_USER_ID", "A_ACCESS_HASH",
                "B_ACCESS_HASH", "TG_DC_HOST", "TG_DC_PORT")}
            row = collect_minute(
                evidence=root / "production.jsonl", metrics=root / "textfile/teamgram_slo.prom",
                raw_dir=root / "raw", region="region-1", shard="gateway-a",
                probe=root / "probe.ts", gramjs_dir=root / "gramjs", environment=environment,
                now_fn=clock.now, sleep_fn=clock.sleep, run=runner,
            )

            self.assertTrue(row["collectorComplete"])
            self.assertTrue(row["settled"])
            self.assertEqual(row["eligible"], 2)
            self.assertGreaterEqual(dt.datetime.fromisoformat(row["observedAt"]).timestamp(),
                                    dt.datetime.fromisoformat(row["bucketStart"]).timestamp() + 60)
            self.assertEqual(os.stat(root / "production.jsonl").st_mode & 0o777, 0o600)
            self.assertEqual(os.stat(row["rawEvidence"]).st_mode & 0o777, 0o600)
            self.assertIn("teamgram_slo_live_good_total", (root / "textfile/teamgram_slo.prom").read_text())

    def test_late_timer_writes_failed_bucket_without_running_canary(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            clock = Clock(dt.datetime(2025, 10, 15, 10, 20, 7, tzinfo=UTC).timestamp())
            environment = {key: "test" for key in (
                "A_AUTH_B64", "B_AUTH_B64", "A_USER_ID", "B_USER_ID", "A_ACCESS_HASH",
                "B_ACCESS_HASH", "TG_DC_HOST", "TG_DC_PORT")}
            row = collect_minute(
                evidence=root / "production.jsonl", metrics=root / "textfile/teamgram_slo.prom",
                raw_dir=root / "raw", region="region-1", shard="gateway-a",
                probe=root / "probe.ts", gramjs_dir=root / "gramjs", environment=environment,
                now_fn=clock.now, sleep_fn=clock.sleep,
                run=lambda *_args, **_kwargs: self.fail("late collection must not send a canary"),
            )
            self.assertFalse(row["collectorComplete"])
            self.assertEqual(row["collectorError"], "COLLECTOR_MISSED_SCHEDULE")
            self.assertTrue(row["settled"])

    def test_duplicate_minute_is_rejected(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            clock = Clock(dt.datetime(2025, 10, 15, 10, 20, 7, tzinfo=UTC).timestamp())
            environment = {key: "test" for key in (
                "A_AUTH_B64", "B_AUTH_B64", "A_USER_ID", "B_USER_ID", "A_ACCESS_HASH",
                "B_ACCESS_HASH", "TG_DC_HOST", "TG_DC_PORT")}
            kwargs = dict(
                evidence=root / "production.jsonl", metrics=root / "textfile/teamgram_slo.prom",
                raw_dir=root / "raw", region="region-1", shard="gateway-a",
                probe=root / "probe.ts", gramjs_dir=root / "gramjs", environment=environment,
                now_fn=clock.now, sleep_fn=clock.sleep,
                run=lambda *_args, **_kwargs: None,
            )
            collect_minute(**kwargs)
            clock.value -= 60
            with self.assertRaisesRegex(ValueError, "already has evidence"):
                collect_minute(**kwargs)


if __name__ == "__main__":
    unittest.main()
