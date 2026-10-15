from pathlib import Path
import tempfile
import unittest

from messaging_slo_recovery_event import append_event, make_event


class MessagingSloRecoveryEventTests(unittest.TestCase):
    def event(self, **overrides):
        values = {"event_id": "drill-1", "environment": "production", "region": "test-1",
                  "source": "synthetic-layer229", "shard": "pair-1",
                  "started_at": "2026-10-15T00:00:00Z", "recovered_at": "2026-10-15T00:00:12Z",
                  "drill_id": "INC-123"}
        values.update(overrides)
        return make_event(**values)

    def test_production_event_requires_measured_window_and_drill(self):
        self.assertEqual(self.event()["recoverySeconds"], 12)
        with self.assertRaises(ValueError):
            self.event(drill_id="")
        with self.assertRaises(ValueError):
            self.event(drill_id="   ")
        with self.assertRaises(ValueError):
            self.event(recovered_at="2026-10-14T23:59:59Z")

    def test_append_is_idempotent_and_rejects_conflicting_event_id(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "evidence.jsonl"
            event = self.event()
            self.assertTrue(append_event(path, event))
            self.assertFalse(append_event(path, event))
            with self.assertRaisesRegex(ValueError, "different evidence"):
                append_event(path, self.event(recovered_at="2026-10-15T00:00:15Z"))
            with self.assertRaisesRegex(ValueError, "different evidence"):
                append_event(path, self.event(shard="pair-2"))
            self.assertEqual(len(path.read_text().splitlines()), 1)
            self.assertEqual(path.stat().st_mode & 0o777, 0o600)


if __name__ == "__main__":
    unittest.main()
