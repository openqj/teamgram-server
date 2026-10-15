import json
from pathlib import Path
import tempfile
import unittest

from messaging_slo_metrics import render_metrics, write_metrics
from messaging_slo_window import DEFINITION, LEGACY_SCHEMA, SCHEMA_VERSION


def row(**overrides):
    value = {"schemaVersion": SCHEMA_VERSION, "type": "messaging.slo.minute", "environment": "production",
             "region": "test-1", "source": "synthetic-layer229", "apiLayer": 229,
             "definition": DEFINITION, "shard": "pair-1", "bucketStart": "2026-10-15T00:00:00+00:00",
             "bucketSeconds": 60,
             "observedAt": "2026-10-15T00:01:00+00:00",
             "planned": 2, "eligible": 2, "ackGood": 2, "liveGood": 1,
             "settled": True, "collectorComplete": True, "counterReset": False,
             "measurements": {"connection": {"complete": True, "eligible": 2, "good": 2},
                              "reconnect": {"complete": True, "eligible": 1, "good": 1},
                              "messageIntegrity": {"complete": True, "eligible": 2, "lost": 0, "duplicates": 1}}}
    value.update(overrides)
    return value


def recovery_event():
    return {"schemaVersion": SCHEMA_VERSION, "type": "messaging.slo.recovery", "environment": "production",
            "region": "test-1", "source": "synthetic-layer229", "apiLayer": 229, "definition": DEFINITION,
            "shard": "pair-1", "eventId": "drill-1", "startedAt": "2026-10-15T00:00:00+00:00",
            "recoveredAt": "2026-10-15T00:00:12+00:00", "recoverySeconds": 12, "drillId": "INC-1"}


class MessagingSloMetricsTests(unittest.TestCase):
    def test_rebuilds_stable_counters_by_scope(self):
        first = row()
        second = row(bucketStart="2026-10-15T00:01:00+00:00", observedAt="2026-10-15T00:02:00+00:00",
                     ackGood=1, liveGood=2)
        output = render_metrics([first, second])
        self.assertIn('teamgram_slo_ack_eligible_total{environment="production",region="test-1",source="synthetic-layer229"} 4', output)
        self.assertIn('teamgram_slo_ack_good_total{environment="production",region="test-1",source="synthetic-layer229"} 3', output)
        self.assertIn('teamgram_slo_live_good_total{environment="production",region="test-1",source="synthetic-layer229"} 3', output)
        self.assertIn('teamgram_slo_connection_good_total{environment="production",region="test-1",source="synthetic-layer229"} 4', output)
        self.assertIn('teamgram_slo_message_duplicates_total{environment="production",region="test-1",source="synthetic-layer229"} 2', output)

    def test_incomplete_bucket_counts_as_failed_scheduled_canary(self):
        output = render_metrics([row(ackGood=0, liveGood=0, collectorComplete=False,
                                     measurements={"connection": {"complete": False},
                                                   "reconnect": {"complete": False},
                                                   "messageIntegrity": {"complete": False}})])
        self.assertIn("teamgram_slo_ack_eligible_total{", output)
        self.assertIn('teamgram_slo_live_good_total{environment="production",region="test-1",source="synthetic-layer229"} 0', output)
        self.assertIn('teamgram_slo_measurement_complete{environment="production",region="test-1",source="synthetic-layer229",sli="connection"} 0', output)

    def test_measurement_completeness_recovers_from_a_newer_healthy_bucket(self):
        incomplete = row(bucketStart="2026-10-15T00:01:00+00:00", observedAt="2026-10-15T00:02:00+00:00",
                         ackGood=0, liveGood=0, collectorComplete=False,
                         measurements={"connection": {"complete": False},
                                       "reconnect": {"complete": False},
                                       "messageIntegrity": {"complete": False}})
        recovered = row(bucketStart="2026-10-15T00:02:00+00:00", observedAt="2026-10-15T00:03:00+00:00")
        output = render_metrics([row(), incomplete, recovered])
        self.assertIn("Whether the latest settled bucket is complete", output)
        self.assertIn('teamgram_slo_measurement_complete{environment="production",region="test-1",source="synthetic-layer229",sli="connection"} 1', output)
        latest_incomplete = render_metrics([row(), incomplete])
        self.assertIn('teamgram_slo_measurement_complete{environment="production",region="test-1",source="synthetic-layer229",sli="connection"} 0', latest_incomplete)

    def test_recovery_events_export_histogram(self):
        output = render_metrics([row(), recovery_event()])
        self.assertIn('teamgram_slo_recovery_seconds_bucket{environment="production",region="test-1",source="synthetic-layer229",le="30"} 1', output)
        self.assertIn('teamgram_slo_recovery_seconds_count{environment="production",region="test-1",source="synthetic-layer229"} 1', output)

    def test_legacy_rows_preserve_ack_live_but_mark_new_slis_incomplete(self):
        legacy = row(schemaVersion=LEGACY_SCHEMA[0], definition=LEGACY_SCHEMA[1], measurements=None)
        output = render_metrics([legacy])
        self.assertIn('teamgram_slo_ack_good_total{environment="production",region="test-1",source="synthetic-layer229"} 2', output)
        self.assertIn('teamgram_slo_measurement_complete{environment="production",region="test-1",source="synthetic-layer229",sli="connection"} 0', output)

    def test_latest_evidence_timestamp_is_a_gauge(self):
        output = render_metrics([row()])
        self.assertIn("# TYPE teamgram_slo_evidence_latest_timestamp_seconds gauge", output)
        self.assertIn('teamgram_slo_evidence_latest_timestamp_seconds{environment="production",region="test-1",source="synthetic-layer229"} 1792022460.0', output)

    def test_duplicate_or_inconsistent_evidence_is_rejected(self):
        with self.assertRaisesRegex(ValueError, "duplicate"):
            render_metrics([row(), row()])
        with self.assertRaisesRegex(ValueError, "inconsistent"):
            render_metrics([row(ackGood=3)])
        with self.assertRaisesRegex(ValueError, "duplicate"):
            render_metrics([row(), recovery_event(), recovery_event()])
        with self.assertRaisesRegex(ValueError, "duplicate"):
            render_metrics([row(), recovery_event(), recovery_event().copy() | {"shard": "pair-2"}])

    def test_output_rebuild_is_atomic_and_repeatable(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            evidence, output = root / "minutes.jsonl", root / "node/teamgram_slo.prom"
            evidence.write_text(json.dumps(row()) + "\n", encoding="utf-8")
            write_metrics(evidence, output)
            first = output.read_text(encoding="utf-8")
            self.assertEqual(output.stat().st_mode & 0o777, 0o644)
            write_metrics(evidence, output)
            self.assertEqual(output.read_text(encoding="utf-8"), first)
            self.assertEqual(list(output.parent.glob(".teamgram_slo.prom.*")), [])


if __name__ == "__main__":
    unittest.main()
