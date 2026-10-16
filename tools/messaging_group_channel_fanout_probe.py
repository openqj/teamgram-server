#!/usr/bin/env python3
"""Measure Layer 229 live fan-out to 5,000 online channel and megagroup members."""

from __future__ import annotations

import argparse
import datetime as dt
import json
import os
from pathlib import Path
import secrets
import shutil
import time

import messaging_online_capacity_probe as online_probe
import messaging_reliability_probe
from messaging_reliability_probe import ROOT, seed_accounts, validate_ports, wait_backend, wait_kafka
from postgres_recovery_probe import Lab, ProbeError, redact


FANOUT_STAGE_METRICS = {
    "persistenceCount": 'teamgram_apifull_channel_persistence_seconds_count{job="bff.bff"}',
    "persistenceSum": 'teamgram_apifull_channel_persistence_seconds_sum{job="bff.bff"}',
    "persistenceBucket": 'teamgram_apifull_channel_persistence_seconds_bucket{job="bff.bff"}',
    "apifullStageCount": 'teamgram_apifull_channel_delivery_stage_seconds_count{job="bff.bff"}',
    "apifullStageSum": 'teamgram_apifull_channel_delivery_stage_seconds_sum{job="bff.bff"}',
    "apifullStageBucket": 'teamgram_apifull_channel_delivery_stage_seconds_bucket{job="bff.bff"}',
    "apifullStageResults": 'teamgram_apifull_channel_delivery_stage_total{job="bff.bff"}',
    "syncStageCount": 'teamgram_sync_delivery_seconds_count{job="messenger.sync"}',
    "syncStageSum": 'teamgram_sync_delivery_seconds_sum{job="messenger.sync"}',
    "syncStageBucket": 'teamgram_sync_delivery_seconds_bucket{job="messenger.sync"}',
    "syncStageResults": 'teamgram_sync_delivery_total{job="messenger.sync"}',
    "syncRecordAgeBucket": 'mqconsumer_record_age_seconds_bucket{job="messenger.sync"}',
    "syncHandlerQueueBucket": 'mqconsumer_handler_queue_seconds_bucket{job="messenger.sync"}',
    "syncPartitionLag": 'mqconsumer_partition_lag{job="messenger.sync"}',
    "syncHandlerInFlight": 'mqconsumer_handler_in_flight{job="messenger.sync"}',
    "syncOffsetCommitBucket": 'mqconsumer_offset_commit_seconds_bucket{job="messenger.sync"}',
    "syncHandlerRetries": 'mqconsumer_handler_retries_total{job="messenger.sync",group="Sync-MainCommunity-S",topic="Sync-T"}',
    "syncPoolConnections": 'teamgram_sync_postgres_pool_connections{job="messenger.sync"}',
    "syncPoolMaxConnections": 'teamgram_sync_postgres_pool_max_connections{job="messenger.sync"}',
    "syncPoolAcquireCount": 'teamgram_sync_postgres_pool_acquire_count{job="messenger.sync"}',
    "syncPoolAcquireDuration": 'teamgram_sync_postgres_pool_acquire_duration_seconds{job="messenger.sync"}',
    "syncPoolEmptyAcquireCount": 'teamgram_sync_postgres_pool_empty_acquire_count{job="messenger.sync"}',
    "syncPoolEmptyAcquireWait": 'teamgram_sync_postgres_pool_empty_acquire_wait_seconds{job="messenger.sync"}',
    "syncPoolCanceledAcquireCount": 'teamgram_sync_postgres_pool_canceled_acquire_count{job="messenger.sync"}',
    "gatewayWriteQueueWaitCount": 'gnetway_gateway_write_queue_wait_ms_count{job="interface.gateway"}',
    "gatewayWriteQueueWaitSum": 'gnetway_gateway_write_queue_wait_ms_sum{job="interface.gateway"}',
    "gatewayWriteQueueWaitBucket": 'gnetway_gateway_write_queue_wait_ms_bucket{job="interface.gateway"}',
    "gatewayWriteRejected": 'gnetway_gateway_write_rejected_total{job="interface.gateway"}',
    "gatewayWriteFailed": 'gnetway_gateway_write_failed_total{job="interface.gateway"}',
    "gatewayWriteFlushCount": 'gnetway_gateway_write_flush_duration_ms_count{job="interface.gateway"}',
    "gatewayWriteFlushSum": 'gnetway_gateway_write_flush_duration_ms_sum{job="interface.gateway"}',
    "gatewayWriteFlushBucket": 'gnetway_gateway_write_flush_duration_ms_bucket{job="interface.gateway"}',
    "gatewayWriteSocketBufferedCount": 'gnetway_gateway_write_socket_buffered_bytes_count{job="interface.gateway"}',
    "gatewayWriteSocketBufferedSum": 'gnetway_gateway_write_socket_buffered_bytes_sum{job="interface.gateway"}',
    "gatewayWriteSocketBufferedBucket": 'gnetway_gateway_write_socket_buffered_bytes_bucket{job="interface.gateway"}',
    "gatewayStreamHandlerCount": 'gnetway_gateway_stream_handler_duration_ms_count{job="interface.gateway"}',
    "gatewayStreamHandlerSum": 'gnetway_gateway_stream_handler_duration_ms_sum{job="interface.gateway"}',
    "gatewayStreamHandlerBucket": 'gnetway_gateway_stream_handler_duration_ms_bucket{job="interface.gateway"}',
    "sessionGatewayDeliveryCount": 'teamgram_session_gateway_delivery_seconds_count{job="interface.session"}',
    "sessionGatewayDeliverySum": 'teamgram_session_gateway_delivery_seconds_sum{job="interface.session"}',
    "sessionGatewayDeliveryBucket": 'teamgram_session_gateway_delivery_seconds_bucket{job="interface.session"}',
    "sessionGatewayDeliveryResults": 'teamgram_session_gateway_delivery_total{job="interface.session"}',
}


def fanout_stage_metric_snapshot(port: int) -> dict:
    snapshot = {}
    for name, expression in FANOUT_STAGE_METRICS.items():
        rows = messaging_reliability_probe.prometheus_query(port, expression)
        snapshot[name] = [
            {"labels": {key: value for key, value in row["metric"].items() if key != "__name__"},
             "value": float(row["value"][1])}
            for row in rows
        ]
    return snapshot


def fanout_stage_metric_delta(before: dict, after: dict) -> dict:
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


def sync_pool_metric_snapshot(metrics: dict) -> dict:
    return {key: value for key, value in metrics.items() if key.startswith("syncPool")}


def parse_evidence(result, phase: str) -> dict:
    for line in result.stdout.splitlines():
        try:
            candidate = json.loads(line)
        except json.JSONDecodeError:
            continue
        if candidate.get("probe") == "messaging.group-channel-fanout" and candidate.get("phase") == phase:
            return candidate
    raise ProbeError(f"group/channel fan-out probe returned no {phase} evidence")


def evaluate(evidence: dict, database: list[dict], users: int, backend_instances: int) -> dict:
    connections = evidence.get("connections", {})
    expected_replicas = {
        f"backend-{index + 1}": users // backend_instances + (index < users % backend_instances)
        for index in range(backend_instances)
    }
    distribution = connections.get("endpointDistribution", {})
    deliveries = evidence.get("deliveries", [])
    recovery_expected = evidence.get("recoveryPlan", {}).get("requestedPerTarget", 0)
    by_kind = {item.get("kind"): item for item in deliveries}
    db_by_channel = {str(item.get("channel_id")): item for item in database}

    delivery_checks = {}
    for item in deliveries:
        db = db_by_channel.get(str(item.get("channelId")), {})
        kind = item.get("kind", "unknown")
        recovery = item.get("recovery", {})
        live_expected = item.get("liveRecipientsExpected", users - recovery_expected)
        delivery_checks[kind] = {
            "allRecipientsReceived": item.get("liveRecipients") == live_expected
            and item.get("uniqueRecipients") == users and item.get("missingRecipients") == 0
            and recovery.get("recoveredRecipients") == recovery_expected,
            "noDuplicateUpdates": item.get("duplicateUpdates") == 0
            and recovery.get("duplicateRecoveredMessages") == 0
            and recovery.get("unexpectedRealtimeRecipients") == 0,
            "channelPtsValid": item.get("ptsValid") is True
            and (recovery_expected == 0 or recovery.get("ptsContinuous") is True),
            "historyReadable": item.get("channelHistoryReadByListener") is True
            and (recovery_expected == 0 or recovery.get("historyReadable") is True),
            "recoveryFinal": recovery_expected == 0 or recovery.get("allFinal") is True,
            "recoveryConnections": recovery_expected == 0 or recovery.get("reconnectedRecipients") == recovery_expected,
            "outboxComplete": db.get("events", -1) == 1 and db.get("completed_events", -1) == 1
            and db.get("pending_events", -1) == 0 and db.get("recipient_rows", -1) == 0
            and db.get("pending_recipients", -1) == 0,
            "ackWithin1s": item.get("ackWithin1s") is True,
            "allLiveWithin2s": item.get("receiveWithin2s") == live_expected,
        }

    checks = {
        "allClientsAuthenticated": connections.get("authenticated") == users,
        "allClientsRegisteredForUpdates": connections.get("activated") == users,
        "connectionsHeldThroughFanout": connections.get("connectedAtStart") == users
        and connections.get("connectedAtEnd") == users,
        "allReplicasServeExpectedConnections": all(distribution.get(name, -1) == count
                                                    for name, count in expected_replicas.items()),
        "serverGaugeReachedRequestedUsers": evidence.get("serverGaugeAtStart", 0) >= users,
        "bothTargetKindsTested": set(by_kind) == {"channel", "megagroup"},
        "channelDeliveryBudgets": all(all(kind_checks.values()) for kind_checks in delivery_checks.values())
        and len(delivery_checks) == 2,
        "connectionCleanup": evidence.get("connectionCleanupObserved") is True,
    }
    complete = all(
        item.get("uniqueRecipients") == users and item.get("missingRecipients") == 0
        and item.get("duplicateUpdates") == 0
        and item.get("recovery", {}).get("recoveredRecipients") == recovery_expected
        for item in deliveries
    ) and len(deliveries) == 2
    latency_checks = {"ackWithin1s", "allLiveWithin2s"}
    non_latency_checks = all(
        value for kind_checks in delivery_checks.values()
        for name, value in kind_checks.items() if name not in latency_checks
    )
    status = "PASS" if all(checks.values()) else (
        "BUDGET_EXCEEDED" if complete and non_latency_checks and all(checks[name] for name in checks
            if name != "channelDeliveryBudgets") else "FAIL"
    )
    return {"status": status, "checks": checks, "deliveryChecks": delivery_checks,
            "expectedReplicaConnections": expected_replicas}


def run_ts(lab: Lab, executable: Path, phase: str, *, session_file: Path, channels_file: Path,
           args: argparse.Namespace) -> dict:
    lab.env.update({
        "TEAMGRAM_GRAMJS_DIR": str(args.gramjs_dir.resolve()),
        "FANOUT_PHASE": phase,
        "FANOUT_TAG": lab.run_id,
        "FANOUT_USERS": str(args.users),
        "FANOUT_BATCH_SIZE": str(args.batch_size),
        "FANOUT_RECOVERY_USERS": str(args.recovery_users),
        "FANOUT_TIMEOUT_MS": str(args.timeout_ms),
        "FANOUT_WAIT_MS": str(args.delivery_timeout_seconds * 1000),
        "FANOUT_ENDPOINTS": json.dumps([
            {"host": "127.0.0.1", "port": args.mt_port, "replica": "backend-1"},
            *([{"host": "127.0.0.1", "port": args.mt_port_2, "replica": "backend-2"}]
              if args.backend_instances == 2 else []),
        ]),
        "ONLINE_SESSIONS_FILE": str(session_file),
        "FANOUT_CHANNELS_FILE": str(channels_file),
        "TG_DC_PORT": str(args.mt_port),
        "METRICS_PORT": str(args.metrics_port),
        "FANOUT_OWNER_AUTH_B64": lab.env["A_AUTH_B64"],
        "FANOUT_OWNER_USER_ID": lab.env["A_USER_ID"],
        "FANOUT_CLIENT_PROCESSES": str(args.client_processes),
    })
    if args.client_processes > 1:
        diagnostic_dir = Path(args.output.parent) / f".{args.output.stem}-multiprocess"
        diagnostic_dir.mkdir(parents=True, exist_ok=True)
        lab.env["FANOUT_DIAGNOSTIC_DIR"] = str(diagnostic_dir)
        lab.env["FANOUT_DIAGNOSTIC_WAIT_MS"] = str(args.diagnostic_wait_seconds * 1000)
    result = lab.run([str(executable), str(ROOT / "docker/production/probe-group-channel-fanout.ts")],
                     timeout=max(lab.timeout, args.delivery_timeout_seconds * 2 + 600), check=False)
    evidence = parse_evidence(result, phase)
    if result.returncode and evidence.get("error") is None:
        evidence["processExitCode"] = result.returncode
    return evidence


def channel_delivery_snapshot(lab: Lab, channel_ids: list[str]) -> list[dict]:
    ids = ",".join(str(int(item)) for item in channel_ids)
    value = lab.sql("postgres", f"""SELECT coalesce(json_agg(row_to_json(s) ORDER BY s.channel_id),'[]'::json)
FROM (
  SELECT o.channel_id, count(DISTINCT o.id) AS events,
         count(DISTINCT o.id) FILTER (WHERE o.state='completed') AS completed_events,
         count(DISTINCT o.id) FILTER (WHERE o.state<>'completed') AS pending_events,
         count(r.user_id) AS recipient_rows,
         count(r.user_id) FILTER (WHERE r.state='pending') AS pending_recipients
  FROM apifull_channel_delivery_outbox o
  LEFT JOIN apifull_channel_delivery_recipient r ON r.delivery_id=o.id
  WHERE o.channel_id IN ({ids})
  GROUP BY o.channel_id
) s;""")
    return json.loads(value)


def wait_for_outbox(lab: Lab, channel_ids: list[str], timeout: int) -> list[dict]:
    deadline = time.monotonic() + timeout
    latest: list[dict] = []
    while time.monotonic() < deadline:
        latest = channel_delivery_snapshot(lab, channel_ids)
        if len(latest) == len(channel_ids) and all(
            item["completed_events"] == item["events"] and item["pending_recipients"] == 0 for item in latest
        ):
            return latest
        time.sleep(0.5)
    return latest


def experiment(lab: Lab, args: argparse.Namespace, report: dict, session_file: Path,
               channels_file: Path) -> None:
    report["preflight"] = lab.preflight()
    gramjs = args.gramjs_dir.resolve()
    executable = gramjs / "node_modules/.bin/tsx"
    if not executable.is_file() or not (gramjs / "src/lib/gramjs/client/TelegramClient.ts").is_file():
        raise ProbeError("--gramjs-dir must contain the Layer 229 Web client and installed tsx runtime")
    report["image"] = {"name": args.image,
                       "id": lab.run(["docker", "image", "inspect", args.image, "--format", "{{.Id}}"], timeout=30).stdout.strip()}
    lab.start("postgres", "postgres-migrate", "kafka", "redis", "etcd", "minio")
    lab.compose("wait", "postgres-migrate")
    if lab.container("postgres-migrate")["State"].get("ExitCode") != 0:
        raise ProbeError("isolated migrations failed")
    report["migration_count"] = int(lab.sql("postgres", "SELECT count(*) FROM schema_migrations;"))
    wait_kafka(lab)
    for topic in ("Inbox-T", "Sync-T", "BotFather-T", "reliability-producer-acceptance"):
        partitions = args.sync_partitions if topic == "Sync-T" else 6
        lab.compose("exec", "-T", "kafka", "/opt/kafka/bin/kafka-topics.sh", "--bootstrap-server", "kafka:9092",
                    "--create", "--if-not-exists", "--topic", topic, "--partitions", str(partitions), "--replication-factor", "1")
    seed_accounts(lab)
    report["seed"] = {"users": args.users, "baseUserId": args.base_user_id, "status": "PASS"}
    online_probe.seed_online_accounts(lab, args.users, session_file, base_user_id=args.base_user_id)
    report["seed"]["authRows"] = int(lab.sql("postgres", "SELECT count(*) FROM auth_users WHERE user_id >= %d AND user_id < %d;" %
                                               (args.base_user_id, args.base_user_id + args.users)))
    if report["seed"]["authRows"] != args.users:
        raise ProbeError("online account seeding did not create the requested auth rows")
    services = ["backend"] + (["backend-2"] if args.backend_instances == 2 else []) + ["prometheus"]
    lab.start(*services)
    wait_backend(lab)
    if args.backend_instances == 2:
        wait_backend(lab, "backend-2")

    setup = run_ts(lab, executable, "setup", session_file=session_file, channels_file=channels_file, args=args)
    fixtures = setup.get("channelFixtures", [])
    if setup.get("error") or len(fixtures) != 2:
        raise ProbeError(f"test channel setup failed: {setup.get('error', {}).get('code', 'INVALID_CHANNEL_FIXTURE')}")
    report["targets"] = setup.get("channels", [])
    channels_file.write_text(json.dumps(fixtures, sort_keys=True) + "\n")
    channels_file.chmod(0o600)
    membership_statements = ["BEGIN;"]
    for channel in fixtures:
        channel_id = int(channel["id"])
        membership_statements.append(f"""INSERT INTO apifull_channel_member (channel_id,user_id,invited_by_user_id,joined_at)
SELECT {channel_id},id,910001,extract(epoch FROM now())::integer FROM users
WHERE id >= {args.base_user_id} AND id < {args.base_user_id + args.users}
ON CONFLICT (channel_id,user_id) DO NOTHING;""")
    membership_statements.append("COMMIT;")
    lab.sql("postgres", "\n".join(membership_statements))
    report["memberships"] = int(lab.sql("postgres", "SELECT count(*) FROM apifull_channel_member WHERE channel_id IN (" +
                                         ",".join(str(int(item["id"])) for item in fixtures) + ");"))
    if report["memberships"] != args.users * 2:
        raise ProbeError("channel/megagroup membership seeding did not create all requested recipients")

    try:
        stage_metrics_before = fanout_stage_metric_snapshot(args.metrics_port)
        report["syncPoolMetricBefore"] = sync_pool_metric_snapshot(stage_metrics_before)
    except (OSError, ProbeError, ValueError, KeyError, TypeError):
        stage_metrics_before = {"error": "unavailable"}
        report["syncPoolMetricBefore"] = {"error": "unavailable"}
    phase = "multiprocess" if args.client_processes > 1 else "fanout"
    report["fanout"] = run_ts(lab, executable, phase, session_file=session_file,
                              channels_file=channels_file, args=args)
    channel_ids = [str(item["id"]) for item in fixtures]
    report["databaseDelivery"] = wait_for_outbox(lab, channel_ids, args.delivery_timeout_seconds)
    if "error" not in stage_metrics_before:
        time.sleep(6)
        try:
            stage_metrics_after = fanout_stage_metric_snapshot(args.metrics_port)
            report["deliveryStageMetricDelta"] = fanout_stage_metric_delta(stage_metrics_before, stage_metrics_after)
            report["syncPoolMetricAfter"] = sync_pool_metric_snapshot(stage_metrics_after)
        except (OSError, ProbeError, ValueError, KeyError, TypeError):
            report["deliveryStageMetricDelta"] = {"error": "unavailable"}
            report["syncPoolMetricAfter"] = {"error": "unavailable"}
    else:
        report["deliveryStageMetricDelta"] = stage_metrics_before
        report["syncPoolMetricAfter"] = {"error": "unavailable"}
    report["postgresDuringFanout"] = online_probe.postgres_connection_snapshot(lab)
    if args.client_processes > 1:
        database_complete = len(report["databaseDelivery"]) == len(channel_ids) and all(
            item["events"] == 1 and item["completed_events"] == 1 and item["pending_events"] == 0
            and item["recipient_rows"] == 0 and item["pending_recipients"] == 0
            for item in report["databaseDelivery"]
        )
        diagnostic_complete = report["fanout"].get("status") in {"PASS", "BUDGET_EXCEEDED"}
        report["acceptance"] = {
            "status": "PASS" if diagnostic_complete and database_complete and report["fanout"].get("status") == "PASS"
            else "BUDGET_EXCEEDED" if diagnostic_complete and database_complete else "FAIL",
            "mode": "multi_process_listener_diagnostic",
            "allRecipientsReceived": diagnostic_complete,
            "databaseDeliveryComplete": database_complete,
            "productionCapacityAccepted": False,
        }
        report["status"] = report["acceptance"]["status"]
        return
    report["acceptance"] = evaluate(report["fanout"], report["databaseDelivery"], args.users, args.backend_instances)
    if report["acceptance"]["status"] != "PASS":
        report["status"] = report["acceptance"]["status"]
        return

    for service in services + ["postgres", "kafka", "redis", "etcd", "minio"]:
        state = lab.container(service)["State"]
        health = state.get("Health", {}).get("Status")
        if not state["Running"] or health not in (None, "healthy"):
            raise ProbeError(f"fan-out dependency {service} is not healthy")
    report["dependenciesAtCompletion"] = {service: {"running": True,
            "health": lab.container(service)["State"].get("Health", {}).get("Status")}
            for service in services + ["postgres", "kafka", "redis", "etcd", "minio"]}
    report["status"] = "PASS"


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--execute", action="store_true")
    parser.add_argument("--users", type=int, default=5000)
    parser.add_argument("--recovery-users", type=int)
    parser.add_argument("--backend-instances", type=int, choices=(1, 2), default=2)
    parser.add_argument("--sync-partitions", type=int, default=6)
    parser.add_argument("--batch-size", type=int, default=50)
    parser.add_argument("--timeout-ms", type=int, default=60000)
    parser.add_argument("--delivery-timeout-seconds", type=int, default=180)
    parser.add_argument("--base-user-id", type=int, default=920000)
    parser.add_argument("--image", default="teamgram-reliability:20261015-dual-backend-safe-commit-batch-r8")
    parser.add_argument("--gramjs-dir", type=Path, default=ROOT.parent / "telegram-tt-master")
    parser.add_argument("--output", type=Path, default=Path("/tmp/teamgram-group-channel-fanout-5000.json"))
    parser.add_argument("--postgres-port", type=int, default=55439)
    parser.add_argument("--mt-port", type=int, default=21545)
    parser.add_argument("--mt-port-2", type=int, default=21546)
    parser.add_argument("--kafka-port", type=int, default=19096)
    parser.add_argument("--metrics-port", type=int, default=19092)
    parser.add_argument("--client-processes", type=int, default=1,
                        help="use multiple listener processes for a diagnostic fan-out run")
    parser.add_argument("--diagnostic-wait-seconds", type=int, default=60)
    return parser


def main() -> int:
    parser = build_parser()
    args = parser.parse_args()
    if args.recovery_users is None:
        args.recovery_users = min(100, max(1, args.users // 50)) if args.users > 1 else 0
    report = {"status": "PLAN", "scope": "5000 live Layer 229 recipients in one broadcast channel and one megagroup",
              "productionCapacityAccepted": False,
              "requested": {"users": args.users, "backendInstances": args.backend_instances,
                            "syncPartitions": args.sync_partitions,
                            "postsPerTarget": 1, "recoveryUsersPerTarget": args.recovery_users,
                            "liveDeliveryBudgetMs": 2000, "ackBudgetMs": 1000},
              "started_at": dt.datetime.now(dt.timezone.utc).isoformat()}
    if args.client_processes > 1:
        report["scope"] = "multi-process Layer 229 listener diagnostic for one broadcast channel and one megagroup"
    session_file = args.output.parent / f"{secrets.token_hex(12)}-fanout-online-sessions.json"
    channels_file = args.output.parent / f"{secrets.token_hex(12)}-fanout-channels.json"
    lab = None
    try:
        if not 1 <= args.users <= 5000 or not 1 <= args.batch_size <= 200:
            raise ProbeError("users must be 1..5000 and batch-size 1..200")
        if not 1 <= args.client_processes <= 16 or not 5 <= args.diagnostic_wait_seconds <= 600:
            raise ProbeError("client-processes must be 1..16 and diagnostic-wait-seconds 5..600")
        if not 0 <= args.recovery_users < args.users:
            raise ProbeError("recovery-users must be 0..users-1")
        if not 6 <= args.sync_partitions <= 96:
            raise ProbeError("sync-partitions must be 6..96")
        if not 1000 <= args.timeout_ms <= 60000 or not 5 <= args.delivery_timeout_seconds <= 600:
            raise ProbeError("timeout-ms must be 1000..60000 and delivery-timeout-seconds 5..600")
        if args.base_user_id <= 0 or args.base_user_id + args.users > 9000000:
            raise ProbeError("base-user-id range is invalid")
        ports = [args.postgres_port, args.mt_port, args.kafka_port, args.metrics_port]
        if args.backend_instances == 2:
            ports.append(args.mt_port_2)
        validate_ports(ports)
        lab = Lab("teamgram-reliability-fanout-" + secrets.token_hex(6), port=args.postgres_port,
                  timeout=1200, wait=180, profiles=["messaging", "backend", "monitoring"], env={
                      "TEAMGRAM_RELIABILITY_IMAGE": args.image,
                      "TEAMGRAM_RELIABILITY_MT_PORT": str(args.mt_port),
                      "TEAMGRAM_RELIABILITY_MT_PORT_2": str(args.mt_port_2),
                      "TEAMGRAM_RELIABILITY_PG_MAX_CONNECTIONS": os.getenv(
                          "TEAMGRAM_RELIABILITY_PG_MAX_CONNECTIONS", "300"),
                      "TEAMGRAM_RELIABILITY_KAFKA_PORT": str(args.kafka_port),
                      "TEAMGRAM_RELIABILITY_METRICS_PORT": str(args.metrics_port)})
        report["project"] = lab.project
        if args.execute:
            experiment(lab, args, report, session_file, channels_file)
        else:
            report["output"] = str(args.output)
    except (ProbeError, OSError, ValueError, KeyboardInterrupt) as error:
        report.update(status="FAIL", reason=redact(str(error), lab.secret_values if lab else []))
    finally:
        if lab and args.execute:
            try:
                report["cleanup"] = lab.cleanup()
            except (ProbeError, OSError, ValueError) as error:
                report.update(status="FAIL", cleanup={"status": "FAIL", "reason": redact(str(error), lab.secret_values)})
        session_file.unlink(missing_ok=True)
        channels_file.unlink(missing_ok=True)
        diagnostic_dir = args.output.parent / f".{args.output.stem}-multiprocess"
        if args.client_processes > 1:
            shutil.rmtree(diagnostic_dir, ignore_errors=True)
        report["finished_at"] = dt.datetime.now(dt.timezone.utc).isoformat()
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(json.dumps(report, indent=2, sort_keys=True) + "\n")
    print(json.dumps({key: report[key] for key in ("status", "project", "output", "reason", "acceptance") if key in report}))
    return 0 if report["status"] in {"PLAN", "PASS", "BUDGET_EXCEEDED"} else 1


if __name__ == "__main__":
    raise SystemExit(main())
