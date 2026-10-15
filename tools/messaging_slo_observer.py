#!/usr/bin/env python3
"""Collect persistent synthetic minute evidence in one disposable local lab."""
from __future__ import annotations

import argparse
import datetime as dt
import hashlib
import json
import os
from pathlib import Path
import secrets
import time

from messaging_capacity_probe import ROOT, evaluate, run_profile, start_lab
from messaging_reliability_probe import validate_ports
from messaging_slo_window import DEFINITION, UTC
from postgres_recovery_probe import Lab, ProbeError, redact


def wait_until(timestamp: float) -> None:
    while time.time() < timestamp:
        time.sleep(max(0, min(1, timestamp - time.time())))


def observe(lab: Lab, args: argparse.Namespace, report: dict) -> None:
    gramjs = start_lab(lab, args, report)
    first = (int(time.time()) // 60 + 1) * 60
    report["first_bucket"] = dt.datetime.fromtimestamp(first, UTC).isoformat()
    args.evidence.parent.mkdir(parents=True, exist_ok=True)
    raw_dir = args.evidence.parent / f"{lab.run_id}-slo-raw"
    raw_dir.mkdir()
    report["raw_evidence_directory"] = str(raw_dir)
    report["buckets_written"] = 0
    for minute in range(args.minutes):
        bucket_start = first + minute * 60
        wait_until(bucket_start)
        lateness = time.time() - bucket_start
        row = {"schemaVersion": 1, "type": "messaging.slo.minute", "environment": "local-isolated", "region": "local",
               "source": "synthetic-layer229", "apiLayer": 229, "definition": DEFINITION, "shard": "pair-1",
               "bucketStart": dt.datetime.fromtimestamp(bucket_start, UTC).isoformat(), "bucketSeconds": 60,
               "planned": 2, "eligible": 2, "ackGood": 0, "liveGood": 0, "counterReset": False,
               "collectorComplete": False, "run": lab.run_id, "startLatenessSeconds": round(lateness, 3)}
        if lateness <= 5:
            try:
                evidence, pending = run_profile(lab, gramjs, 1, 2, 32, raw_dir / "unused-state.json", args.metrics_port)
                raw = json.dumps(evidence, sort_keys=True) + "\n"
                raw_path = raw_dir / f"{minute:06d}.json"
                raw_path.write_text(raw)
                summary = evaluate(evidence)
                row.update(rawEvidence=str(raw_path), rawSha256=hashlib.sha256(raw.encode()).hexdigest(),
                           pendingAfterDrain=pending, collectorComplete=True)
                # Consistency failure is a failed logical message, not a retry
                # that can replace the original outcome or enlarge its count.
                if summary["consistency"]:
                    row.update(ackGood=summary["counts"]["ack_on_time"], liveGood=summary["counts"]["live_on_time"])
            except (ProbeError, OSError, ValueError):
                row["collectorError"] = "CANARY_EVIDENCE_UNAVAILABLE"
        else:
            row["collectorError"] = "COLLECTOR_MISSED_SCHEDULE"
        wait_until(bucket_start + 60)
        row.update(settled=True, observedAt=dt.datetime.now(UTC).isoformat())
        # O_EXCL prevents two runs from appending conflicting evidence. Each
        # settled record is fsynced; interruption leaves absent minutes visible.
        with args.evidence.open("a") as output:
            output.write(json.dumps(row, sort_keys=True) + "\n")
            output.flush()
            os.fsync(output.fileno())
        report["buckets_written"] += 1
    report["status"] = "COLLECTED"


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--execute", action="store_true")
    parser.add_argument("--minutes", type=int, default=10)
    parser.add_argument("--image", default="teamgram-reliability:20261015")
    parser.add_argument("--gramjs-dir", type=Path, default=ROOT.parent / "telegram-tt-master")
    parser.add_argument("--evidence", type=Path, default=Path("/tmp/teamgram-messaging-slo-minutes.jsonl"))
    parser.add_argument("--output", type=Path, default=Path("/tmp/teamgram-messaging-observation.json"))
    parser.add_argument("--postgres-port", type=int, default=55440)
    parser.add_argument("--mt-port", type=int, default=21545)
    parser.add_argument("--kafka-port", type=int, default=19096)
    parser.add_argument("--metrics-port", type=int, default=19092)
    args = parser.parse_args()
    report = {"status": "PLAN", "productionSloAccepted": False, "source": "synthetic-layer229", "minutes": args.minutes,
              "evidence": str(args.evidence), "started_at": dt.datetime.now(UTC).isoformat()}
    lab = None
    try:
        if not 1 <= args.minutes <= 43200:
            raise ProbeError("minutes must be 1..43200")
        validate_ports([args.postgres_port, args.mt_port, args.kafka_port, args.metrics_port])
        lab = Lab("teamgram-reliability-slo-" + secrets.token_hex(6), port=args.postgres_port, timeout=300, wait=150,
                  profiles=["messaging", "backend", "monitoring"], env={
                      "TEAMGRAM_RELIABILITY_IMAGE": args.image, "TEAMGRAM_RELIABILITY_MT_PORT": str(args.mt_port),
                      "TEAMGRAM_RELIABILITY_KAFKA_PORT": str(args.kafka_port), "TEAMGRAM_RELIABILITY_METRICS_PORT": str(args.metrics_port)})
        report["project"] = lab.project
        if args.execute:
            args.evidence.parent.mkdir(parents=True, exist_ok=True)
            with args.evidence.open("x"):
                pass
            observe(lab, args, report)
    except (ProbeError, OSError, ValueError, KeyboardInterrupt) as error:
        report.update(status="FAIL", reason=redact(str(error), lab.secret_values if lab else []))
    finally:
        if lab and args.execute:
            try:
                report["cleanup"] = lab.cleanup()
            except (ProbeError, OSError, ValueError) as error:
                report.update(status="FAIL", cleanup={"status": "FAIL", "reason": redact(str(error), lab.secret_values)})
        report["finished_at"] = dt.datetime.now(UTC).isoformat()
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(json.dumps(report, indent=2, sort_keys=True) + "\n")
    print(json.dumps({"status": report["status"], "output": str(args.output), "bucketsWritten": report.get("buckets_written"), "reason": report.get("reason")}))
    return 0 if report["status"] in ("PLAN", "COLLECTED") else 1


if __name__ == "__main__":
    raise SystemExit(main())
