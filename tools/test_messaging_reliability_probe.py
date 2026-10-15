"""Protocol and isolation regressions for the local communication runner."""

import base64
from concurrent.futures import Future
import hashlib
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parent))
import messaging_reliability_probe as probe


class MessagingRunnerTests(unittest.TestCase):
    def setUp(self):
        self.lab = probe.Lab("teamgram-reliability-msg-unittest", env={"TEAMGRAM_RELIABILITY_MT_PORT": "21543"})

    def test_ports_reject_collisions_and_privileged_bindings(self):
        probe.validate_ports([55436, 21543, 19094, 19090])
        for ports in ([55436, 21543, 21543, 19090], [80, 21543, 19094, 19090], [65536, 21543, 19094, 19090]):
            with self.subTest(ports=ports), self.assertRaises(probe.ProbeError):
                probe.validate_ports(ports)

    def test_process_health_waits_for_gateway_callback_listener(self):
        item = {"State": {"Running": True, "Health": {"Status": "healthy"}}}
        results = [subprocess.CompletedProcess([], 1, "", ""), subprocess.CompletedProcess([], 0, "", "")]
        with patch.object(self.lab, "container", return_value=item), patch.object(self.lab, "run", side_effect=results) as run, patch.object(probe.time, "sleep"):
            probe.wait_backend(self.lab)
        self.assertEqual(run.call_count, 2)
        self.assertIn("20110", run.call_args.args[0][-1])
        self.assertNotIn("20420", run.call_args.args[0][-1])

    def test_sampler_must_run_after_send_and_cannot_be_in_the_future(self):
        self.assertTrue(probe.fresh_outage_sample(1001, 1000, 1002))
        self.assertFalse(probe.fresh_outage_sample(999, 1000, 1002))
        self.assertFalse(probe.fresh_outage_sample(1003, 1000, 1002))
        self.assertFalse(probe.fresh_outage_sample(1001, 1000, 1026))

    def test_seed_keys_have_mtproto_ids_and_one_authoritative_transaction(self):
        with patch.object(self.lab, "sql") as sql:
            probe.seed_accounts(self.lab)
        statement = sql.call_args.args[1]
        self.assertTrue(statement.startswith("BEGIN;\n"))
        self.assertTrue(statement.endswith("\nCOMMIT;"))
        keys = []
        for role in ("A", "B"):
            key = base64.b64decode(self.lab.env[f"{role}_AUTH_B64"], validate=True)
            self.assertEqual(len(key), 256)
            key_id = int.from_bytes(hashlib.sha1(key).digest()[-8:], "little", signed=True)
            self.assertIn(f"VALUES ({key_id},decode(", statement)
            self.assertIn(f"VALUES ({key_id},0,{key_id})", statement)
            self.assertIn(key.hex(), self.lab.secret_values)
            keys.append(key)
        self.assertNotEqual(*keys)

    def test_probe_requires_successful_exit_and_matching_run(self):
        evidence = {"probe": "messaging.cross-account-live-updates", "phase": "single", "tag": self.lab.run_id, "ok": True}
        result = subprocess.CompletedProcess([], 0, json.dumps(evidence), "")
        with patch.object(self.lab, "run", return_value=result):
            self.assertEqual(probe.probe(self.lab, Path("/fake/gramjs"), "single", Path("/tmp/state"), 20), evidence)
        for change, code in (({"tag": "other-run"}, 0), ({"phase": "before"}, 0), ({"ok": False}, 0), ({}, 1)):
            value = {**evidence, **change}
            with self.subTest(change=change, code=code), patch.object(self.lab, "run", return_value=subprocess.CompletedProcess([], code, json.dumps(value), "")), self.assertRaises(probe.ProbeError):
                probe.probe(self.lab, Path("/fake/gramjs"), "single", Path("/tmp/state"), 20)

    def test_probe_can_route_two_accounts_to_distinct_gateways(self):
        evidence = {"probe": "messaging.cross-account-live-updates", "phase": "capacity", "tag": self.lab.run_id, "ok": True}
        result = subprocess.CompletedProcess([], 0, json.dumps(evidence), "")
        with patch.object(self.lab, "run", return_value=result):
            probe.probe(self.lab, Path("/fake/gramjs"), "capacity", Path("/tmp/state"), 1,
                        actor_ports=(21543, 21544))
        self.assertEqual(self.lab.env["A_TG_DC_PORT"], "21543")
        self.assertEqual(self.lab.env["B_TG_DC_PORT"], "21544")
        with patch.object(self.lab, "run", return_value=subprocess.CompletedProcess(
                [], 0, json.dumps({"probe": "messaging.cross-account-live-updates", "phase": "single",
                                   "tag": self.lab.run_id, "ok": True}), "")):
            probe.probe(self.lab, Path("/fake/gramjs"), "single", Path("/tmp/state"), 20)
        self.assertNotIn("A_TG_DC_PORT", self.lab.env)
        self.assertNotIn("B_TG_DC_PORT", self.lab.env)

    def test_raw_sdk_error_cannot_enter_failure_reason(self):
        error = probe.MessagingProbeError("single", {"error": {"code": "request includes secret"}})
        self.assertNotIn("secret", str(error))
        self.assertIn("UNKNOWN_PROBE_FAILURE", str(error))

    def test_control_rejects_other_run_and_early_exit(self):
        future = Future()
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory)
            probe.fault_control(self.lab, path, "ready")
            self.assertEqual(probe.wait_control(self.lab, path, "ready", future)["tag"], self.lab.run_id)
            (path / "ready.json").write_text(json.dumps({"schemaVersion": 1, "tag": "other", "stage": "ready"}))
            with self.assertRaises(probe.ProbeError):
                probe.wait_control(self.lab, path, "ready", future)
            future.set_result({"ok": True})
            with self.assertRaises(probe.ProbeError):
                probe.wait_control(self.lab, path, "sent", future)

    def test_control_reports_fixed_probe_failure_stage(self):
        future = Future()
        future.set_result({"phase": "capacity", "error": {
            "stage": "fixed-rate-capacity", "code": "CAPACITY_PTS_GAP_OR_DUPLICATE"}})
        with tempfile.TemporaryDirectory() as directory, self.assertRaisesRegex(
                probe.ProbeError, "fixed-rate-capacity/CAPACITY_PTS_GAP_OR_DUPLICATE"):
            probe.wait_control(self.lab, Path(directory), "sent", future)

    def test_outage_requires_both_commits_and_retained_delivery_intents(self):
        self.lab.env.update(A_USER_ID="910001", B_USER_ID="910002")
        sent = {"messages": [{"sender": role, "text": f"{self.lab.run_id}:{role}:fault", "randomId": str(index)} for index, role in enumerate(("A", "B"), 1)]}
        expected = {"sender_rows": 2, "recipient_rows": 0, "pending_intents": 4}
        with patch.object(self.lab, "sql", return_value=json.dumps(expected)):
            self.assertEqual(probe.fault_sql(self.lab, sent), expected)
        for snapshot in ({**expected, "sender_rows": 1}, {**expected, "recipient_rows": 1}, {**expected, "pending_intents": 0}):
            with self.subTest(snapshot=snapshot), patch.object(self.lab, "sql", return_value=json.dumps(snapshot)), self.assertRaises(probe.ProbeError):
                probe.fault_sql(self.lab, sent)
        sent["messages"][0]["text"] += "'; DROP TABLE messages;--"
        with patch.object(self.lab, "sql") as sql, self.assertRaises(probe.ProbeError):
            probe.fault_sql(self.lab, sent)
        sql.assert_not_called()


if __name__ == "__main__":
    unittest.main()
