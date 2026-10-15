#!/usr/bin/env python3
"""Evaluate settled UTC minute evidence over exactly 30 days; missing is not good."""
from __future__ import annotations

import argparse
import datetime as dt
import json
import math
from pathlib import Path

UTC = dt.timezone.utc
MINUTES = 30 * 24 * 60
SCHEMA_VERSION = 2
DEFINITION = "logical-send:ack<=1000ms:live<=2000ms:connection+reconnect+integrity+recovery:v2"
LEGACY_SCHEMA = (1, "logical-send:scheduled-ack<=1000ms:online-live<=2000ms:v1")
MEASUREMENTS = ("connection", "reconnect", "messageIntegrity")


def timestamp(value: str) -> dt.datetime:
    result = dt.datetime.fromisoformat(value.replace("Z", "+00:00"))
    if result.tzinfo is None:
        raise ValueError("timestamps require an explicit timezone")
    return result.astimezone(UTC)


def evaluate(rows: list[dict], *, end: dt.datetime, as_of: dt.datetime,
             environment: str, region: str, source: str, shards: list[str], target: float = .9995,
             minimum_organic_active_minutes: int = 1440, recovery_target: float = .95,
             recovery_budget_seconds: float = 60, minimum_recovery_events: int = 1) -> dict:
    end, as_of = end.astimezone(UTC), as_of.astimezone(UTC)
    if (end.second or end.microsecond or not 0 < target <= 1 or not 0 < recovery_target <= 1
            or not isinstance(recovery_budget_seconds, (int, float)) or isinstance(recovery_budget_seconds, bool)
            or not math.isfinite(recovery_budget_seconds) or recovery_budget_seconds <= 0
            or not isinstance(minimum_recovery_events, int) or isinstance(minimum_recovery_events, bool)
            or minimum_recovery_events < 1 or not shards or len(set(shards)) != len(shards)
            or source not in ("synthetic-layer229", "organic-layer229")
            or not isinstance(minimum_organic_active_minutes, int) or minimum_organic_active_minutes < 1):
        raise ValueError("minute-aligned window, unique shards and target in (0,1] required")
    start = end - dt.timedelta(days=30)
    expected = MINUTES * len(shards)
    buckets: dict[tuple[dt.datetime, str], dict] = {}
    invalid, unsettled, duplicates = [], 0, 0
    incomplete_measurements = {name: 0 for name in MEASUREMENTS}
    active_minutes = {shard: 0 for shard in shards}
    eligible = ack = live = 0
    connection_eligible = connection_good = 0
    reconnect_eligible = reconnect_good = 0
    integrity_eligible = lost = duplicated = 0
    recovery_events: dict[str, float] = {}
    for index, row in enumerate(rows):
        try:
            current_schema = (row.get("schemaVersion"), row.get("definition"))
            if (current_schema not in ((SCHEMA_VERSION, DEFINITION), LEGACY_SCHEMA)
                    or row.get("environment") != environment or row.get("region") != region
                    or row.get("source") != source or row.get("apiLayer") != 229
                    or row.get("shard") not in shards):
                raise ValueError("scope, schema, source or SLI mismatch")
            if row.get("type") == "messaging.slo.minute":
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
                if row.get("settled") is not True or row.get("counterReset") is not False:
                    unsettled += 1
                    if source == "synthetic-layer229":
                        eligible += bucket_eligible
                elif observed < when + dt.timedelta(minutes=1):
                    raise ValueError("settled evidence recorded before bucket end")
                elif row.get("collectorComplete") is not True:
                    unsettled += 1
                    if source == "synthetic-layer229":
                        eligible += bucket_eligible
                else:
                    if bucket_eligible:
                        active_minutes[row["shard"]] += 1
                    eligible += bucket_eligible
                    ack += bucket_ack
                    live += bucket_live

                measurements_settled = (row.get("settled") is True and row.get("counterReset") is False
                                        and row.get("collectorComplete") is True)
                measurements = row.get("measurements") if row["schemaVersion"] == SCHEMA_VERSION else {}
                if not isinstance(measurements, dict):
                    measurements = {}
                for name in MEASUREMENTS:
                    sample = measurements.get(name)
                    if (not measurements_settled or not isinstance(sample, dict)
                            or sample.get("complete") is not True):
                        incomplete_measurements[name] += 1
                        continue
                    fields = {"connection": ("eligible", "good"),
                              "reconnect": ("eligible", "good"),
                              "messageIntegrity": ("eligible", "lost", "duplicates")}[name]
                    values = [sample.get(field) for field in fields]
                    if not all(isinstance(value, int) and not isinstance(value, bool) and value >= 0 for value in values):
                        raise ValueError(f"invalid {name} measurement")
                    if ((values[0] == 0 and not (name == "messageIntegrity" and planned == 0))
                            or (name == "messageIntegrity" and values[1] > values[0])):
                        incomplete_measurements[name] += 1
                        continue
                    if name == "connection":
                        if values[1] > values[0]:
                            raise ValueError("connection successes exceed samples")
                        connection_eligible += values[0]
                        connection_good += values[1]
                    elif name == "reconnect":
                        if values[1] > values[0]:
                            raise ValueError("reconnect successes exceed attempts")
                        reconnect_eligible += values[0]
                        reconnect_good += values[1]
                    else:
                        integrity_eligible += values[0]
                        lost += values[1]
                        duplicated += values[2]
            elif row.get("type") == "messaging.slo.recovery":
                if not start <= timestamp(row["startedAt"]) < end:
                    continue
                started, recovered = timestamp(row["startedAt"]), timestamp(row["recoveredAt"])
                event_id = row.get("eventId")
                if (not isinstance(event_id, str) or not event_id or recovered > as_of
                        or event_id in recovery_events):
                    raise ValueError("future or duplicate recovery event")
                seconds = row.get("recoverySeconds")
                actual = (recovered - started).total_seconds()
                if (not isinstance(seconds, (int, float)) or isinstance(seconds, bool) or seconds < 0
                        or not math.isfinite(seconds)
                        or abs(seconds - actual) > .001
                        or (environment == "production" and not isinstance(row.get("drillId"), str))
                        or (environment == "production" and not row["drillId"].strip())):
                    raise ValueError("invalid recovery duration")
                recovery_events[event_id] = actual
            else:
                raise ValueError("unsupported SLO evidence type")
        except (ValueError, TypeError, KeyError, AttributeError) as error:
            invalid.append({"row": index, "reason": str(error)})
    missing = expected - len(buckets)
    organic_activity_sufficient = (not source.startswith("organic-")
                                   or all(value >= minimum_organic_active_minutes for value in active_minutes.values()))
    complete = (not invalid and not missing and not unsettled and not any(incomplete_measurements.values())
                and end <= as_of)
    known_window_eligible = 2 * expected if source == "synthetic-layer229" else None
    recovery_values = sorted(recovery_events.values())
    recovery_p95 = recovery_values[max(0, math.ceil(len(recovery_values) * .95) - 1)] if recovery_values else None
    recovery_good = sum(value <= recovery_budget_seconds for value in recovery_values)
    slis = {
        "ack": {"eligible": eligible, "good": ack, "ratio": ack / eligible if eligible else None, "target": target},
        "live": {"eligible": eligible, "good": live, "ratio": live / eligible if eligible else None, "target": target},
        "connection": {"eligible": connection_eligible, "good": connection_good,
                       "ratio": connection_good / connection_eligible if connection_eligible else None, "target": target},
        "reconnect": {"eligible": reconnect_eligible, "good": reconnect_good,
                      "ratio": reconnect_good / reconnect_eligible if reconnect_eligible else None, "target": target},
        "messageLoss": {"eligible": integrity_eligible, "errors": lost, "ratio": 1 - lost / integrity_eligible if integrity_eligible else None, "target": 1.0},
        "messageDuplicates": {"eligible": integrity_eligible, "errors": duplicated,
                              "ratio": 1 - duplicated / integrity_eligible if integrity_eligible else None, "target": 1.0},
        "recovery": {"events": len(recovery_values), "good": recovery_good,
                     "ratio": recovery_good / len(recovery_values) if recovery_values else None,
                     "target": recovery_target, "budgetSeconds": recovery_budget_seconds,
                     "p95Seconds": recovery_p95, "maxSeconds": max(recovery_values) if recovery_values else None},
    }
    known_message_errors = lost > 0 or duplicated > 0
    recovery_exceeded = bool(recovery_values) and recovery_good / len(recovery_values) < recovery_target
    known_ack_budget_exceeded = (known_window_eligible is not None
                                 and max(eligible-ack, eligible-live) > known_window_eligible * (1-target) + 1e-9)
    budget_exceeded = known_ack_budget_exceeded or known_message_errors or recovery_exceeded
    observed_ratio_exceeded = eligible > 0 and (ack / eligible < target or live / eligible < target)
    ratio_exceeded = complete and any(
        item["ratio"] is not None and item["ratio"] < item["target"]
        for key, item in slis.items() if key != "recovery"
    )
    exceeded = budget_exceeded or ratio_exceeded
    recovery_sufficient = len(recovery_values) >= minimum_recovery_events
    status = ("INVALID" if invalid else "FAIL" if exceeded else "NO_TRAFFIC" if complete and not eligible
              else "INCOMPLETE" if not complete or not recovery_sufficient
              else "INCOMPLETE" if not organic_activity_sufficient else "PASS")
    return {"status": status, "productionSloAccepted": False,
            "scope": {"environment": environment, "region": region, "source": source, "apiLayer": 229,
                      "definition": DEFINITION, "shards": shards},
            "windowStart": start.isoformat(), "windowEnd": end.isoformat(), "asOf": as_of.isoformat(),
            "coverage": {"complete": complete, "expectedBuckets": expected, "observedBuckets": len(buckets),
                         "missingBuckets": missing, "unsettledBuckets": unsettled, "duplicateBuckets": duplicates,
                         "incompleteMeasurements": incomplete_measurements,
                         "activeMinutesByShard": active_minutes, "minimumOrganicActiveMinutes": minimum_organic_active_minutes,
                         "sufficientActivity": organic_activity_sufficient,
                         "recoveryEvents": len(recovery_values), "minimumRecoveryEvents": minimum_recovery_events,
                         "recoveryEvidenceSufficient": recovery_sufficient},
            "target": target, "eligible": eligible, "ackGood": ack, "liveGood": live,
            "ackRatio": ack / eligible if eligible else None, "liveRatio": live / eligible if eligible else None,
            "observedRatioBelowTarget": observed_ratio_exceeded,
            "measurements": {"connection": {"eligible": connection_eligible, "good": connection_good},
                             "reconnect": {"eligible": reconnect_eligible, "good": reconnect_good},
                             "messageIntegrity": {"eligible": integrity_eligible, "lost": lost, "duplicates": duplicated}},
            "slis": slis, "wholeWindowErrorBudgetExceeded": exceeded,
            "knownWindowEligible": known_window_eligible, "invalid": invalid,
            "interpretation": "weighted settled Layer 229 evidence; connection and reconnect use sampled attempts, recovery uses recorded incident or drill durations; synthetic and organic evidence remain separate"}


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("evidence", type=Path, help="append-only minute JSONL evidence")
    parser.add_argument("--end", help="exclusive UTC minute; defaults to current completed minute")
    parser.add_argument("--environment", default="local-isolated")
    parser.add_argument("--region", default="local")
    parser.add_argument("--source", default="synthetic-layer229")
    parser.add_argument("--shards", nargs="+", default=["pair-1"])
    parser.add_argument("--target", type=float, default=.9995)
    parser.add_argument("--recovery-target", type=float, default=.95)
    parser.add_argument("--recovery-budget-seconds", type=float, default=60)
    parser.add_argument("--minimum-recovery-events", type=int, default=1)
    parser.add_argument("--minimum-organic-active-minutes", type=int, default=1440)
    parser.add_argument("--output", type=Path, default=Path("/tmp/teamgram-messaging-30day-slo.json"))
    args = parser.parse_args()
    try:
        rows = [json.loads(line) for line in args.evidence.read_text().splitlines() if line.strip()]
        now = dt.datetime.now(UTC)
        end = timestamp(args.end) if args.end else now.replace(second=0, microsecond=0)
        report = evaluate(rows, end=end, as_of=now, environment=args.environment, region=args.region,
                          source=args.source, shards=args.shards, target=args.target,
                          minimum_organic_active_minutes=args.minimum_organic_active_minutes,
                          recovery_target=args.recovery_target,
                          recovery_budget_seconds=args.recovery_budget_seconds,
                          minimum_recovery_events=args.minimum_recovery_events)
    except (OSError, ValueError, TypeError) as error:
        report = {"status": "INVALID", "reason": str(error), "productionSloAccepted": False}
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(report, indent=2, sort_keys=True) + "\n")
    print(json.dumps({"status": report["status"], "output": str(args.output), "coverage": report.get("coverage")}))
    return 0 if report["status"] == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
