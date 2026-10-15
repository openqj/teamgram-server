import copy
import datetime as dt
import unittest

import messaging_slo_window as slo

END = dt.datetime(2026, 10, 15, tzinfo=slo.UTC)


def buckets(source="synthetic-layer229", eligible=2):
    start = END - dt.timedelta(days=30)
    return [{"schemaVersion": 1, "type": "messaging.slo.minute", "environment": "local-isolated", "region": "local",
             "source": source, "apiLayer": 229, "definition": slo.DEFINITION, "shard": "pair-1", "bucketSeconds": 60,
             "bucketStart": (start + dt.timedelta(minutes=i)).isoformat(),
             "observedAt": (start + dt.timedelta(minutes=i+1)).isoformat(), "planned": eligible, "eligible": eligible,
             "ackGood": eligible, "liveGood": eligible, "settled": True, "collectorComplete": True, "counterReset": False}
            for i in range(slo.MINUTES)]


def evaluate(rows, source="synthetic-layer229"):
    return slo.evaluate(rows, end=END, as_of=END, environment="local-isolated", region="local", source=source, shards=["pair-1"])


class SloWindowTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.complete = buckets()

    def test_complete_30day_synthetic_window(self):
        result = evaluate(self.complete)
        self.assertEqual(result["status"], "PASS")
        self.assertEqual(result["eligible"], 86400)
        self.assertFalse(result["productionSloAccepted"])

    def test_missing_minute_never_passes(self):
        result = evaluate(self.complete[:-1])
        self.assertEqual(result["status"], "INCOMPLETE")
        self.assertEqual(result["coverage"]["missingBuckets"], 1)

    def test_conflicting_or_identical_duplicate_is_invalid(self):
        for good in (1, 2):
            duplicate = {**self.complete[0], "ackGood": good}
            self.assertEqual(evaluate([*self.complete, duplicate])["status"], "INVALID")

    def test_no_traffic_and_nearly_empty_window_cannot_pass(self):
        rows = buckets("organic-layer229", 0)
        self.assertEqual(evaluate(rows, "organic-layer229")["status"], "NO_TRAFFIC")
        rows[0].update(planned=1, eligible=1, ackGood=1, liveGood=1)
        self.assertEqual(evaluate(rows, "organic-layer229")["status"], "INCOMPLETE")
        rows = [{**row, "source": "synthetic-layer229"} for row in rows]
        self.assertEqual(evaluate(rows)["status"], "INVALID")

    def test_counts_are_weighted_across_days_not_average_of_percentiles(self):
        rows = copy.deepcopy(self.complete)
        for index in range(100):
            rows[index*400]["liveGood"] = 0
        self.assertEqual(evaluate(rows)["status"], "FAIL")
        # Missing observations cannot mask already observed bad messages.
        self.assertEqual(evaluate(rows[:40001])["status"], "FAIL")

    def test_early_bad_message_does_not_exhaust_whole_window_budget(self):
        row = {**self.complete[0], "ackGood": 1}
        result = evaluate([row])
        self.assertEqual(result["status"], "INCOMPLETE")
        self.assertTrue(result["observedRatioBelowTarget"])
        self.assertFalse(result["wholeWindowErrorBudgetExceeded"])
        # A later retry cannot overwrite the original failure bucket.
        self.assertEqual(evaluate([row, self.complete[0]])["status"], "INVALID")

    def test_unsettled_reset_future_and_overlap_evidence(self):
        for field, value in (("settled", False), ("collectorComplete", False), ("counterReset", True)):
            rows = [*self.complete]
            rows[0] = {**rows[0], field: value}
            self.assertEqual(evaluate(rows)["status"], "INCOMPLETE")
        for change in ({"observedAt": (END + dt.timedelta(minutes=1)).isoformat()}, {"bucketSeconds": 120},
                       {"eligible": 3}, {"bucketStart": "2026-09-15T00:00:30+00:00"}):
            rows = [*self.complete]
            rows[0] = {**rows[0], **change}
            self.assertEqual(evaluate(rows)["status"], "INVALID")


if __name__ == "__main__":
    unittest.main()
