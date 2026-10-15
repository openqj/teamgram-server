#!/usr/bin/env python3
"""Append one measured recovery incident or drill to the SLO evidence log."""
from __future__ import annotations

import argparse
import datetime as dt
import fcntl
import json
import os
from pathlib import Path

from messaging_slo_window import DEFINITION, SCHEMA_VERSION, timestamp


def make_event(*, event_id: str, environment: str, region: str, source: str, shard: str,
               started_at: str, recovered_at: str, drill_id: str) -> dict:
    started, recovered = timestamp(started_at), timestamp(recovered_at)
    seconds = (recovered - started).total_seconds()
    if (not event_id.strip() or not environment or not region or not shard
            or source not in ("synthetic-layer229", "organic-layer229")
            or recovered < started or (environment == "production" and not drill_id.strip())):
        raise ValueError("recovery event scope, ID, timestamps or production drill ID are invalid")
    return {"schemaVersion": SCHEMA_VERSION, "type": "messaging.slo.recovery", "environment": environment,
            "region": region, "source": source, "apiLayer": 229, "definition": DEFINITION, "shard": shard,
            "eventId": event_id, "startedAt": started.isoformat(), "recoveredAt": recovered.isoformat(),
            "recoverySeconds": seconds, "drillId": drill_id}


def append_event(path: Path, event: dict) -> bool:
    path.parent.mkdir(parents=True, exist_ok=True)
    descriptor = os.open(path, os.O_RDWR | os.O_APPEND | os.O_CREAT, 0o600)
    os.fchmod(descriptor, 0o600)
    with os.fdopen(descriptor, "r+", encoding="utf-8") as stream:
        fcntl.flock(stream.fileno(), fcntl.LOCK_EX)
        stream.seek(0)
        for line in stream:
            if not line.strip():
                continue
            existing = json.loads(line)
            if not isinstance(existing, dict):
                raise ValueError("evidence log contains a non-object row")
            if (existing.get("type") == event["type"] and existing.get("environment") == event["environment"]
                    and existing.get("region") == event["region"] and existing.get("source") == event["source"]
                    and existing.get("eventId") == event["eventId"]):
                if existing == event:
                    return False
                raise ValueError("recovery event ID already exists with different evidence")
        stream.seek(0, os.SEEK_END)
        stream.write(json.dumps(event, sort_keys=True) + "\n")
        stream.flush()
        os.fsync(stream.fileno())
        directory = os.open(path.parent, os.O_RDONLY)
        try:
            os.fsync(directory)
        finally:
            os.close(directory)
        return True


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("evidence", type=Path, help="append-only minute and recovery-event JSONL")
    parser.add_argument("--event-id", required=True)
    parser.add_argument("--environment", default="local-isolated")
    parser.add_argument("--region", default="local")
    parser.add_argument("--source", default="synthetic-layer229")
    parser.add_argument("--shard", default="pair-1")
    parser.add_argument("--started-at", required=True, help="UTC timestamp when injection or incident began")
    parser.add_argument("--recovered-at", required=True, help="UTC timestamp when service and message checks recovered")
    parser.add_argument("--drill-id", default="", help="required production incident or drill identifier")
    args = parser.parse_args()
    try:
        event = make_event(event_id=args.event_id, environment=args.environment, region=args.region,
                           source=args.source, shard=args.shard, started_at=args.started_at,
                           recovered_at=args.recovered_at, drill_id=args.drill_id)
        inserted = append_event(args.evidence, event)
    except (OSError, ValueError, TypeError, json.JSONDecodeError) as error:
        print(f"SLO recovery event rejected: {type(error).__name__}")
        return 1
    print(json.dumps({"recorded": inserted, "eventId": event["eventId"], "recoverySeconds": event["recoverySeconds"]}))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
