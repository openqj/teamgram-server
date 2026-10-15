from __future__ import annotations

import json
from pathlib import Path
import subprocess
import sys
import unittest
from unittest.mock import Mock

sys.path.insert(0, str(Path(__file__).resolve().parent))

import messaging_channel_recovery_probe as probe
from postgres_recovery_probe import ProbeError


class ChannelRecoveryProbeRunnerTests(unittest.TestCase):
    def _lab(self, stdout: str, stderr: str = "", returncode: int = 1, secrets: list[str] | None = None):
        lab = Mock()
        lab.env = {"TEAMGRAM_RELIABILITY_MT_PORT": "21548"}
        lab.secret_values = secrets or []
        lab.run.return_value = subprocess.CompletedProcess([], returncode, stdout, stderr)
        return lab

    def test_preserves_structured_failure_evidence(self):
        evidence = {"probe": "messaging.channel-difference-recovery", "ok": False, "errorCode": "CHANNEL_INVALID"}
        lab = self._lab(json.dumps(evidence))
        report = {}

        with self.assertRaisesRegex(ProbeError, "CHANNEL_INVALID"):
            probe.run_probe(lab, probe.ROOT.parent / "telegram-tt-master", 60, report)

        self.assertEqual(report["recovery"], evidence)

    def test_preserves_redacted_process_output_without_structured_evidence(self):
        lab = self._lab("stdout secret-value", "stderr detail", secrets=["secret-value"])
        report = {}

        with self.assertRaisesRegex(ProbeError, "no structured evidence"):
            probe.run_probe(lab, probe.ROOT.parent / "telegram-tt-master", 60, report)

        self.assertEqual(report["recovery_probe_process"]["returncode"], 1)
        self.assertNotIn("secret-value", report["recovery_probe_process"]["stdout_tail"])
        self.assertEqual(report["recovery_probe_process"]["stderr_tail"], "stderr detail")


if __name__ == "__main__":
    unittest.main()
