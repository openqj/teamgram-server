import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

import messaging_slo_report as slo


def probe_fixture(samples=40, phase='single'):
    state = lambda pts: {'pts': pts, 'qts': 0, 'seq': 1, 'date': 1000}
    deliveries = []
    for index in range(samples + 2):
        direction = 'A-to-B' if index % 2 == 0 else 'B-to-A'
        deliveries.append({
            'direction': direction, 'ok': True, 'deliveryMs': 10, 'rpcMs': 5,
            'freshSession': index >= samples, 'messageId': index + 1,
            'pts': index + 1, 'ptsCount': 1, 'date': 1000,
            'before': {'A': state(index), 'B': state(index)},
            'after': {'A': state(index + 1), 'B': state(index + 1)},
            'contiguous': True,
        })
    return {
        'schemaVersion': 1, 'probe': slo.PROBE, 'phase': phase, 'ok': True,
        'evidenceClass': 'local_probe', 'productionSloEvidence': False,
        'clock': 'performance.now', 'loadDurationMs': 400,
        'checks': {'ptsContinuity': True, 'historiesUnique': True,
                   'randomIdReplay': True, 'freshSessionReadbackAndSend': True},
        'freshSession': {'sameAuthKeys': True, 'newClientObjects': True, 'newAuthKeys': False},
        'cleanup': {'ok': True, 'remaining': 0},
        'rpcSamples': [{'ok': True, 'elapsedMs': 5, 'purpose': 'send'} for _ in deliveries],
        'deliverySamples': deliveries,
        'replaySamples': [{'ok': True, 'noNewUpdate': True,
                          'before': {'A': state(1), 'B': state(1)},
                          'after': {'A': state(1), 'B': state(1)}} for _ in range(4)],
    }


class MessagingSloReportTests(unittest.TestCase):
    def test_nearest_rank_percentiles(self):
        self.assertEqual(slo.distribution(list(range(1, 21))),
                         {'count': 20, 'p50Ms': 10, 'p95Ms': 19, 'p99Ms': 20})
        self.assertIsNone(slo.percentile([], .95))

    def test_local_pass_uses_raw_samples_and_excludes_fresh_from_load(self):
        probe = probe_fixture()
        probe['stats'] = {'delivery': {'p95Ms': 999999}}
        result = slo.evaluate(probe)
        self.assertEqual(result['status'], 'PASS')
        self.assertEqual(result['delivery']['count'], 40)
        self.assertEqual(result['throughput']['messagesPerSecond'], 100)
        self.assertFalse(result['productionSloEvidence'])
        self.assertIsNone(result['productionWindowDays'])

    def test_one_send_each_way_is_always_small_sample(self):
        result = slo.evaluate(probe_fixture(2), min_samples=1)
        self.assertEqual(result['status'], 'INSUFFICIENT_SAMPLES')
        self.assertEqual(result['evidenceClass'], 'SMOKE_ONLY')

    def test_latency_budget_failure(self):
        probe = probe_fixture()
        for value in probe['deliverySamples']:
            value['deliveryMs'] = 2001
        result = slo.evaluate(probe)
        self.assertIn('exceeded:delivery_p95', result['failures'])
        self.assertEqual(result['status'], 'FAIL')

    def test_failed_rpc_and_timeout_cannot_pass(self):
        probe = probe_fixture()
        probe['rpcSamples'].append({'ok': False, 'elapsedMs': 8000, 'error': 'PROBE_TIMEOUT'})
        result = slo.evaluate(probe)
        self.assertEqual(result['status'], 'FAIL')
        self.assertLess(result['rpc']['successRate'], 1)

    def test_pts_gap_rejected_even_with_true_summary_flags(self):
        probe = probe_fixture()
        probe['deliverySamples'][0]['after']['B']['pts'] += 1
        self.assertIn('delivery_sample_failed_or_invalid', slo.evaluate(probe)['failures'])

    def test_replay_that_advances_either_account_pts_rejected(self):
        probe = probe_fixture()
        probe['replaySamples'][0]['after']['A']['pts'] += 1
        self.assertIn('replay_advanced_pts', slo.evaluate(probe)['failures'])

    def test_missing_direction_and_claimed_new_auth_are_rejected(self):
        probe = probe_fixture()
        for value in probe['deliverySamples']:
            value['direction'] = 'A-to-B'
        self.assertIn('missing_or_invalid_direction', slo.evaluate(probe)['failures'])
        probe = probe_fixture()
        probe['freshSession']['newAuthKeys'] = True
        self.assertIn('fresh_session_evidence_missing', slo.evaluate(probe)['failures'])

    def test_cleanup_must_read_back_zero_remaining(self):
        probe = probe_fixture()
        probe['cleanup']['remaining'] = 1
        self.assertIn('cleanup_not_verified', slo.evaluate(probe)['failures'])

    def test_before_phase_cannot_be_full_acceptance(self):
        probe = probe_fixture(40, 'before')
        probe['cleanup'] = {'ok': True, 'retainedForRestart': True}
        result = slo.evaluate(probe)
        self.assertEqual(result['status'], 'PRE_RESTART_ONLY')
        self.assertFalse(result['productionSloEvidence'])

    def test_after_phase_is_recovery_evidence_not_a_load_slo(self):
        probe = probe_fixture(0, 'after')
        self.assertEqual(slo.evaluate(probe)['status'], 'RESTART_RECOVERY_ONLY')

    def test_nonfinite_or_missing_raw_metrics_rejected(self):
        for invalid in (float('nan'), float('inf'), -1, True):
            with self.subTest(invalid=invalid):
                probe = probe_fixture()
                probe['deliverySamples'][0]['deliveryMs'] = invalid
                self.assertEqual(slo.evaluate(probe)['status'], 'FAIL')
        probe = probe_fixture()
        probe['rpcSamples'] = []
        self.assertEqual(slo.evaluate(probe)['status'], 'FAIL')

    def test_invalid_duration_can_be_safely_serialized(self):
        probe = probe_fixture()
        probe['loadDurationMs'] = float('inf')
        result = slo.evaluate(probe)
        self.assertEqual(result['status'], 'FAIL')
        json.dumps(result, allow_nan=False)

    def test_missing_fresh_session_object_rejected(self):
        probe = probe_fixture()
        probe['freshSession'] = None
        self.assertEqual(slo.evaluate(probe)['status'], 'FAIL')

    def test_critical_path_exit_status_cannot_be_overridden_by_probe(self):
        probe = probe_fixture()
        value = {'results': [{'id': slo.PROBE, 'result': 'PASS', 'stdout': json.dumps(probe)}]}
        self.assertEqual(slo.extract_probes(value), [probe])
        value['results'][0]['result'] = 'TIMEOUT'
        with self.assertRaises(ValueError):
            slo.extract_probes(value)

    def test_cli_redacts_unparseable_evidence(self):
        with tempfile.TemporaryDirectory() as directory:
            evidence = Path(directory) / 'evidence.json'
            evidence.write_text('sensitive-auth-key-do-not-echo', encoding='utf-8')
            result = subprocess.run([sys.executable, str(Path(slo.__file__)), str(evidence)], capture_output=True, text=True)
        self.assertEqual(result.returncode, 1)
        self.assertNotIn('sensitive-auth-key', result.stdout + result.stderr)
        self.assertEqual(json.loads(result.stdout)['status'], 'FAIL')


if __name__ == '__main__':
    unittest.main()
