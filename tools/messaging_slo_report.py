#!/usr/bin/env python3
"""Evaluate local messaging acceptance evidence; never attest a production SLO.

Example (SLO_SAMPLES=20 means 20 sends in each direction):
  python3 tools/messaging_slo_report.py /tmp/messaging-probe.json --output /tmp/messaging-slo.json

Defaults are LOCAL acceptance budgets chosen for this repository, not Telegram
metrics: 100% successful recorded RPCs, send RPC p95 <= 1000 ms, recipient update
p95 <= 2000 ms, at least 40 load sends with both directions represented. Raw
samples are authoritative; percentiles use the nearest-rank definition. Load
throughput includes state checks and sequential waiting, so it is an achieved
probe rate, not maximum server capacity. Fresh-session sends are excluded from
load statistics. Fewer samples remain SMOKE_ONLY, even with relaxed thresholds.
A before-phase report is PRE_RESTART_ONLY until after and cleanup are verified.
This tool does not aggregate probes into a 30-day production SLO.
"""
from __future__ import annotations

import argparse
import json
import math
from pathlib import Path
from typing import Any

PROBE = "messaging.cross-account-live-updates"


def finite_number(value: Any, *, positive: bool = False) -> bool:
    return isinstance(value, (int, float)) and not isinstance(value, bool) and math.isfinite(value) and (value > 0 if positive else value >= 0)


def percentile(values: list[float], probability: float) -> float | None:
    if not values:
        return None
    ordered = sorted(values)
    return ordered[max(0, math.ceil(len(ordered) * probability) - 1)]


def distribution(values: list[float]) -> dict[str, Any]:
    return {"count": len(values), "p50Ms": percentile(values, .5), "p95Ms": percentile(values, .95), "p99Ms": percentile(values, .99)}


def contiguous_sample(sample: dict[str, Any]) -> bool:
    direction = sample.get("direction")
    if direction not in {"A-to-B", "B-to-A"}:
        return False
    sender, recipient = direction.split("-to-")
    before, after = sample.get("before", {}), sample.get("after", {})
    if not isinstance(before, dict) or not isinstance(after, dict):
        return False
    for actor in (sender, recipient):
        old, new = before.get(actor), after.get(actor)
        if not isinstance(old, dict) or not isinstance(new, dict):
            return False
        if not all(isinstance(state.get(field), int) and not isinstance(state[field], bool)
                   for state in (old, new) for field in ("pts", "qts", "seq", "date")):
            return False
        if new["qts"] != old["qts"] or new["seq"] < old["seq"] or new["date"] < old["date"]:
            return False
    count, pts = sample.get("ptsCount"), sample.get("pts")
    return (isinstance(count, int) and not isinstance(count, bool) and count > 0
            and isinstance(pts, int) and not isinstance(pts, bool)
            and finite_number(sample.get("messageId"), positive=True)
            and pts == before[recipient]["pts"] + count == after[recipient]["pts"]
            and after[sender]["pts"] == before[sender]["pts"] + 1
            and finite_number(sample.get("date"), positive=True)
            and sample["date"] >= before[recipient]["date"])


def extract_probes(value: Any) -> list[dict[str, Any]]:
    """Accept direct probe JSON or this repository's critical-path runner JSON."""
    if isinstance(value, dict) and value.get("probe") == PROBE:
        return [value]
    found: list[dict[str, Any]] = []
    if isinstance(value, dict) and isinstance(value.get("results"), list):
        for row in value["results"]:
            if not isinstance(row, dict) or row.get("id") != PROBE:
                continue
            if row.get("result") != "PASS":
                raise ValueError("critical-path messaging row did not pass")
            for line in str(row.get("stdout", "")).splitlines():
                try:
                    found.extend(extract_probes(json.loads(line)))
                except json.JSONDecodeError:
                    continue
    if not found:
        raise ValueError("no supported messaging probe evidence found")
    return found


def evaluate(probe: dict[str, Any], *, min_samples: int = 40, max_rpc_p95_ms: float = 1000,
             max_delivery_p95_ms: float = 2000, min_throughput: float = 0) -> dict[str, Any]:
    failures: list[str] = []
    if probe.get("schemaVersion") != 1 or probe.get("probe") != PROBE:
        failures.append("unsupported_schema")
    if probe.get("evidenceClass") != "local_probe" or probe.get("productionSloEvidence") is not False or probe.get("clock") != "performance.now":
        failures.append("invalid_evidence_scope_or_clock")
    if probe.get("ok") is not True:
        failures.append("probe_failed")
    checks = probe.get("checks", {})
    phase = probe.get("phase")
    required_checks = ["ptsContinuity", "historiesUnique", "randomIdReplay"]
    if phase in {"single", "after"}:
        required_checks.append("freshSessionReadbackAndSend")
        fresh = probe.get("freshSession", {})
        if not isinstance(fresh, dict):
            fresh = {}
        if fresh.get("sameAuthKeys") is not True or fresh.get("newClientObjects") is not True or fresh.get("newAuthKeys") is not False:
            failures.append("fresh_session_evidence_missing")
    if phase not in {"single", "before", "after"}:
        failures.append("not_a_messaging_acceptance_phase")
    for name in required_checks:
        if not isinstance(checks, dict) or checks.get(name) is not True:
            failures.append(f"missing_check:{name}")
    cleanup = probe.get("cleanup", {})
    if not isinstance(cleanup, dict) or cleanup.get("ok") is not True:
        failures.append("cleanup_failed_or_missing")
    elif phase != "before" and (cleanup.get("remaining") != 0 or cleanup.get("retainedForRestart")):
        failures.append("cleanup_not_verified")
    rpc = probe.get("rpcSamples", [])
    deliveries = probe.get("deliverySamples", [])
    replays = probe.get("replaySamples", [])
    if not isinstance(rpc, list) or not rpc:
        failures.append("rpc_samples_missing")
        rpc = []
    if not isinstance(deliveries, list) or not deliveries:
        failures.append("delivery_samples_missing")
        deliveries = []
    if not isinstance(replays, list) or len(replays) < 2 or not all(isinstance(value, dict) and value.get("ok") is True and value.get("noNewUpdate") is True for value in replays):
        failures.append("replay_evidence_missing_or_failed")
    elif any(not isinstance(value.get("before"), dict) or not isinstance(value.get("after"), dict)
             or any(not isinstance(value["before"].get(actor), dict) or not isinstance(value["after"].get(actor), dict)
                    or value["before"][actor].get("pts") != value["after"][actor].get("pts") for actor in ("A", "B")) for value in replays):
        failures.append("replay_advanced_pts")
    rpc_durations: list[float] = []
    rpc_failed = 0
    for sample in rpc:
        if not isinstance(sample, dict) or not finite_number(sample.get("elapsedMs")) or sample.get("ok") is not True:
            rpc_failed += 1
        elif sample.get("purpose") == "send":
            rpc_durations.append(sample["elapsedMs"])
    if rpc_failed:
        failures.append("rpc_samples_failed_or_invalid")
    load: list[dict[str, Any]] = []
    for sample in deliveries:
        if not isinstance(sample, dict) or sample.get("ok") is not True or not finite_number(sample.get("deliveryMs")) or not contiguous_sample(sample):
            failures.append("delivery_sample_failed_or_invalid")
        elif sample.get("freshSession") is False:
            load.append(sample)
    direction_counts = {direction: sum(value.get("direction") == direction for value in load) for direction in ("A-to-B", "B-to-A")}
    if phase != "after" and (not all(direction_counts.values()) or sum(direction_counts.values()) != len(load)):
        failures.append("missing_or_invalid_direction")
    if len(rpc_durations) != len(deliveries):
        failures.append("send_rpc_sample_count_mismatch")
    latency = distribution([value["deliveryMs"] for value in load])
    rpc_latency = distribution(rpc_durations)
    for name, values, budget in (("send_rpc_p95", rpc_latency, max_rpc_p95_ms), ("delivery_p95", latency, max_delivery_p95_ms)):
        if values["p95Ms"] is not None and values["p95Ms"] > budget:
            failures.append(f"exceeded:{name}")
    duration = probe.get("loadDurationMs")
    throughput = len(load) * 1000 / duration if finite_number(duration, positive=True) else None
    if phase != "after" and throughput is None:
        failures.append("load_duration_missing_or_invalid")
    if min_throughput and (throughput is None or throughput < min_throughput):
        failures.append("exceeded:minimum_throughput")
    enough_samples = len(load) >= max(40, min_samples) and all(count >= math.ceil(max(40, min_samples) / 2) for count in direction_counts.values())
    evidence_class = "SMOKE_ONLY" if len(load) < 40 else "LOCAL_SAMPLE"
    incomplete = phase == "before"
    status = "FAIL" if failures else ("PRE_RESTART_ONLY" if incomplete else ("RESTART_RECOVERY_ONLY" if phase == "after" else ("INSUFFICIENT_SAMPLES" if not enough_samples else "PASS")))
    return {
        "status": status, "evidenceClass": evidence_class, "productionSloEvidence": False,
        "productionWindowDays": None, "phase": phase, "failures": sorted(set(failures)),
        "limits": {"minLoadSamples": max(40, min_samples), "maxSendRpcP95Ms": max_rpc_p95_ms, "maxDeliveryP95Ms": max_delivery_p95_ms, "minProbeMessagesPerSecond": min_throughput, "source": "local acceptance budgets, not official Telegram metrics"},
        "rpc": {"total": len(rpc), "failures": rpc_failed, "successRate": (len(rpc) - rpc_failed) / len(rpc) if rpc else None, "sendLatency": rpc_latency},
        "delivery": {**latency, "directions": direction_counts},
        "throughput": {"messages": len(load), "durationMs": duration if finite_number(duration) else None, "messagesPerSecond": throughput, "capacityBenchmark": False},
        "limitations": ["Local sequential probe samples do not establish a 30-day production SLO.", "Fresh sessions reuse existing auth keys; no new account authentication is tested.", "Probe throughput includes state verification and is not maximum server capacity."],
    }


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("evidence", type=Path)
    parser.add_argument("--output", type=Path)
    parser.add_argument("--min-samples", type=int, default=40)
    parser.add_argument("--max-rpc-p95-ms", type=float, default=1000)
    parser.add_argument("--max-delivery-p95-ms", type=float, default=2000)
    parser.add_argument("--min-throughput", type=float, default=0)
    args = parser.parse_args()
    if args.min_samples < 1 or not all(finite_number(value) for value in (args.max_rpc_p95_ms, args.max_delivery_p95_ms, args.min_throughput)):
        parser.error("sample count must be positive and budgets must be finite non-negative numbers")
    try:
        probes = extract_probes(json.loads(args.evidence.read_text(encoding="utf-8")))
        results = [evaluate(probe, min_samples=args.min_samples, max_rpc_p95_ms=args.max_rpc_p95_ms,
                            max_delivery_p95_ms=args.max_delivery_p95_ms, min_throughput=args.min_throughput) for probe in probes]
        status = "FAIL" if any(value["status"] == "FAIL" for value in results) else ("PASS" if all(value["status"] == "PASS" for value in results) else "INCOMPLETE")
        report = {"status": status, "source": str(args.evidence), "productionSloEvidence": False, "results": results}
    except (OSError, ValueError, TypeError) as error:
        # Avoid copying raw evidence or its possibly sensitive contents to stderr.
        report = {"status": "FAIL", "error": type(error).__name__, "productionSloEvidence": False}
    rendered = json.dumps(report, indent=2, sort_keys=True, allow_nan=False) + "\n"
    if args.output:
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(rendered, encoding="utf-8")
    print(rendered, end="")
    return 1 if report["status"] == "FAIL" else (0 if report["status"] == "PASS" else 2)


if __name__ == "__main__":
    raise SystemExit(main())
