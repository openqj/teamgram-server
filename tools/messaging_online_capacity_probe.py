#!/usr/bin/env python3
"""Verify concurrent Layer 229 connections in a disposable local lab."""

from __future__ import annotations

import argparse
import base64
from concurrent.futures import ThreadPoolExecutor
import datetime as dt
import hashlib
import json
from pathlib import Path
import secrets
import time

import messaging_reliability_probe
from messaging_reliability_probe import ROOT, prometheus_query, seed_accounts, validate_ports, wait_backend, wait_kafka
import messaging_capacity_probe as message_capacity
from postgres_recovery_probe import Lab, ProbeError, redact


def seed_online_accounts(lab: Lab, count: int, output: Path, *, base_user_id: int = 920000) -> list[dict[str, str]]:
    sessions: list[dict[str, str]] = []
    users: list[str] = []
    keys: list[str] = []
    infos: list[str] = []
    owners: list[str] = []
    for index in range(count):
        user_id = base_user_id + index
        key = secrets.token_bytes(256)
        key_id = int.from_bytes(hashlib.sha1(key).digest()[-8:], "little", signed=True)
        access_hash = secrets.randbits(62) + 1
        auth_b64 = base64.b64encode(key).decode()
        sessions.append({"userId": str(user_id), "accessHash": str(access_hash), "authB64": auth_b64})
        lab.secret_values.extend([auth_b64, key.hex()])
        users.append(f"({user_id},'fixture-online-{user_id}','online_{user_id}','Online {user_id}',{access_hash})")
        keys.append(f"({key_id},decode('{auth_b64}','base64'))")
        infos.append(f"({key_id},0,{key_id})")
        owners.append(f"({key_id},{user_id},{access_hash},extract(epoch from now())::bigint,extract(epoch from now())::bigint)")
    sql = "BEGIN;"
    sql += "INSERT INTO users(id,phone,username,first_name,access_hash) VALUES " + ",".join(users) + ";"
    sql += "INSERT INTO auth_keys(auth_key_id,body) VALUES " + ",".join(keys) + ";"
    sql += "INSERT INTO auth_key_infos(auth_key_id,auth_key_type,perm_auth_key_id) VALUES " + ",".join(infos) + ";"
    sql += "INSERT INTO auth_users(auth_key_id,user_id,hash,date_created,date_active) VALUES " + ",".join(owners) + ";"
    sql += "COMMIT;"
    lab.sql("postgres", sql)
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(sessions, sort_keys=True) + "\n")
    output.chmod(0o600)
    return sessions


def evaluate(value: dict, requested_users: int, reconnect_count: int, replica_count: int = 1) -> dict:
    online = value.get("online", {})
    connect = online.get("connect", {})
    hold = online.get("hold", {})
    reconnect = online.get("reconnect", {})
    gauge = online.get("serviceGauge", {})
    connected = connect.get("connected")
    minimum = hold.get("minConnected")
    reconnect_ok = reconnect.get("connected")
    gauge_min = hold.get("serverGaugeMin", gauge.get("min"))
    replica_gauge_mins = hold.get("serverGaugeByReplicaMin", {})
    expected_replica_counts = {
        f"backend-{index + 1}": requested_users // replica_count + (index < requested_users % replica_count)
        for index in range(replica_count)
    }
    checks = {
        "allConnectionsAuthenticated": connected == requested_users,
        "allConnectionsHeld": minimum == requested_users,
        "reconnectSample": reconnect_ok == reconnect_count,
        "serverActiveGauge": bool(gauge.get("observed")) and isinstance(gauge_min, (int, float)) and gauge_min >= requested_users,
        "connectionCleanup": gauge.get("cleanupObserved") is True,
        "allReplicasServeExpectedConnections": all(
            replica_gauge_mins.get(replica, -1) >= count
            for replica, count in expected_replica_counts.items()
        ),
        "probeOk": online.get("ok") is True,
    }
    return {"status": "PASS" if all(checks.values()) else "FAIL", "checks": checks,
            "requestedUsers": requested_users, "connected": connected,
            "minimumHeld": minimum, "reconnected": reconnect_ok, "serverGaugeMin": gauge_min,
            "expectedReplicaConnections": expected_replica_counts,
            "minimumReplicaGauge": replica_gauge_mins}


def evaluate_mixed(online: dict, load: dict, users: int, reconnect_count: int,
                   replica_count: int = 1) -> dict:
    online_result = evaluate({"online": online}, users, reconnect_count, replica_count)
    load_result = message_capacity.evaluate(load)
    checks = {"onlineSessions": online_result["status"] == "PASS",
              "messageLoad": load_result["status"] == "PASS"}
    return {"status": "PASS" if all(checks.values()) else "FAIL", "checks": checks,
            "online": online_result, "load": load_result}


def wait_for_online_gauge(lab: Lab, port: int, users: int, future) -> float:
    deadline = time.monotonic() + max(120, lab.wait + 60)
    while time.monotonic() < deadline:
        active = sum(float(row["value"][1]) for row in prometheus_query(port, "gnetway_conn_active"))
        if active >= users:
            return active
        if future.done():
            raise ProbeError("online session probe exited before the requested connection gauge was reached")
        time.sleep(0.5)
    raise ProbeError("online session connection gauge did not reach the requested user count")


def online_probe_command(lab: Lab, executable: Path, args: argparse.Namespace, session_file: Path) -> list[str]:
    lab.env.update({
        "TEAMGRAM_GRAMJS_DIR": str(args.gramjs_dir.resolve()),
        "ONLINE_SESSIONS_FILE": str(session_file),
        "ONLINE_USERS": str(args.users),
        "ONLINE_BATCH_SIZE": str(args.batch_size),
        "ONLINE_HOLD_MS": str(args.effective_hold_seconds * 1000),
        "ONLINE_RECONNECT_COUNT": str(args.reconnect_count),
        "ONLINE_TIMEOUT_MS": str(args.timeout_ms),
        "TG_DC_ID": "2",
        "TG_DC_HOST": "127.0.0.1",
        "TG_DC_PORT": str(args.mt_port),
        "METRICS_PORT": str(args.metrics_port),
        "ONLINE_ENDPOINTS": json.dumps([
            {"host": "127.0.0.1", "port": args.mt_port, "replica": "backend-1"},
            *([{"host": "127.0.0.1", "port": args.mt_port_2, "replica": "backend-2"}]
              if args.backend_instances == 2 else []),
        ]),
    })
    return [str(executable), str(ROOT / "docker/production/probe-online-sessions.ts")]


def postgres_connection_snapshot(lab: Lab) -> dict:
    return json.loads(lab.sql("postgres", """SELECT json_build_object(
        'maxConnections',(SELECT setting::integer FROM pg_settings WHERE name='max_connections'),
        'clientConnections',(SELECT count(*) FROM pg_stat_activity WHERE backend_type='client backend'),
        'remainingConfiguredSlots',(SELECT setting::integer FROM pg_settings WHERE name='max_connections')
          -(SELECT count(*) FROM pg_stat_activity WHERE backend_type='client backend'));"""))


def parse_online_result(result) -> dict:
    for line in result.stdout.splitlines():
        try:
            candidate = json.loads(line)
        except json.JSONDecodeError:
            continue
        if isinstance(candidate, dict) and candidate.get("probe") == "messaging.online-sessions":
            return candidate
    raise ProbeError("online session probe returned no structured evidence")


def experiment(lab: Lab, args: argparse.Namespace, report: dict) -> None:
    report["preflight"] = lab.preflight()
    gramjs = args.gramjs_dir.resolve()
    executable = gramjs / "node_modules/.bin/tsx"
    if not executable.is_file() or not (gramjs / "src/lib/gramjs/client/TelegramClient.ts").is_file():
        raise ProbeError("--gramjs-dir must contain the Layer 229 Web client and its installed tsx runtime")
    report["image"] = {"name": args.image,
                       "id": lab.run(["docker", "image", "inspect", args.image, "--format", "{{.Id}}"], timeout=30).stdout.strip()}
    lab.start("postgres", "postgres-migrate", "kafka", "redis", "etcd", "seaweedfs")
    lab.compose("wait", "postgres-migrate")
    if lab.container("postgres-migrate")["State"].get("ExitCode") != 0:
        raise ProbeError("isolated migrations failed")
    report["migration_count"] = int(lab.sql("postgres", "SELECT count(*) FROM schema_migrations;"))
    wait_kafka(lab)
    for topic in ("Inbox-T", "Sync-T", "BotFather-T", "reliability-producer-acceptance"):
        lab.compose("exec", "-T", "kafka", "/opt/kafka/bin/kafka-topics.sh", "--bootstrap-server", "kafka:9092",
                    "--create", "--if-not-exists", "--topic", topic, "--partitions", "6", "--replication-factor", "1")
    if args.mix_rate is not None or args.backend_instances == 2:
        seed_accounts(lab)
    session_file = args.output.parent / f"{lab.run_id}-online-sessions.json"
    report["seed"] = {"users": args.users, "baseUserId": args.base_user_id,
                       "sessionFile": str(session_file), "status": "PASS"}
    seed_online_accounts(lab, args.users, session_file, base_user_id=args.base_user_id)
    report["seed"]["authRows"] = int(lab.sql("postgres", "SELECT count(*) FROM auth_users WHERE user_id >= %d AND user_id < %d;" %
                                               (args.base_user_id, args.base_user_id + args.users)))
    if report["seed"]["authRows"] != args.users:
        raise ProbeError("online account seeding did not create the requested auth rows")
    services = ["backend"] + (["backend-2"] if args.backend_instances == 2 else []) + ["prometheus"]
    lab.start(*services)
    wait_backend(lab)
    if args.backend_instances == 2:
        wait_backend(lab, "backend-2")
    command = online_probe_command(lab, executable, args, session_file)
    if args.mix_rate is not None:
        with ThreadPoolExecutor(max_workers=1) as executor:
            future = executor.submit(lab.run, command,
                                     timeout=max(lab.timeout, args.users // max(1, args.batch_size) * args.timeout_ms / 1000
                                                 + args.effective_hold_seconds + 180), check=False)
            try:
                report["mixedLoad"] = {"connectionGaugeAtStart": wait_for_online_gauge(
                    lab, args.metrics_port, args.users, future)}
                load_evidence, pending = message_capacity.run_profile(
                    lab, gramjs, args.mix_rate, args.mix_seconds, args.mix_max_inflight,
                    args.output.parent / f"{lab.run_id}-mixed-capacity-state.json", args.metrics_port,
                    actor_ports=(args.mt_port, args.mt_port_2) if args.backend_instances == 2 else None)
                report["mixedLoad"].update({"pendingAfterDrain": pending, "evidence": load_evidence})
            finally:
                result = future.result()
                evidence = parse_online_result(result)
                report["online"] = evidence
                if result.returncode:
                    report["onlineProbeError"] = evidence.get("error", {}).get("code", f"EXIT_{result.returncode}")
    else:
        result = lab.run(command,
                         timeout=max(lab.timeout, args.users // max(1, args.batch_size) * args.timeout_ms / 1000
                                     + args.effective_hold_seconds + 180), check=False)
        try:
            evidence = parse_online_result(result)
        except ProbeError as error:
            detail = redact((result.stderr or result.stdout)[-5000:], lab.secret_values)
            raise ProbeError(f"{error}: {detail}") from None
    report["online"] = evidence
    report["acceptance"] = evaluate(report, args.users, args.reconnect_count, args.backend_instances)
    if (report.get("onlineProbeError") or report["acceptance"]["status"] != "PASS"):
        raise ProbeError(f"{args.users}-online-session acceptance budget was not met")
    if args.mix_rate is not None:
        report["mixedLoadAcceptance"] = evaluate_mixed(
            evidence, load_evidence, args.users, args.reconnect_count, args.backend_instances)
        if report["mixedLoadAcceptance"]["status"] != "PASS":
            report["status"] = "MIXED_LOAD_BUDGET_EXCEEDED"
    report["postgresDuringLoad"] = postgres_connection_snapshot(lab)
    if args.backend_instances == 2:
        state_file = args.output.parent / f"{lab.run_id}-replica-failover-state.json"
        try:
            lab.env["TEAMGRAM_RELIABILITY_MT_PORT"] = str(args.mt_port_2)
            before = messaging_reliability_probe.probe(lab, gramjs, "before", state_file, 1)
            if messaging_reliability_probe.evaluate(before).get("status") != "PRE_RESTART_ONLY":
                raise ProbeError("replica failover pre-state did not pass Layer 229 checks")
            pending = lab.wait_sql("postgres", "SELECT (SELECT count(*) FROM msg_inbox_delivery_outbox WHERE state=0)+(SELECT count(*) FROM msg_state_delivery_outbox WHERE state=0);", "0")
            second = lab.container("backend-2")
            lab.compose("stop", "--timeout", "45", "backend-2")
            stopped = lab.container("backend-2")
            if stopped["State"]["Running"] or stopped["HostConfig"]["RestartPolicy"]["Name"] not in {"", "no"}:
                raise ProbeError("backend-2 did not remain stopped; refusing survivor recovery test")
            lab.env["TEAMGRAM_RELIABILITY_MT_PORT"] = str(args.mt_port)
            after = messaging_reliability_probe.probe(lab, gramjs, "after", state_file, 1)
            if messaging_reliability_probe.evaluate(after).get("status") != "RESTART_RECOVERY_ONLY":
                raise ProbeError("surviving backend did not recover the saved Layer 229 update state")
            remaining = lab.wait_sql("postgres", "SELECT (SELECT count(*) FROM msg_inbox_delivery_outbox WHERE state=0)+(SELECT count(*) FROM msg_state_delivery_outbox WHERE state=0);", "0")
            report["replicaFailover"] = {
                "status": "PASS", "failedReplica": "backend-2", "survivor": "backend-1",
                "failedContainerId": second["Id"], "stoppedContainerId": stopped["Id"],
                "backend2Stopped": True, "restartDisabled": True,
                "preFailurePending": int(pending), "postRecoveryPending": int(remaining),
                "before": before, "after": after,
                "routing": "probe reconnected directly to the surviving local endpoint",
            }
        finally:
            state_file.unlink(missing_ok=True)
    report["postgresAtCompletion"] = postgres_connection_snapshot(lab)
    report["dependencies_at_completion"] = {}
    services = ["postgres", "kafka", "redis", "etcd", "seaweedfs", "backend", "prometheus"]
    if args.backend_instances == 2:
        services.append("backend-2")
    for service in services:
        state = lab.container(service)["State"]
        health = state.get("Health", {}).get("Status")
        expected_stopped = service == "backend-2" and args.backend_instances == 2
        if expected_stopped:
            if state["Running"]:
                raise ProbeError(f"capacity dependency {service} unexpectedly restarted")
        elif not state["Running"] or health not in (None, "healthy"):
            raise ProbeError(f"capacity dependency {service} is not healthy")
        report["dependencies_at_completion"][service] = {"running": state["Running"], "health": health}
    if report["status"] != "MIXED_LOAD_BUDGET_EXCEEDED":
        report["status"] = "PASS"
    session_file.unlink(missing_ok=True)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--execute", action="store_true")
    parser.add_argument("--users", type=int, default=1000)
    parser.add_argument("--backend-instances", type=int, choices=(1, 2), default=1)
    parser.add_argument("--hold-seconds", type=int, default=20)
    parser.add_argument("--batch-size", type=int, default=50)
    parser.add_argument("--reconnect-count", type=int, default=10)
    parser.add_argument("--timeout-ms", type=int, default=20000)
    parser.add_argument("--mix-rate", type=int)
    parser.add_argument("--mix-seconds", type=int, default=20)
    parser.add_argument("--mix-max-inflight", type=int, default=32)
    parser.add_argument("--base-user-id", type=int, default=920000)
    parser.add_argument("--image", default="teamgram-reliability:20261015-online-v1")
    parser.add_argument("--gramjs-dir", type=Path, default=ROOT.parent / "telegram-tt-master")
    parser.add_argument("--output", type=Path, default=Path("/tmp/teamgram-online-capacity.json"))
    parser.add_argument("--postgres-port", type=int, default=55439)
    parser.add_argument("--mt-port", type=int, default=21545)
    parser.add_argument("--mt-port-2", type=int, default=21546)
    parser.add_argument("--kafka-port", type=int, default=19096)
    parser.add_argument("--metrics-port", type=int, default=19092)
    args = parser.parse_args()
    report = {"status": "PLAN", "scope": "local seeded Layer 229 concurrent online sessions",
              "productionCapacityAccepted": False, "requested": {"users": args.users,
              "holdSeconds": args.hold_seconds, "batchSize": args.batch_size,
              "reconnectCount": args.reconnect_count, "backendInstances": args.backend_instances},
              "started_at": dt.datetime.now(dt.timezone.utc).isoformat()}
    lab = None
    try:
        if not 1 <= args.users <= 5000 or not 5 <= args.hold_seconds <= 120 or not 1 <= args.batch_size <= 200:
            raise ProbeError("users 1..5000, hold-seconds 5..120 and batch-size 1..200 are required")
        if not 0 <= args.reconnect_count <= args.users or not 1000 <= args.timeout_ms <= 60000:
            raise ProbeError("reconnect-count and timeout-ms are outside the accepted range")
        args.effective_hold_seconds = max(args.hold_seconds, args.mix_seconds + 30) if args.mix_rate is not None else args.hold_seconds
        if args.mix_rate is not None and (args.mix_rate <= 0 or not 1 <= args.mix_seconds <= 60
                              or args.mix_rate * args.mix_seconds > 20000
                              or not 1 <= args.mix_max_inflight <= 256 or args.effective_hold_seconds > 120):
            raise ProbeError("mixed load requires a positive rate, 1..60 seconds, <=20000 messages, inflight 1..256, and <=120 seconds online hold")
        report["requested"]["mixedLoad"] = ({"rate": args.mix_rate, "seconds": args.mix_seconds,
                                               "maxInflight": args.mix_max_inflight,
                                               "effectiveHoldSeconds": args.effective_hold_seconds}
                                              if args.mix_rate is not None else None)
        ports = [args.postgres_port, args.mt_port, args.kafka_port, args.metrics_port]
        if args.backend_instances == 2:
            ports.append(args.mt_port_2)
        validate_ports(ports)
        lab = Lab("teamgram-reliability-online-" + secrets.token_hex(6), port=args.postgres_port, timeout=600, wait=180,
                  profiles=["messaging", "backend", "monitoring"], env={
                      "TEAMGRAM_RELIABILITY_IMAGE": args.image,
                      "TEAMGRAM_RELIABILITY_MT_PORT": str(args.mt_port),
                      "TEAMGRAM_RELIABILITY_MT_PORT_2": str(args.mt_port_2),
                      "TEAMGRAM_RELIABILITY_PG_MAX_CONNECTIONS": "300",
                      "TEAMGRAM_RELIABILITY_KAFKA_PORT": str(args.kafka_port),
                      "TEAMGRAM_RELIABILITY_METRICS_PORT": str(args.metrics_port)})
        report["project"] = lab.project
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
        seeded_file = report.get("seed", {}).get("sessionFile")
        if seeded_file:
            Path(seeded_file).unlink(missing_ok=True)
        report["finished_at"] = dt.datetime.now(dt.timezone.utc).isoformat()
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(json.dumps(report, indent=2, sort_keys=True) + "\n")
    print(json.dumps({key: report[key] for key in ("status", "project", "output", "reason", "acceptance") if key in report}))
    return 0 if report["status"] in {"PLAN", "PASS"} else 1


if __name__ == "__main__":
    raise SystemExit(main())
