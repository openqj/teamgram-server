#!/usr/bin/env python3

from __future__ import annotations

import unittest
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import messaging_group_channel_fanout_probe as probe


class GroupChannelFanoutProbeTests(unittest.TestCase):
    def _evidence(self, **overrides):
        evidence = {
            "connections": {
                "authenticated": 5000,
                "activated": 5000,
                "connectedAtStart": 5000,
                "connectedAtEnd": 5000,
                "endpointDistribution": {"backend-1": 2500, "backend-2": 2500},
            },
            "serverGaugeAtStart": 5000,
            "connectionCleanupObserved": True,
            "recoveryPlan": {"requestedPerTarget": 100},
            "deliveries": [
                {"kind": kind, "channelId": channel_id, "uniqueRecipients": 5000,
                 "liveRecipients": 4900, "liveRecipientsExpected": 4900, "missingRecipients": 0,
                 "duplicateUpdates": 0, "ptsValid": True, "channelHistoryReadByListener": True,
                 "ackWithin1s": True, "receiveWithin2s": 4900,
                 "recovery": {"recoveredRecipients": 100, "duplicateRecoveredMessages": 0,
                              "unexpectedRealtimeRecipients": 0, "ptsContinuous": True,
                              "allFinal": True, "historyReadable": True,
                              "reconnectedRecipients": 100}}
                for kind, channel_id in (("channel", "101"), ("megagroup", "102"))
            ],
        }
        evidence.update(overrides)
        return evidence

    def _database(self):
        return [{"channel_id": channel_id, "events": 1, "completed_events": 1,
                 "pending_events": 0, "recipient_rows": 0, "pending_recipients": 0}
                for channel_id in (101, 102)]

    def test_accepts_complete_live_fanout_for_both_target_kinds(self):
        result = probe.evaluate(self._evidence(), self._database(), 5000, 2)
        self.assertEqual(result["status"], "PASS")

    def test_rejects_missing_or_duplicate_live_updates(self):
        evidence = self._evidence()
        evidence["deliveries"][0]["recovery"]["recoveredRecipients"] = 99
        evidence["deliveries"][1]["duplicateUpdates"] = 1
        result = probe.evaluate(evidence, self._database(), 5000, 2)
        self.assertEqual(result["status"], "FAIL")

    def test_marks_complete_but_late_delivery_as_budget_exceeded(self):
        evidence = self._evidence()
        evidence["deliveries"][0]["receiveWithin2s"] = 4800
        result = probe.evaluate(evidence, self._database(), 5000, 2)
        self.assertEqual(result["status"], "BUDGET_EXCEEDED")

    def test_requires_both_backend_connection_shares(self):
        evidence = self._evidence()
        evidence["connections"]["endpointDistribution"]["backend-2"] = 2499
        result = probe.evaluate(evidence, self._database(), 5000, 2)
        self.assertEqual(result["status"], "FAIL")

    def test_rejects_realtime_delivery_to_disconnected_recovery_cohort(self):
        evidence = self._evidence()
        evidence["deliveries"][0]["recovery"]["unexpectedRealtimeRecipients"] = 1
        result = probe.evaluate(evidence, self._database(), 5000, 2)
        self.assertEqual(result["status"], "FAIL")

    def test_stage_metric_delta_matches_bounded_labels(self):
        before = {"count": [{"labels": {"stage": "sync_rpc"}, "value": 2}]}
        after = {"count": [{"labels": {"stage": "sync_rpc"}, "value": 5},
                            {"labels": {"stage": "receipt_persist"}, "value": 1}]}
        delta = probe.fanout_stage_metric_delta(before, after)
        self.assertEqual(delta["count"], [
            {"labels": {"stage": "sync_rpc"}, "value": 3},
            {"labels": {"stage": "receipt_persist"}, "value": 1},
        ])

    def test_sync_partition_override_defaults_to_six(self):
        parser = probe.build_parser()
        self.assertEqual(parser.parse_args([]).sync_partitions, 6)
        self.assertEqual(parser.parse_args(["--sync-partitions", "48"]).sync_partitions, 48)

if __name__ == "__main__":
    unittest.main()
