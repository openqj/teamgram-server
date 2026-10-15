#!/usr/bin/env python3
"""Evaluate settled UTC minute evidence over exactly 30 days; missing is not good."""
from __future__ import annotations

import argparse
import datetime as dt
import json
from pathlib import Path

UTC = dt.timezone.utc
MINUTES = 30 * 24 * 60
DEFINITION = "logical-send:scheduled-ack<=1000ms:online-live<=2000ms:v1"


def timestamp(value: str) -> dt.datetime:
    result = dt.datetime.fromisoformat(value.replace("Z", "+00:00"))
    if result.tzinfo is None:
        raise ValueError("timestamps require an explicit timezone")
    return result.astimezone(UTC)


def evaluate(rows: list[dict], *, end: dt.datetime, as_of: dt.datetime,
             environment: str, region: str, source: str, shards: list[str], target: float = .9995,
             minimum_organic_active_minutes: int = 1440) -> dict:
    end, as_of = end.astimezone(UTC), as_of.astimezone(UTC)
    if (end.second or end.microsecond or not 0 < target <= 1 or not shards or len(set(shards)) != len(shards)
            or source not in ("synthetic-layer229", "organic-layer229")
            or not isinstance(minimum_organic_active_minutes, int) or minimum_organic_active_minutes < 1):
        raise ValueError("minute-aligned window, unique shards and target in (0,1] required")
    start = end - dt.timedelta(days=30)
    expected = MINUTES * len(shards)
    buckets: dict[tuple[dt.datetime, str], dict] = {}
    invalid, unsettled, duplicates = [], 0, 0
    active_minutes = {shard: 0 for shard in shards}
    eligible = ack = live = 0
    for index, row in enumerate(rows):
        try:
            if (row.get("schemaVersion") != 1 or row.get("type") != "messaging.slo.minute"
                    or row.get("environment") != environment or row.get("region") != region
                    or row.get("source") != source or row.get("apiLayer") != 229
                    or row.get("definition") != DEFINITION or row.get("shard") not in shards):
                raise ValueError("scope, schema, source or SLI mismatch")
            when, observed = timestamp(row["bucketStart"]), timestamp(row["observedAt"])
            if when.second or when.microsecond or row.get("bucketSeconds") != 60:
                raise ValueError("overlapping or non-minute bucket")
            if when > as_of or observed > as_of:
                raise ValueError("future evidence")
            if not start <= when < end:
                continue
            key = (when, row["shard"])
            if key in buckets:
                duplicates += 1
                raise ValueError("duplicate bucket; collector must emit one immutable record per minute and shard")
            buckets[key] = row
            counts = [row.get(name) for name in ("planned", "eligible", "ackGood", "liveGood")]
            if (not all(isinstance(value, int) and not isinstance(value, bool) and value >= 0 for value in counts)
                    or counts[1] > counts[0] or counts[2] > counts[1] or counts[3] > counts[1]):
                raise ValueError("invalid logical-message counts")
            planned, bucket_eligible, bucket_ack, bucket_live = counts
            if source.startswith("synthetic-") and (planned != 2 or bucket_eligible != planned):
                raise ValueError("synthetic bucket must fulfill every planned canary message")
            if (row.get("settled") is not True or row.get("collectorComplete") is not True
                    or row.get("counterReset") is not False):
                unsettled += 1
                if source == "synthetic-layer229":
                    # Scheduled probes still belong to the denominator; an
                    # incomplete collector provides no proof of good results.
                    eligible += bucket_eligible
                continue
            if observed < when + dt.timedelta(minutes=1):
                raise ValueError("settled evidence recorded before bucket end")
            if bucket_eligible:
                active_minutes[row["shard"]] += 1
            eligible += bucket_eligible
            ack += bucket_ack
            live += bucket_live
        except (ValueError, TypeError, KeyError, AttributeError) as error:
            invalid.append({"row": index, "reason": str(error)})
    missing = expected - len(buckets)
    organic_activity_sufficient = (not source.startswith("organic-")
                                   or all(value >= minimum_organic_active_minutes for value in active_minutes.values()))
    complete = not invalid and not missing and not unsettled and end <= as_of
    observed_ratio_exceeded = eligible > 0 and (ack / eligible < target or live / eligible < target)
    known_window_eligible = 2 * expected if source == "synthetic-layer229" else None
    # One early failure does not exhaust a 30-day budget. With a fixed canary
    # schedule, already observed bad messages can exceed the whole-window
    # budget even before all minutes arrive. Organic future traffic is unknown.
    budget_exhausted = (known_window_eligible is not None
                       and max(eligible-ack, eligible-live) > known_window_eligible * (1-target) + 1e-9)
    exceeded = budget_exhausted or (complete and observed_ratio_exceeded)
    status = ("INVALID" if invalid else "FAIL" if exceeded else "INCOMPLETE" if not complete
              else "NO_TRAFFIC" if not eligible else "INCOMPLETE" if not organic_activity_sufficient else "PASS")
    return {"status": status, "productionSloAccepted": False,
            "scope": {"environment": environment, "region": region, "source": source, "apiLayer": 229,
                      "definition": DEFINITION, "shards": shards},
            "windowStart": start.isoformat(), "windowEnd": end.isoformat(), "asOf": as_of.isoformat(),
            "coverage": {"complete": complete, "expectedBuckets": expected, "observedBuckets": len(buckets),
                         "missingBuckets": missing, "unsettledBuckets": unsettled, "duplicateBuckets": duplicates,
                         "activeMinutesByShard": active_minutes, "minimumOrganicActiveMinutes": minimum_organic_active_minutes,
                         "sufficientActivity": organic_activity_sufficient},
            "target": target, "eligible": eligible, "ackGood": ack, "liveGood": live,
            "ackRatio": ack / eligible if eligible else None, "liveRatio": live / eligible if eligible else None,
            "observedRatioBelowTarget": observed_ratio_exceeded, "wholeWindowErrorBudgetExceeded": exceeded,
            "knownWindowEligible": known_window_eligible, "invalid": invalid,
            "interpretation": "weighted logical messages in settled observed buckets; synthetic and organic evidence remain separate"}


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("evidence", type=Path, help="append-only minute JSONL evidence")
    parser.add_argument("--end", help="exclusive UTC minute; defaults to current completed minute")
    parser.add_argument("--environment", default="local-isolated")
    parser.add_argument("--region", default="local")
    parser.add_argument("--source", default="synthetic-layer229")
    parser.add_argument("--shards", nargs="+", default=["pair-1"])
    parser.add_argument("--target", type=float, default=.9995)
    parser.add_argument("--minimum-organic-active-minutes", type=int, default=1440)
    parser.add_argument("--output", type=Path, default=Path("/tmp/teamgram-messaging-30day-slo.json"))
    args = parser.parse_args()
    try:
        rows = [json.loads(line) for line in args.evidence.read_text().splitlines() if line.strip()]
        now = dt.datetime.now(UTC)
        end = timestamp(args.end) if args.end else now.replace(second=0, microsecond=0)
        report = evaluate(rows, end=end, as_of=now, environment=args.environment, region=args.region,
                          source=args.source, shards=args.shards, target=args.target,
                          minimum_organic_active_minutes=args.minimum_organic_active_minutes)
    except (OSError, ValueError, TypeError) as error:
        report = {"status": "INVALID", "reason": str(error), "productionSloAccepted": False}
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(report, indent=2, sort_keys=True) + "\n")
    print(json.dumps({"status": report["status"], "output": str(args.output), "coverage": report.get("coverage")}))
    return 0 if report["status"] == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
