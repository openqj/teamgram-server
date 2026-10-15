#!/usr/bin/env python3
"""Collect one scheduled production Layer 229 minute canary."""
from __future__ import annotations

import argparse
import datetime as dt
import fcntl
import hashlib
import json
import math
import os
from pathlib import Path
import secrets
import subprocess
import time

from messaging_slo_metrics import write_metrics
from messaging_slo_window import DEFINITION, SCHEMA_VERSION, UTC


ROOT = Path(__file__).resolve().parents[1]
PROBE = ROOT / "docker/production/probe-cross-account-live-updates.ts"
CONNECTION_HOLD_MS = 10_000
PROBE_TIMEOUT_SECONDS = 45


def minute_start(now: dt.datetime) -> dt.datetime:
    value = now.astimezone(UTC)
    return value.replace(second=0, microsecond=0)


def _count(value: object, name: str) -> int:
    if not isinstance(value, int) or isinstance(value, bool) or value < 0:
        raise ValueError(f"invalid {name}")
    return value


def _duration(value: object, name: str) -> float:
    if not isinstance(value, (int, float)) or isinstance(value, bool):
        raise ValueError(f"invalid {name}")
    result = float(value)
    if not math.isfinite(result) or result < 0:
        raise ValueError(f"invalid {name}")
    return result


def _fsync_directory(path: Path) -> None:
    descriptor = os.open(path, os.O_RDONLY)
    try:
        os.fsync(descriptor)
    finally:
        os.close(descriptor)


def _write_all(descriptor: int, payload: bytes) -> None:
    offset = 0
    while offset < len(payload):
        written = os.write(descriptor, payload[offset:])
        if written <= 0:
            raise OSError("short evidence write")
        offset += written


def parse_probe_output(stdout: str, expected_tag: str) -> dict:
    for line in reversed(stdout.splitlines()):
        try:
            value = json.loads(line)
        except json.JSONDecodeError:
            continue
        if (isinstance(value, dict)
                and value.get("probe") == "messaging.cross-account-live-updates"
                and value.get("phase") == "slo" and value.get("tag") == expected_tag):
            return value
    raise ValueError("matching structured canary evidence was not returned")


def measurement_row(report: dict) -> dict:
    deliveries = report.get("deliverySamples")
    connections = report.get("connectionObservation")
    reconnects = report.get("reconnectSample")
    integrity = report.get("messageIntegrity")
    checks = report.get("checks")
    cleanup = report.get("cleanup")
    if (not isinstance(deliveries, list) or len(deliveries) != 2
            or not isinstance(connections, dict) or not isinstance(reconnects, dict)
            or not isinstance(integrity, dict) or not isinstance(checks, dict)
            or not isinstance(cleanup, dict)):
        raise ValueError("canary measurement groups are incomplete")

    directions = set()
    ack_good = live_good = 0
    for sample in deliveries:
        if not isinstance(sample, dict):
            raise ValueError("invalid delivery sample")
        direction = sample.get("direction")
        if direction not in ("A-to-B", "B-to-A") or direction in directions:
            raise ValueError("delivery directions are incomplete")
        directions.add(direction)
        if "rpcMs" in sample:
            ack_good += int(_duration(sample.get("rpcMs"), "rpcMs") <= 1000)
        if "deliveryMs" in sample:
            live_good += int(_duration(sample.get("deliveryMs"), "deliveryMs") <= 2000)

    connection_eligible = _count(connections.get("eligible"), "connection eligible")
    connection_good = _count(connections.get("good"), "connection good")
    reconnect_eligible = _count(reconnects.get("eligible"), "reconnect eligible")
    reconnect_good = _count(reconnects.get("good"), "reconnect good")
    integrity_eligible = _count(integrity.get("eligible"), "integrity eligible")
    lost = _count(integrity.get("lost"), "lost")
    duplicates = _count(integrity.get("duplicates"), "duplicates")
    if (connection_eligible < 2 or connection_good > connection_eligible
            or reconnect_eligible != 2 or reconnect_good > reconnect_eligible
            or integrity_eligible != 2 or lost > integrity_eligible):
        raise ValueError("canary measurement counts are inconsistent")

    complete = (report.get("collectionComplete") is True and checks.get("sloCanary") is True
                and cleanup.get("ok") is True
                and cleanup.get("remaining") == 0 and directions == {"A-to-B", "B-to-A"}
                and integrity.get("complete") is True)
    return {
        "collectorComplete": complete,
        "ackGood": ack_good,
        "liveGood": live_good,
        "measurements": {
            "connection": {"complete": complete, "eligible": connection_eligible,
                           "good": connection_good},
            "reconnect": {"complete": complete, "eligible": reconnect_eligible,
                          "good": reconnect_good},
            "messageIntegrity": {"complete": complete, "eligible": integrity_eligible,
                                 "lost": lost, "duplicates": duplicates},
        },
    }


def collect_minute(*, evidence: Path, metrics: Path, raw_dir: Path, region: str, shard: str,
                   probe: Path, gramjs_dir: Path, environment: dict[str, str],
                   now_fn=time.time, sleep_fn=time.sleep, run=subprocess.run,
                   timeout_seconds: int = PROBE_TIMEOUT_SECONDS) -> dict:
    required = ("A_AUTH_B64", "B_AUTH_B64", "A_USER_ID", "B_USER_ID", "A_ACCESS_HASH",
                "B_ACCESS_HASH", "TG_DC_HOST", "TG_DC_PORT")
    if not region or not shard or any(not environment.get(name) for name in required):
        raise ValueError("production canary scope and credentials are required")
    now = dt.datetime.fromtimestamp(now_fn(), UTC)
    bucket = minute_start(now)
    tag = f"slo-{bucket.strftime('%Y%m%dT%H%M')}-{secrets.token_hex(6)}"
    row = {
        "schemaVersion": SCHEMA_VERSION, "type": "messaging.slo.minute",
        "environment": "production", "region": region, "source": "synthetic-layer229",
        "apiLayer": 229, "definition": DEFINITION, "shard": shard,
        "bucketStart": bucket.isoformat(), "bucketSeconds": 60,
        "planned": 2, "eligible": 2, "ackGood": 0, "liveGood": 0,
        "counterReset": False, "collectorComplete": False,
        "measurements": {"connection": {"complete": False}, "reconnect": {"complete": False},
                         "messageIntegrity": {"complete": False}},
    }

    evidence.parent.mkdir(parents=True, exist_ok=True)
    evidence_created = not evidence.exists()
    descriptor = os.open(evidence, os.O_RDWR | os.O_CREAT, 0o600)
    try:
        os.fchmod(descriptor, 0o600)
        fcntl.flock(descriptor, fcntl.LOCK_EX)
        with os.fdopen(os.dup(descriptor), "r", encoding="utf-8") as reader:
            for line in reader:
                if not line.strip():
                    continue
                prior = json.loads(line)
                if (prior.get("type") == row["type"] and prior.get("environment") == row["environment"]
                        and prior.get("region") == region and prior.get("source") == row["source"]
                        and prior.get("shard") == shard and prior.get("bucketStart") == bucket.isoformat()):
                    raise ValueError("minute bucket already has evidence")

        lateness = now.timestamp() - bucket.timestamp()
        if lateness <= 5:
            raw_dir.mkdir(parents=True, exist_ok=True, mode=0o700)
            tag_path = raw_dir / f"{tag}.json"
            command = [str(gramjs_dir / "node_modules/.bin/tsx"), str(probe)]
            child_env = dict(environment)
            child_env.update({"PROBE_PHASE": "slo", "PROBE_SLO_METRICS": "1",
                              "SLO_CONNECTION_HOLD_MS": str(CONNECTION_HOLD_MS), "PROBE_TAG": tag,
                              "LIVE_UPDATE_TIMEOUT_MS": "8000"})
            try:
                result = run(command, env=child_env, capture_output=True, text=True,
                             timeout=timeout_seconds, check=False)
                report = parse_probe_output(result.stdout, tag)
                raw = json.dumps(report, sort_keys=True, separators=(",", ":")) + "\n"
                raw_bytes = raw.encode()
                raw_fd = os.open(tag_path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
                with os.fdopen(raw_fd, "wb") as output:
                    output.write(raw_bytes)
                    output.flush()
                    os.fsync(output.fileno())
                _fsync_directory(raw_dir)
                row["rawEvidence"] = str(tag_path)
                row["rawSha256"] = hashlib.sha256(raw_bytes).hexdigest()
                measured = measurement_row(report)
                row.update(measured)
                if result.returncode != 0 and not row["collectorComplete"]:
                    row["collectorError"] = "CANARY_FAILED"
            except (OSError, subprocess.SubprocessError, ValueError, json.JSONDecodeError):
                row["collectorError"] = "CANARY_EVIDENCE_UNAVAILABLE"
        else:
            row["collectorError"] = "COLLECTOR_MISSED_SCHEDULE"

        remaining = bucket.timestamp() + 60 - now_fn()
        if remaining > 0:
            sleep_fn(remaining)
        row.update(settled=True, observedAt=dt.datetime.fromtimestamp(now_fn(), UTC).isoformat())
        payload = (json.dumps(row, sort_keys=True, separators=(",", ":")) + "\n").encode()
        os.lseek(descriptor, 0, os.SEEK_END)
        _write_all(descriptor, payload)
        os.fsync(descriptor)
        if evidence_created:
            _fsync_directory(evidence.parent)
        fcntl.flock(descriptor, fcntl.LOCK_UN)
    finally:
        os.close(descriptor)

    write_metrics(evidence, metrics)
    return row


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--evidence", type=Path, default=Path("/var/lib/teamgram/slo/production.jsonl"))
    parser.add_argument("--metrics", type=Path,
                        default=Path("/var/lib/node_exporter/textfile_collector/teamgram_slo.prom"))
    parser.add_argument("--raw-dir", type=Path, default=Path("/var/lib/teamgram/slo/raw"))
    parser.add_argument("--probe", type=Path, default=PROBE)
    parser.add_argument("--gramjs-dir", type=Path,
                        default=Path(os.environ.get("TEAMGRAM_SLO_GRAMJS_DIR", ROOT.parent / "telegram-tt-master")))
    parser.add_argument("--region", default=os.environ.get("TEAMGRAM_SLO_REGION", ""))
    parser.add_argument("--shard", default=os.environ.get("TEAMGRAM_SLO_SHARD", ""))
    parser.add_argument("--timeout", type=int, default=PROBE_TIMEOUT_SECONDS)
    args = parser.parse_args()
    try:
        row = collect_minute(evidence=args.evidence, metrics=args.metrics, raw_dir=args.raw_dir,
                             region=args.region, shard=args.shard, probe=args.probe,
                             gramjs_dir=args.gramjs_dir, environment=os.environ,
                             timeout_seconds=args.timeout)
    except (OSError, ValueError) as error:
        print(f"Production SLO collection failed: {type(error).__name__}")
        return 1
    print(json.dumps({"bucketStart": row["bucketStart"], "collectorComplete": row["collectorComplete"],
                      "error": row.get("collectorError")}))
    return 0 if row["collectorComplete"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
