"""Safety regressions for the disposable PostgreSQL lab (no Docker required)."""

from copy import deepcopy
import json
from pathlib import Path
import subprocess
import sys
import unittest
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parent))
import postgres_recovery_probe as probe


def config_for(lab):
    return {
        "volumes": {"primary_data": {"name": f"{lab.project}_primary_data", "labels": {probe.RUN_LABEL: lab.run_id}}},
        "networks": {"default": {"name": f"{lab.project}_default", "labels": {probe.RUN_LABEL: lab.run_id}}},
        "services": {"postgres": {"labels": {probe.RUN_LABEL: lab.run_id},
                                  "ports": [{"host_ip": "127.0.0.1", "published": "55435"}],
                                  "volumes": [{"type": "bind", "source": str(probe.ROOT / "docker/reliability/postgres"),
                                               "read_only": True}]}},
    }


class LabSafetyTests(unittest.TestCase):
    def setUp(self):
        self.lab = probe.Lab("teamgram-reliability-unittest")

    def test_old_project_names_are_rejected_before_docker(self):
        for name in ("backend", "teamgram", "teamgram-reliability", "teamgram-reliability-../backend", "TEAMGRAM-reliability-test"):
            with self.subTest(name=name), self.assertRaises(probe.ProbeError):
                probe.Lab(name)

    def test_project_environment_cannot_redirect_compose(self):
        with patch.dict(probe.os.environ, {"COMPOSE_FILE": "old.yaml", "COMPOSE_PROJECT_NAME": "backend", "COMPOSE_PROFILES": "old"}):
            lab = probe.Lab("teamgram-reliability-unittest")
        self.assertNotIn("COMPOSE_FILE", lab.env)
        self.assertNotIn("COMPOSE_PROJECT_NAME", lab.env)
        self.assertNotIn("COMPOSE_PROFILES", lab.env)
        self.assertEqual(lab.command("up")[-1], "up")
        self.assertIn(str(probe.COMPOSE_FILE), lab.command("up"))
        self.assertIn("/dev/null", lab.command("up"))

    def test_config_rejects_external_or_shared_volume(self):
        for change in ({"external": True}, {"name": "backend_postgres_data"}, {"labels": {}}):
            config = config_for(self.lab)
            config["volumes"]["primary_data"].update(change)
            with self.subTest(change=change), self.assertRaises(probe.ProbeError):
                self.lab.validate_config(config)

    def test_config_rejects_public_ports_and_existing_data(self):
        config = config_for(self.lab)
        self.lab.validate_config(config)
        invalid = deepcopy(config)
        invalid["services"]["postgres"]["ports"][0]["host_ip"] = "0.0.0.0"
        with self.assertRaises(probe.ProbeError):
            self.lab.validate_config(invalid)
        for source, readonly in ((probe.ROOT / "data/postgres", True), (probe.ROOT / "docker/reliability", False), ("/tmp/shared", True)):
            invalid = deepcopy(config)
            invalid["services"]["postgres"]["volumes"][0].update(source=str(source), read_only=readonly)
            with self.subTest(source=source), self.assertRaises(probe.ProbeError):
                self.lab.validate_config(invalid)

    def test_preflight_rejects_existing_project_and_does_not_cleanup(self):
        with patch.object(self.lab, "run"), patch.object(self.lab, "resources", return_value={"container": ["existing"], "volume": [], "network": []}):
            with self.assertRaises(probe.ProbeError):
                self.lab.preflight()
            self.assertEqual(self.lab.cleanup()["status"], "SKIPPED")

    def test_preflight_rejects_unlabelled_occupied_volume_name(self):
        config = config_for(self.lab)
        def run(command, **kwargs):
            if "config" in command:
                return subprocess.CompletedProcess(command, 0, json.dumps(config), "")
            return subprocess.CompletedProcess(command, 0, "[]", "")
        with patch.object(self.lab, "run", side_effect=run), patch.object(self.lab, "resources", return_value={"container": [], "volume": [], "network": []}):
            with self.assertRaisesRegex(probe.ProbeError, "existing lab volume name"):
                self.lab.preflight()
            self.assertFalse(self.lab.started)

    def test_cleanup_refuses_another_run_token(self):
        self.lab.started = True
        self.lab.checked = True
        item = {"Config": {"Labels": {probe.RUN_LABEL: "different-run", "com.docker.compose.project": self.lab.project}}}
        with patch.object(self.lab, "resources", return_value={"container": ["old"], "volume": [], "network": []}), \
             patch.object(self.lab, "run", return_value=subprocess.CompletedProcess([], 0, json.dumps([item]), "")) as run:
            with self.assertRaises(probe.ProbeError):
                self.lab.cleanup()
            self.assertFalse(any("down" in call.args[0] for call in run.call_args_list))

    def test_primary_fence_requires_stopped_container_and_disabled_restart(self):
        for running, restart in ((True, "no"), (False, "always")):
            item = {"Id": "primary", "State": {"Running": running}, "HostConfig": {"RestartPolicy": {"Name": restart}}}
            with patch.object(self.lab, "stop"), patch.object(self.lab, "container", return_value=item), self.assertRaises(probe.ProbeError):
                self.lab.fence_primary()
        item = {"Id": "primary", "State": {"Running": False}, "HostConfig": {"RestartPolicy": {"Name": "no"}}}
        with patch.object(self.lab, "stop"), patch.object(self.lab, "container", return_value=item):
            self.assertTrue(self.lab.fence_primary()["old_primary_stopped"])

    def test_exact_markers_reject_missing_extra_or_changed_commit(self):
        expected = {"baseline": "ok", "committed": "ok"}
        probe.require_snapshot(expected, expected, "recovery")
        for actual in ({"baseline": "ok"}, {**expected, "later": "bad"}, {"baseline": "ok", "committed": "changed"}):
            with self.subTest(actual=actual), self.assertRaises(probe.ProbeError):
                probe.require_snapshot(actual, expected, "recovery")

    def test_error_redaction_removes_password_and_uri_credentials(self):
        message = "password=example-secret postgres://user:other-secret@postgres:5432/lab"
        redacted = probe.redact(message, ["example-secret"])
        self.assertNotIn("example-secret", redacted)
        self.assertNotIn("user:other-secret", redacted)
        self.assertIn("<redacted>", redacted)

    def test_timed_out_or_interrupted_command_terminates_its_process_group(self):
        for error in (subprocess.TimeoutExpired(["docker"], 1), KeyboardInterrupt()):
            process = unittest.mock.Mock(pid=1234)
            process.communicate.side_effect = [error, ("", "")]
            with self.subTest(error=type(error).__name__), patch.object(probe.subprocess, "Popen", return_value=process), \
                 patch.object(probe.os, "killpg") as kill:
                with self.assertRaises((probe.ProbeError, KeyboardInterrupt)):
                    self.lab.run(["docker"])
                kill.assert_called_once_with(1234, probe.signal.SIGTERM)


if __name__ == "__main__":
    unittest.main()
