#!/usr/bin/env python3
"""Rebuild SLO counters from append-only minute evidence for a textfile collector."""
from __future__ import annotations

import argparse
from collections import defaultdict
import datetime as dt
import json
import math
import os
from pathlib import Path
import tempfile

from messaging_slo_window import DEFINITION, LEGACY_SCHEMA, SCHEMA_VERSION, UTC, timestamp


METRICS = (
    ("teamgram_slo_ack_eligible_total", "Scheduled logical messages eligible for ACK SLO measurement."),
    ("teamgram_slo_ack_good_total", "Eligible logical messages acknowledged within the SLO budget."),
    ("teamgram_slo_live_eligible_total", "Scheduled logical messages eligible for live-delivery SLO measurement."),
    ("teamgram_slo_live_good_total", "Eligible logical messages delivered live within the SLO budget."),
    ("teamgram_slo_connection_eligible_total", "Sampled client connections eligible for retention measurement."),
    ("teamgram_slo_connection_good_total", "Sampled client connections held for the observation interval."),
    ("teamgram_slo_reconnect_eligible_total", "Synthetic or observed reconnect attempts eligible for measurement."),
    ("teamgram_slo_reconnect_good_total", "Reconnect attempts that authenticated and resumed successfully."),
    ("teamgram_slo_integrity_eligible_total", "Logical messages checked for loss and duplication."),
    ("teamgram_slo_message_loss_total", "Eligible logical messages missing from recipient history."),
    ("teamgram_slo_message_duplicates_total", "Extra copies found in recipient history or live updates."),
    ("teamgram_slo_recovery_events_total", "Recorded incident or drill recovery events."),
    ("teamgram_slo_recovery_seconds_sum", "Total measured incident or drill recovery time in seconds."),
    ("teamgram_slo_recovery_seconds_count", "Count of measured incident or drill recovery durations."),
)

GAUGES = (("teamgram_slo_evidence_latest_timestamp_seconds", "Timestamp of the newest durable SLO evidence row."),)

RECOVERY_BUCKETS = (1, 5, 10, 30, 60, 120, 300)


def _counter(value: object, name: str) -> int:
    if not isinstance(value, int) or isinstance(value, bool) or value < 0:
        raise ValueError(f"invalid {name}")
    return value


def _labels(keys: tuple[str, ...], values: tuple[str, ...]) -> str:
    return ",".join(
        f'{key}="{value.replace(chr(92), chr(92) * 2).replace(chr(34), chr(92) + chr(34)).replace(chr(10), chr(92) + "n")}"'
        for key, value in zip(keys, values)
    )


def render_metrics(rows: list[dict]) -> str:
    totals: dict[tuple[str, str, str], dict[str, int]] = defaultdict(lambda: defaultdict(int))
    latest_completeness: dict[tuple[str, str, str, str, str], tuple[dt.datetime, int]] = {}
    recovery_buckets: dict[tuple[tuple[str, str, str], int | str], int] = defaultdict(int)
    latest_evidence: dict[tuple[str, str, str], float] = {}
    seen_minutes: set[tuple[str, str, str, str, str]] = set()
    seen_recovery: set[tuple[str, str, str, str, str]] = set()
    for row in rows:
        row_schema = (row.get("schemaVersion"), row.get("definition"))
        if (row_schema not in ((SCHEMA_VERSION, DEFINITION), LEGACY_SCHEMA)
                or row.get("apiLayer") != 229):
            raise ValueError("unsupported SLO evidence schema or definition")
        labels = tuple(row.get(key) for key in ("environment", "region", "source"))
        shard = row.get("shard")
        if not all(isinstance(value, str) and value for value in (*labels, shard)):
            raise ValueError("SLO evidence scope is incomplete")
        values = totals[labels]
        if row.get("type") == "messaging.slo.minute":
            bucket = row.get("bucketStart")
            if not isinstance(bucket, str) or not bucket:
                raise ValueError("SLO minute bucket is incomplete")
            bucket_time = timestamp(bucket)
            observed_time = timestamp(row["observedAt"])
            if (bucket_time.second or bucket_time.microsecond or row.get("bucketSeconds") != 60
                    or observed_time < bucket_time + dt.timedelta(minutes=1)
                    or observed_time > dt.datetime.now(UTC)):
                raise ValueError("SLO minute bucket timing is invalid")
            observed = observed_time.timestamp()
            latest_evidence[labels] = max(latest_evidence.get(labels, 0), observed)
            identity = (*labels, shard, bucket)
            if identity in seen_minutes:
                raise ValueError("duplicate SLO evidence bucket")
            seen_minutes.add(identity)
            planned = _counter(row.get("planned"), "planned")
            eligible = _counter(row.get("eligible"), "eligible")
            ack_good = _counter(row.get("ackGood"), "ackGood")
            live_good = _counter(row.get("liveGood"), "liveGood")
            if eligible > planned or ack_good > eligible or live_good > eligible:
                raise ValueError("SLO evidence counters are inconsistent")
            if row["source"].startswith("synthetic-") and (planned != 2 or eligible != planned):
                raise ValueError("synthetic evidence must retain both scheduled canaries")

            ack_complete = (row.get("settled") is True and row.get("counterReset") is False
                            and row.get("collectorComplete") is True)
            values["teamgram_slo_ack_eligible_total"] += eligible
            values["teamgram_slo_ack_good_total"] += ack_good if ack_complete else 0
            values["teamgram_slo_live_eligible_total"] += eligible
            values["teamgram_slo_live_good_total"] += live_good if ack_complete else 0
            for name in ("ack", "live"):
                key = (*labels, shard, name)
                previous = latest_completeness.get(key)
                if previous is None or bucket_time > previous[0]:
                    latest_completeness[key] = (bucket_time, int(ack_complete))

            measurements = row.get("measurements") if row["schemaVersion"] == SCHEMA_VERSION else {}
            if not isinstance(measurements, dict):
                measurements = {}
            for name, metric_fields, metric_names in (
                ("connection", ("eligible", "good"),
                 ("teamgram_slo_connection_eligible_total", "teamgram_slo_connection_good_total")),
                ("reconnect", ("eligible", "good"),
                 ("teamgram_slo_reconnect_eligible_total", "teamgram_slo_reconnect_good_total")),
                ("messageIntegrity", ("eligible", "lost", "duplicates"),
                 ("teamgram_slo_integrity_eligible_total", "teamgram_slo_message_loss_total", "teamgram_slo_message_duplicates_total")),
            ):
                sample = measurements.get(name)
                complete = ack_complete and isinstance(sample, dict) and sample.get("complete") is True
                key = (*labels, shard, name)
                previous = latest_completeness.get(key)
                if previous is None or bucket_time > previous[0]:
                    latest_completeness[key] = (bucket_time, int(complete))
                if not complete:
                    continue
                sample_values = [_counter(sample.get(field), f"{name}.{field}") for field in metric_fields]
                if sample_values[0] == 0:
                    if latest_completeness[key][0] == bucket_time:
                        latest_completeness[key] = (bucket_time, 0)
                    continue
                if name in ("connection", "reconnect") and sample_values[1] > sample_values[0]:
                    raise ValueError("SLO good count exceeds eligible count")
                if name == "messageIntegrity" and sample_values[1] > sample_values[0]:
                    raise ValueError("SLO message-loss count exceeds eligible count")
                for metric, count in zip(metric_names, sample_values):
                    values[metric] += count
        elif row.get("type") == "messaging.slo.recovery":
            if row_schema != (SCHEMA_VERSION, DEFINITION):
                raise ValueError("recovery events require the current evidence schema")
            event_id = row.get("eventId")
            started, recovered = timestamp(row["startedAt"]), timestamp(row["recoveredAt"])
            if recovered > dt.datetime.now(UTC):
                raise ValueError("future SLO recovery event")
            latest_evidence[labels] = max(latest_evidence.get(labels, 0), recovered.timestamp())
            seconds = row.get("recoverySeconds")
            actual = (recovered - started).total_seconds()
            if (not isinstance(event_id, str) or not event_id or not isinstance(seconds, (int, float))
                    or isinstance(seconds, bool) or not math.isfinite(seconds) or seconds < 0
                    or abs(seconds - actual) > .001
                    or (row["environment"] == "production"
                        and (not isinstance(row.get("drillId"), str) or not row["drillId"].strip()))):
                raise ValueError("invalid SLO recovery event")
            identity = (*labels, event_id)
            if identity in seen_recovery:
                raise ValueError("duplicate SLO recovery event")
            seen_recovery.add(identity)
            values["teamgram_slo_recovery_events_total"] += 1
            values["teamgram_slo_recovery_seconds_sum"] += seconds
            values["teamgram_slo_recovery_seconds_count"] += 1
            for bucket_limit in (*RECOVERY_BUCKETS, "+Inf"):
                if bucket_limit == "+Inf" or seconds <= bucket_limit:
                    recovery_buckets[(labels, bucket_limit)] += 1
        else:
            raise ValueError("unsupported SLO evidence type")

    output = []
    for name, help_text in METRICS:
        output.extend((f"# HELP {name} {help_text}", f"# TYPE {name} counter"))
    for name, help_text in GAUGES:
        output.extend((f"# HELP {name} {help_text}", f"# TYPE {name} gauge"))
    for labels, values in sorted(totals.items()):
        rendered_labels = _labels(("environment", "region", "source"), labels)
        for name, _ in METRICS:
            output.append(f"{name}{{{rendered_labels}}} {values[name]}")
        for name, _ in GAUGES:
            if labels in latest_evidence:
                output.append(f"{name}{{{rendered_labels}}} {latest_evidence[labels]}")
    output.extend(("# HELP teamgram_slo_measurement_complete Whether the latest settled bucket is complete for every scoped shard.",
                   "# TYPE teamgram_slo_measurement_complete gauge"))
    completeness: dict[tuple[str, str, str, str], int] = {}
    for (environment, region, source, shard, sli), (_bucket, complete) in latest_completeness.items():
        key = (environment, region, source, sli)
        completeness[key] = min(completeness.get(key, 1), complete)
    for (environment, region, source, sli), complete in sorted(completeness.items()):
        rendered_labels = _labels(("environment", "region", "source", "sli"), (environment, region, source, sli))
        output.append(f"teamgram_slo_measurement_complete{{{rendered_labels}}} {complete}")
    output.extend(("# HELP teamgram_slo_recovery_seconds Recovery duration histogram in seconds.",
                   "# TYPE teamgram_slo_recovery_seconds histogram"))
    for labels in sorted(totals):
        rendered_scope = _labels(("environment", "region", "source"), labels)
        for bucket_limit in (*RECOVERY_BUCKETS, "+Inf"):
            count = recovery_buckets[(labels, bucket_limit)]
            output.append(f'teamgram_slo_recovery_seconds_bucket{{{rendered_scope},le="{bucket_limit}"}} {count}')
        output.append(f"teamgram_slo_recovery_seconds_sum{{{rendered_scope}}} {totals[labels]['teamgram_slo_recovery_seconds_sum']}")
        output.append(f"teamgram_slo_recovery_seconds_count{{{rendered_scope}}} {totals[labels]['teamgram_slo_recovery_seconds_count']}")
    return "\n".join(output) + "\n"


def write_metrics(evidence: Path, output: Path) -> None:
    rows = [json.loads(line) for line in evidence.read_text(encoding="utf-8").splitlines() if line.strip()]
    rendered = render_metrics(rows)
    output.parent.mkdir(parents=True, exist_ok=True)
    descriptor, temporary = tempfile.mkstemp(prefix=f".{output.name}.", dir=output.parent, text=True)
    try:
        os.fchmod(descriptor, 0o644)
        with os.fdopen(descriptor, "w", encoding="utf-8") as stream:
            stream.write(rendered)
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(temporary, output)
        directory = os.open(output.parent, os.O_RDONLY)
        try:
            os.fsync(directory)
        finally:
            os.close(directory)
    except BaseException:
        try:
            os.unlink(temporary)
        except FileNotFoundError:
            pass
        raise


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("evidence", type=Path, help="append-only minute JSONL evidence")
    parser.add_argument("output", type=Path, help="Prometheus node_exporter textfile output")
    args = parser.parse_args()
    try:
        write_metrics(args.evidence, args.output)
    except (OSError, ValueError, TypeError, json.JSONDecodeError) as error:
        print(f"SLO metrics export failed: {type(error).__name__}")
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
