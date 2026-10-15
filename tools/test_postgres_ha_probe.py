import subprocess
import unittest
from unittest.mock import Mock, patch

import postgres_ha_probe as ha


class HaAssertionsTests(unittest.TestCase):
    def test_sync_standby_is_registered_in_dcs_before_fault(self):
        lab = Mock()
        with patch.object(ha, "sync_state", return_value={"leader": "pg1", "sync_standby": ""}), patch.object(ha, "sql") as sql:
            self.assertFalse(ha.synchronous(lab, "pg1"))
            sql.assert_not_called()
        with patch.object(ha, "sync_state", return_value={"leader": "pg1", "sync_standby": "pg2"}), patch.object(ha, "sql", return_value='["pg3"]'):
            self.assertFalse(ha.synchronous(lab, "pg1"))
        with patch.object(ha, "sync_state", return_value={"leader": "pg1", "sync_standby": "pg2"}), patch.object(ha, "sql", return_value='["pg2"]'):
            self.assertTrue(ha.synchronous(lab, "pg1"))

    def test_fencing_requires_recovery_and_read_only_sqlstate(self):
        lab = Mock()
        lab.run.side_effect = [subprocess.CompletedProcess([], 0, "t\n", ""),
                               subprocess.CompletedProcess([], 3, "", "ERROR:  25006: cannot execute INSERT in a read-only transaction")]
        self.assertTrue(ha.local_write_rejected(lab, "pg1"))
        for code in ("42P01", "28P01"):
            lab.run.side_effect = [subprocess.CompletedProcess([], 0, "t\n", ""),
                                   subprocess.CompletedProcess([], 3, "", f"ERROR: {code}: unrelated error")]
            with self.subTest(code=code), self.assertRaises(ha.ProbeError):
                ha.local_write_rejected(lab, "pg1")
        lab.run.side_effect = [subprocess.CompletedProcess([], 0, "f\n", "")]
        self.assertFalse(ha.local_write_rejected(lab, "pg1"))

    def test_unknown_running_member_prevents_single_leader_claim(self):
        lab = Mock()
        lab.container.return_value = {"State": {"Running": True}}
        states = {"pg1": {"role": "primary", "state": "running"}, "pg2": {}, "pg3": {"role": "replica"}}
        with patch.object(ha, "roles", return_value=states):
            self.assertIsNone(ha.leader(lab))
        states["pg2"] = {"role": "primary", "state": "running"}
        with patch.object(ha, "roles", return_value=states), self.assertRaises(ha.ProbeError):
            ha.leader(lab)


if __name__ == "__main__":
    unittest.main()
