#!/usr/bin/env python3
"""Measure fixed-rate, two-account Layer 229 capacity in a disposable local lab."""
from __future__ import annotations

import argparse
from concurrent.futures import ThreadPoolExecutor
import datetime as dt
import json
from pathlib import Path
import secrets
import tempfile
import time

from messaging_reliability_probe import (ROOT, fault_control, probe, prometheus_query, seed_accounts,
                                         validate_ports, wait_backend, wait_control, wait_kafka)
from messaging_slo_report import distribution, finite_number
from postgres_recovery_probe import Lab, ProbeError, redact


STAGE_METRICS = {
    "deliveryClaimWaitCount": 'teamgram_delivery_claim_wait_seconds_count{job="messenger.msg"}',
    "deliveryClaimWaitSum": 'teamgram_delivery_claim_wait_seconds_sum{job="messenger.msg"}',
    "deliveryPublicationCount": 'teamgram_delivery_publication_seconds_count{job="messenger.msg"}',
    "deliveryPublicationSum": 'teamgram_delivery_publication_seconds_sum{job="messenger.msg"}',
    "deliveryCompletionCount": 'teamgram_delivery_completion_seconds_count{job="messenger.msg"}',
    "deliveryCompletionSum": 'teamgram_delivery_completion_seconds_sum{job="messenger.msg"}',
    "deliveryResults": 'teamgram_delivery_publications_total{job="messenger.msg"}',
    "syncDeliveryCount": 'teamgram_sync_delivery_seconds_count{job="messenger.sync"}',
    "syncDeliverySum": 'teamgram_sync_delivery_seconds_sum{job="messenger.sync"}',
    "syncResults": 'teamgram_sync_delivery_total{job="messenger.sync"}',
}


def stage_metric_snapshot(port: int) -> dict:
    """Return low-cardinality stage counters without making diagnostics a gate."""
    snapshot = {}
    for name, expression in STAGE_METRICS.items():
        rows = prometheus_query(port, expression)
        snapshot[name] = [
            {"labels": {key: value for key, value in row["metric"].items()
                        if key != "__name__"},
             "value": float(row["value"][1])}
            for row in rows
        ]
    return snapshot


def stage_metric_delta(before: dict, after: dict) -> dict:
    """Compare counter/histogram-count samples by their bounded label sets."""
    delta = {}
    for name, rows in after.items():
        previous = {
            json.dumps(item.get("labels", {}), sort_keys=True): item.get("value", 0)
            for item in before.get(name, [])
        }
        delta[name] = []
        for item in rows:
            key = json.dumps(item.get("labels", {}), sort_keys=True)
            delta[name].append({"labels": item.get("labels", {}),
                                "value": item.get("value", 0) - previous.get(key, 0)})
    return delta


def settled_stage_metrics(port: int, before: dict, timeout: float = 15) -> dict:
    """Allow the 5-second scrape interval to observe the async sync consumer."""
    deadline = time.monotonic() + timeout
    latest = {}
    while time.monotonic() < deadline:
        latest = stage_metric_snapshot(port)
        if latest.get("syncDeliveryCount") or latest.get("syncResults"):
            return latest
        time.sleep(1)
    return latest


def evaluate(value: dict, *, ack_ms: float = 1000, live_ms: float = 2000) -> dict:
    data = value.get("capacity", {})
    rate, duration = data.get("rate"), data.get("durationMs")
    if (not finite_number(rate, positive=True) or not finite_number(duration, positive=True)
            or not finite_number(ack_ms, positive=True) or not finite_number(live_ms, positive=True)
            or not float(rate * duration / 1000).is_integer()):
        raise ValueError("invalid capacity schedule")
    planned = int(rate * duration / 1000)
    samples = data.get("samples", [])
    if not isinstance(samples, list) or len(samples) != planned or data.get("planned") != planned:
        raise ValueError("every scheduled slot must be present in the denominator")
    ack, live, ack_dispatch, live_dispatch = [], [], [], []
    counts = dict(planned=planned, dispatched=0, dropped=0, acked=0, delivered=0,
                  ack_on_time=0, live_on_time=0, ack_within_window=0, live_within_window=0)
    dropped_reasons: dict[str, int] = {}
    for index, row in enumerate(samples):
        if (row.get("slot") != index or row.get("direction") != ("B-to-A" if index % 2 else "A-to-B")
                or not finite_number(row.get("scheduledAtMs"))
                or abs(row["scheduledAtMs"] - index * 1000 / rate) > 1e-6):
            raise ValueError("omitted, reordered or conflicting scheduled slot")
        dispatched = row.get("dispatchedAtMs")
        if row.get("dropped"):
            if dispatched is not None or row.get("ack") or row.get("live"):
                raise ValueError("dropped slot cannot contain a successful send")
            counts["dropped"] += 1
            dropped_reasons[row["dropped"]] = dropped_reasons.get(row["dropped"], 0) + 1
            continue
        if not finite_number(dispatched) or dispatched < row["scheduledAtMs"] - 2:
            raise ValueError("invalid dispatch clock")
        counts["dispatched"] += 1
        for kind, budget, scheduled_values, dispatch_values, count_key in (
            ("ack", ack_ms, ack, ack_dispatch, "acked"), ("live", live_ms, live, live_dispatch, "delivered")
        ):
            timestamp = row.get(f"{kind}AtMs")
            event = row.get(kind)
            if not event or row.get(f"{kind}Error"):
                continue
            if (not finite_number(timestamp) or timestamp < dispatched
                    or not isinstance(event, dict) or not finite_number(event.get("id"), positive=True)):
                raise ValueError("invalid message result or clock")
            elapsed = timestamp - row["scheduledAtMs"]
            counts[count_key] += 1
            counts[f"{kind}_on_time"] += elapsed <= budget
            counts[f"{kind}_within_window"] += timestamp <= duration
            scheduled_values.append(elapsed)
            dispatch_values.append(timestamp - dispatched)
    checks = value.get("checks", {})
    consistent = (value.get("schemaVersion") == 1 and value.get("phase") == "capacity"
                  and value.get("evidenceClass") == "local_probe" and value.get("productionSloEvidence") is False
                  and value.get("clock") == "performance.now" and value.get("ok") is True
                  and value.get("processExitCode") == 0
                  and all(checks.get(key) is True for key in ("ptsContinuity", "historiesUnique", "liveMatchesHistory", "outboxDrainedBeforeCleanup"))
                  and value.get("cleanup", {}).get("ok") is True and value["cleanup"].get("remaining") == 0)
    passed = consistent and counts["ack_on_time"] == planned and counts["live_on_time"] == planned and not counts["dropped"]
    return {"status": "PASS" if passed else "FAIL", "scope": "local two-account private chat",
            "productionCapacityAccepted": False, "offeredMessagesPerSecond": rate, "durationSeconds": duration / 1000,
            "counts": counts, "consistency": consistent, "budgetsMs": {"scheduledToAck": ack_ms, "scheduledToLive": live_ms},
            "generatorDroppedReasons": dropped_reasons,
            "ackSuccessRatio": counts["acked"] / planned, "liveSuccessRatio": counts["delivered"] / planned,
            "ackOnTimeRatio": counts["ack_on_time"] / planned, "liveOnTimeRatio": counts["live_on_time"] / planned,
            "windowAckGoodput": counts["ack_within_window"] * 1000 / duration,
            "windowLiveGoodput": counts["live_within_window"] * 1000 / duration,
            "latencyConditionalOnObservedResult": {"scheduledToAck": distribution(ack), "scheduledToLive": distribution(live),
                "dispatchToAck": distribution(ack_dispatch), "dispatchToLive": distribution(live_dispatch)}}


def start_lab(lab: Lab, args: argparse.Namespace, report: dict) -> Path:
    report["preflight"] = lab.preflight()
    gramjs = args.gramjs_dir.resolve()
    if not (gramjs / "node_modules/.bin/tsx").is_file():
        raise ProbeError("installed GramJS tsx runtime required")
    report["image"] = lab.run(["docker", "image", "inspect", args.image, "--format", "{{.Id}}"]).stdout.strip()
    report["docker"] = lab.run(["docker", "info", "--format", "CPUs={{.NCPU}}; MemoryBytes={{.MemTotal}}; OS={{.OperatingSystem}}"]).stdout.strip()
    lab.start("postgres", "postgres-migrate", "kafka", "redis", "etcd", "minio")
    lab.compose("wait", "postgres-migrate")
    if lab.container("postgres-migrate")["State"]["ExitCode"]:
        raise ProbeError("capacity lab migrations failed")
    report["migration_count"] = int(lab.sql("postgres", "SELECT count(*) FROM schema_migrations;"))
    wait_kafka(lab)
    for topic in ("Inbox-T", "Sync-T", "BotFather-T"):
        lab.compose("exec", "-T", "kafka", "/opt/kafka/bin/kafka-topics.sh", "--bootstrap-server", "kafka:9092",
                    "--create", "--if-not-exists", "--topic", topic, "--partitions", "3", "--replication-factor", "1")
    seed_accounts(lab)
    lab.start("backend", "prometheus")
    wait_backend(lab)
    return gramjs


def run_profile(lab: Lab, gramjs: Path, rate: int, seconds: int, max_inflight: int,
                state_file: Path, metrics_port: int) -> tuple[dict, int]:
    lab.env.update(LOAD_RATE=str(rate), LOAD_SECONDS=str(seconds), LOAD_MAX_INFLIGHT=str(max_inflight))
    with tempfile.TemporaryDirectory(prefix=f"{lab.project}-capacity-") as temporary, ThreadPoolExecutor(max_workers=1) as executor:
        directory = Path(temporary)
        lab.env["PROBE_CONTROL_DIR"] = temporary
        try:
            metrics_before = stage_metric_snapshot(metrics_port)
        except (OSError, ProbeError, ValueError):
            metrics_before = {"error": "unavailable"}
        future = executor.submit(probe, lab, gramjs, "capacity", state_file, 1, require_success=False)
        backlog = []
        try:
            started = time.monotonic()
            while not (directory / "sent.json").exists() and not future.done():
                snapshot = json.loads(lab.sql("postgres", """SELECT json_build_object(
                    'pending',(SELECT count(*) FROM msg_inbox_delivery_outbox WHERE state=0)+(SELECT count(*) FROM msg_state_delivery_outbox WHERE state=0),
                    'oldestSeconds',(SELECT coalesce(max(extract(epoch from now()-created_at)),0) FROM msg_inbox_delivery_outbox WHERE state=0));"""))
                backlog.append({"elapsedSeconds": round(time.monotonic()-started, 3), **snapshot})
                if time.monotonic() - started > seconds + 120:
                    raise ProbeError("capacity drain milestone exceeded deadline")
                time.sleep(2)
            wait_control(lab, directory, "sent", future)
            pending = lab.wait_sql("postgres", "SELECT (SELECT count(*) FROM msg_inbox_delivery_outbox WHERE state=0)+(SELECT count(*) FROM msg_state_delivery_outbox WHERE state=0);", "0")
        finally:
            fault_control(lab, directory, "restored")
        evidence = future.result()
        evidence["databaseBacklogSamples"] = backlog
        try:
            metrics_after = settled_stage_metrics(metrics_port, metrics_before)
            if "error" in metrics_before:
                evidence["stageMetrics"] = {"before": metrics_before, "after": metrics_after}
            else:
                evidence["stageMetrics"] = {"before": metrics_before, "after": metrics_after,
                                              "delta": stage_metric_delta(metrics_before, metrics_after)}
        except (OSError, ProbeError, ValueError):
            evidence["stageMetrics"] = {"before": metrics_before, "after": {"error": "unavailable"}}
        # Cleanup itself creates ordered state intents; drain those before
        # measuring the next profile or final dependency health.
        pending = lab.wait_sql("postgres", "SELECT (SELECT count(*) FROM msg_inbox_delivery_outbox WHERE state=0)+(SELECT count(*) FROM msg_state_delivery_outbox WHERE state=0);", "0")
        lab.env.pop("PROBE_CONTROL_DIR", None)
    return evidence, int(pending)


def experiment(lab: Lab, args: argparse.Namespace, report: dict) -> None:
    gramjs = start_lab(lab, args, report)
    report["profiles"] = []
    for rate in args.rates:
        evidence, pending = run_profile(lab, gramjs, rate, args.seconds, args.max_inflight,
                                        args.output.parent / f"{lab.run_id}-capacity-state.json",
                                        args.metrics_port)
        result = evaluate(evidence)
        report["profiles"].append({"summary": result, "pendingAfterDrain": int(pending), "evidence": evidence})
        if result["status"] != "PASS":
            break
    passed = [row["summary"]["offeredMessagesPerSecond"] for row in report["profiles"] if row["summary"]["status"] == "PASS"]
    report["highestPassingTestedRate"] = max(passed) if passed else None
    report["capacityCeilingFound"] = False
    report["status"] = ("PASS" if len(passed) == len(args.rates) else "PROFILE_FAILED"
                        if any(not row["summary"]["consistency"] for row in report["profiles"]) else "BUDGET_EXCEEDED")
    report["dependencies_at_completion"] = {}
    for service in ("postgres", "kafka", "redis", "etcd", "minio", "backend", "prometheus"):
        state = lab.container(service)["State"]
        health = state.get("Health", {}).get("Status")
        if not state["Running"] or health not in (None, "healthy"):
            raise ProbeError(f"capacity dependency {service} is not healthy")
        report["dependencies_at_completion"][service] = {"running": True, "health": health}


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--execute", action="store_true")
    parser.add_argument("--rates", type=int, nargs="+", default=[5, 10, 20])
    parser.add_argument("--seconds", type=int, default=60)
    parser.add_argument("--max-inflight", type=int, default=32)
    parser.add_argument("--image", default="teamgram-reliability:20261015")
    parser.add_argument("--gramjs-dir", type=Path, default=ROOT.parent / "telegram-tt-master")
    parser.add_argument("--output", type=Path, default=Path("/tmp/teamgram-messaging-capacity.json"))
    parser.add_argument("--postgres-port", type=int, default=55438)
    parser.add_argument("--mt-port", type=int, default=21544)
    parser.add_argument("--kafka-port", type=int, default=19095)
    parser.add_argument("--metrics-port", type=int, default=19091)
    args = parser.parse_args()
    report = {"status": "PLAN", "scope": "local fixed-rate two-account seeded Layer 229 private chat",
              "productionCapacityAccepted": False, "started_at": dt.datetime.now(dt.timezone.utc).isoformat()}
    lab = None
    try:
        if (args.rates != sorted(set(args.rates)) or not 1 <= args.seconds <= 600
                or not 1 <= args.max_inflight <= 256 or any(rate <= 0 or rate * args.seconds > 20000 for rate in args.rates)):
            raise ProbeError("increasing unique rates, seconds 1..600, inflight 1..256, <=20000 slots required")
        validate_ports([args.postgres_port, args.mt_port, args.kafka_port, args.metrics_port])
        lab = Lab("teamgram-reliability-cap-" + secrets.token_hex(6), port=args.postgres_port, timeout=300, wait=150,
                  profiles=["messaging", "backend", "monitoring"], env={
                      "TEAMGRAM_RELIABILITY_IMAGE": args.image, "TEAMGRAM_RELIABILITY_MT_PORT": str(args.mt_port),
                      "TEAMGRAM_RELIABILITY_KAFKA_PORT": str(args.kafka_port), "TEAMGRAM_RELIABILITY_METRICS_PORT": str(args.metrics_port)})
        report["project"] = lab.project
        report["requestedProfiles"] = {"rates": args.rates, "seconds": args.seconds, "maxInflight": args.max_inflight}
        if args.execute:
            experiment(lab, args, report)
    except (ProbeError, OSError, ValueError, KeyboardInterrupt) as error:
        report.update(status="FAIL", reason=redact(str(error), lab.secret_values if lab else []))
    finally:
        if lab and args.execute:
            try:
                report["cleanup"] = lab.cleanup()
            except (ProbeError, OSError, ValueError) as error:
                report.update(status="FAIL", cleanup={"status": "FAIL", "reason": redact(str(error), lab.secret_values)})
        report["finished_at"] = dt.datetime.now(dt.timezone.utc).isoformat()
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(json.dumps(report, indent=2, sort_keys=True) + "\n")
    print(json.dumps({"status": report["status"], "output": str(args.output), "highestPassingTestedRate": report.get("highestPassingTestedRate"), "reason": report.get("reason")}))
    return 0 if report["status"] in ("PLAN", "PASS") else 1


if __name__ == "__main__":
    raise SystemExit(main())
