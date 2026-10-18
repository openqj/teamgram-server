#!/usr/bin/env python3
"""Run Layer 229 messaging, broker failure and restart acceptance in a disposable lab."""

from __future__ import annotations

import argparse
import base64
from concurrent.futures import ThreadPoolExecutor, TimeoutError as FutureTimeout
import datetime as dt
import hashlib
import json
from pathlib import Path
import re
import secrets
import sys
import tempfile
import time
import urllib.parse
import urllib.request

from postgres_recovery_probe import Lab, ProbeError, redact
from messaging_slo_report import evaluate


ROOT = Path(__file__).resolve().parents[1]


class MessagingProbeError(ProbeError):
    def __init__(self, phase: str, evidence: dict):
        self.evidence = evidence
        error = evidence.get("error", {})
        code = error.get("code") if isinstance(error, dict) else None
        code = code if isinstance(code, str) and re.fullmatch(r"[A-Z][A-Z0-9_]{0,79}", code) else "UNKNOWN_PROBE_FAILURE"
        super().__init__(f"messaging {phase} probe failed: {code}")


def validate_ports(ports: list[int]) -> None:
    if any(not 1024 <= port <= 65535 for port in ports) or len(set(ports)) != len(ports):
        raise ProbeError("lab ports must be distinct and within 1024..65535")


def wait_backend(lab: Lab, service: str = "backend") -> None:
    deadline = time.monotonic() + lab.wait
    while time.monotonic() < deadline:
        item = lab.container(service)
        if item["State"].get("Health", {}).get("Status") == "healthy":
            # Process existence can precede the gateway callback RPC listener.
            # Sync is a Kafka consumer and does not open its configured RPC port.
            # Its delivery readiness is exercised by the protocol probe below.
            readiness = lab.run(lab.command("exec", "-T", service, "bash", "-c",
                "for port in 20010 20030 20120 20110 11443; do timeout 1 bash -c 'exec 3<>/dev/tcp/127.0.0.1/$1' probe \"$port\" || exit 1; done"),
                check=False, timeout=10)
            if readiness.returncode == 0:
                return
        if not item["State"]["Running"]:
            logs = lab.run(["docker", "logs", item["Id"]], check=False)
            lines = [line for line in (logs.stdout + logs.stderr).splitlines()
                     if re.search(r"panic|fatal|error|failed|missing|refusing|exited|bind|in use", line, re.I)]
            detail = redact("\n".join(lines)[:7000], lab.secret_values)
            raise ProbeError(f"isolated {service} exited before becoming healthy: {detail}")
        time.sleep(1)
    raise ProbeError(f"isolated {service} did not become healthy before its deadline")


def seed_accounts(lab: Lab) -> None:
    # These keys exist only in this new test database and in process memory.
    # Seeding tests communication, not SMS/registration/provider acceptance.
    statements = ["BEGIN;"]
    for role, user_id in (("A", 910001), ("B", 910002)):
        key = secrets.token_bytes(256)
        key_id = int.from_bytes(hashlib.sha1(key).digest()[-8:], "little", signed=True)
        access_hash = secrets.randbits(62) + 1
        auth_b64 = base64.b64encode(key).decode()
        lab.env.update({f"{role}_AUTH_B64": auth_b64, f"{role}_USER_ID": str(user_id),
                        f"{role}_ACCESS_HASH": str(access_hash)})
        lab.secret_values.extend([auth_b64, key.hex()])
        statements += [
            f"INSERT INTO users(id,phone,username,first_name,access_hash) VALUES ({user_id},'fixture-{user_id}','reliability_{role.lower()}','Reliability {role}',{access_hash});",
            f"INSERT INTO auth_keys(auth_key_id,body) VALUES ({key_id},decode('{auth_b64}','base64'));",
            f"INSERT INTO auth_key_infos(auth_key_id,auth_key_type,perm_auth_key_id) VALUES ({key_id},0,{key_id});",
            f"INSERT INTO auth_users(auth_key_id,user_id,hash,date_created,date_active) VALUES ({key_id},{user_id},{access_hash},extract(epoch from now())::bigint,extract(epoch from now())::bigint);",
        ]
    statements.append("COMMIT;")
    lab.sql("postgres", "\n".join(statements))


def probe(lab: Lab, gramjs: Path, phase: str, state_file: Path, samples: int, *,
          require_success: bool = True, actor_ports: tuple[int, int] | None = None) -> dict:
    executable = gramjs / "node_modules/.bin/tsx"
    lab.env.update({"TEAMGRAM_GRAMJS_DIR": str(gramjs), "TG_DC_ID": "2", "TG_DC_HOST": "127.0.0.1",
                    "TG_DC_PORT": lab.env["TEAMGRAM_RELIABILITY_MT_PORT"], "SLO_SAMPLES": str(samples),
                    "PROBE_PHASE": phase, "PROBE_TAG": lab.run_id, "PROBE_STATE_FILE": str(state_file)})
    if actor_ports:
        lab.env.update({"A_TG_DC_HOST": "127.0.0.1", "A_TG_DC_PORT": str(actor_ports[0]),
                        "B_TG_DC_HOST": "127.0.0.1", "B_TG_DC_PORT": str(actor_ports[1])})
    else:
        for key in ("A_TG_DC_HOST", "A_TG_DC_PORT", "B_TG_DC_HOST", "B_TG_DC_PORT"):
            lab.env.pop(key, None)
    result = lab.run([str(executable), str(ROOT / "docker/production/probe-cross-account-live-updates.ts")],
                     timeout=max(lab.timeout, samples * 12 + 120, int(lab.env.get("LOAD_SECONDS", "0")) + 180), check=False)
    evidence = None
    for line in result.stdout.splitlines():
        try:
            candidate = json.loads(line)
        except json.JSONDecodeError:
            continue
        if isinstance(candidate, dict) and candidate.get("probe") == "messaging.cross-account-live-updates":
            evidence = candidate
    if evidence is None:
        raise ProbeError("messaging probe returned no structured evidence")
    if evidence.get("phase") != phase or evidence.get("tag") != lab.run_id:
        raise ProbeError("messaging probe evidence did not match this run and phase")
    if not require_success:
        evidence["processExitCode"] = result.returncode
    elif result.returncode or evidence.get("ok") is not True:
        # The TypeScript report emits fixed error codes; raw RPC/debug text is
        # deliberately kept out of the parent report to avoid secret leakage.
        raise MessagingProbeError(phase, evidence)
    return evidence


def prometheus_query(port: int, expression: str) -> list[dict]:
    url = f"http://127.0.0.1:{port}/api/v1/query?" + urllib.parse.urlencode({"query": expression})
    with urllib.request.urlopen(url, timeout=5) as response:
        result = json.load(response)
    if result.get("status") != "success":
        raise ProbeError("Prometheus query failed")
    return result["data"]["result"]


def fresh_outage_sample(sampled_at: float, sent_at: float, now: float) -> bool:
    return sent_at <= sampled_at <= now and now - sampled_at < 25


def wait_kafka(lab: Lab) -> None:
    deadline = time.monotonic() + lab.wait
    while time.monotonic() < deadline:
        result = lab.run(lab.command("exec", "-T", "kafka", "/opt/kafka/bin/kafka-topics.sh",
                                     "--bootstrap-server", "kafka:9092", "--list"), check=False, timeout=15)
        if result.returncode == 0:
            return
        time.sleep(1)
    raise ProbeError("isolated Kafka did not become ready")


def fault_control(lab: Lab, directory: Path, stage: str) -> None:
    target = directory / f"{stage}.json"
    temporary = directory / f".{stage}.json"
    temporary.write_text(json.dumps({"schemaVersion": 1, "tag": lab.run_id, "stage": stage}))
    temporary.replace(target)


def wait_control(lab: Lab, directory: Path, stage: str, future, *, timeout: float = 120) -> dict:
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        target = directory / f"{stage}.json"
        if target.exists():
            value = json.loads(target.read_text())
            if value.get("schemaVersion") != 1 or value.get("tag") != lab.run_id or value.get("stage") != stage:
                raise ProbeError("Kafka fault control did not match this lab run")
            return value
        if future.done():
            evidence = future.result()
            failure = evidence.get("error") if isinstance(evidence, dict) else None
            if isinstance(failure, dict):
                probe_stage = failure.get("stage")
                code = failure.get("code")
                if (isinstance(probe_stage, str) and re.fullmatch(r"[a-z][a-z0-9-]{0,63}", probe_stage)
                        and isinstance(code, str) and re.fullmatch(r"[A-Z][A-Z0-9_]{0,79}", code)):
                    raise ProbeError(f"messaging probe exited before {stage}: {probe_stage}/{code}")
            raise ProbeError(f"Kafka fault probe exited before {stage}")
        time.sleep(.1)
    raise ProbeError(f"Kafka fault probe did not reach {stage} before its deadline")


def fault_sql(lab: Lab, sent: dict) -> dict:
    messages = sent.get("messages", [])
    if not isinstance(messages, list) or len(messages) != 2 or not all(isinstance(value, dict) for value in messages) or {value.get("sender") for value in messages} != {"A", "B"}:
        raise ProbeError("Kafka fault probe did not provide exactly two directions")
    clauses = []
    for message in messages:
        text, random_id = message.get("text"), message.get("randomId")
        if not isinstance(text, str) or not text.startswith(lab.run_id + ":") or not re.fullmatch(r"[A-Za-z0-9_:-]+", text):
            raise ProbeError("Kafka fault message tag is invalid")
        if not isinstance(random_id, str) or not random_id.isdecimal() or not 0 < int(random_id) < 2**63:
            raise ProbeError("Kafka fault message random_id is invalid")
        sender = int(lab.env[f"{message['sender']}_USER_ID"])
        clauses.append(f"(m.sender_user_id={sender} AND m.random_id={int(random_id)} AND m.message='{text}')")
    where = " OR ".join(clauses)
    snapshot = json.loads(lab.sql("postgres", f"""SELECT json_build_object(
        'sender_rows',(SELECT count(*) FROM messages m WHERE m.user_id=m.sender_user_id AND ({where})),
        'recipient_rows',(SELECT count(*) FROM messages m WHERE m.user_id<>m.sender_user_id AND ({where})),
        'pending_intents',(SELECT count(*) FROM msg_inbox_delivery_outbox d JOIN messages m
           ON m.user_id=d.sender_user_id AND m.user_message_box_id=d.sender_message_id
           WHERE d.state=0 AND ({where})));"""))
    if snapshot != {"sender_rows": 2, "recipient_rows": 0, "pending_intents": 4}:
        raise ProbeError("Kafka outage did not retain the exact committed sender messages and pending intents")
    return snapshot


def kafka_fault(lab: Lab, args: argparse.Namespace, gramjs: Path, state_file: Path) -> dict:
    evidence = {}
    with tempfile.TemporaryDirectory(prefix=f"{lab.project}-control-") as temporary:
        directory = Path(temporary)
        lab.env["PROBE_CONTROL_DIR"] = temporary
        with ThreadPoolExecutor(max_workers=1) as executor:
            future = executor.submit(probe, lab, gramjs, "fault", state_file, 1)
            stopped = False
            try:
                wait_control(lab, directory, "ready", future)
                lab.stop("kafka")
                if lab.container("kafka")["State"]["Running"]:
                    raise ProbeError("isolated Kafka remained running after stop")
                stopped = True
                stopped_at = time.monotonic()
                fault_control(lab, directory, "stopped")
                sent = wait_control(lab, directory, "sent", future)
                sent_at = time.time()
                evidence["database_during_outage"] = fault_sql(lab, sent)
                deadline = time.monotonic() + 40
                while time.monotonic() < deadline:
                    series = prometheus_query(args.metrics_port, 'teamgram_delivery_pending{job="messenger.msg",queue="inbox"}')
                    stamp = prometheus_query(args.metrics_port, 'teamgram_delivery_backlog_sample_timestamp_seconds{job="messenger.msg"}')
                    observed_at = time.time()
                    if series and stamp and float(series[0]["value"][1]) >= 4 and fresh_outage_sample(float(stamp[0]["value"][1]), sent_at, observed_at):
                        evidence["independent_backlog_sampler"] = {"pending": float(series[0]["value"][1]), "fresh_during_outage": True,
                            "sent_observed_at": sent_at, "sampled_at": float(stamp[0]["value"][1]), "observed_at": observed_at}
                        break
                    time.sleep(.5)
                else:
                    raise ProbeError("database backlog sampler did not remain current during Kafka outage")
                lab.start("kafka")
                if not lab.container("kafka")["State"]["Running"]:
                    raise ProbeError("isolated Kafka did not restart")
                stopped = False
                evidence["broker_stopped_seconds"] = round(time.monotonic() - stopped_at, 3)
                ready_started = time.monotonic()
                wait_kafka(lab)
                evidence["broker_readiness_seconds"] = round(time.monotonic() - ready_started, 3)
                fault_control(lab, directory, "restored")
                try:
                    evidence["probe"] = future.result(timeout=180)
                except FutureTimeout:
                    raise ProbeError("Kafka recovery probe exceeded its deadline") from None
                evidence.update(status="PASS", broker_stop_and_restart_verified=True)
            finally:
                if stopped:
                    lab.start("kafka")
                    fault_control(lab, directory, "restored")
                lab.env.pop("PROBE_CONTROL_DIR", None)
    return evidence


def experiment(lab: Lab, args: argparse.Namespace, report: dict) -> None:
    report["preflight"] = lab.preflight()
    gramjs = args.gramjs_dir.resolve()
    if not (gramjs / "node_modules/.bin/tsx").is_file() or not (gramjs / "src/lib/gramjs/client/TelegramClient.ts").is_file():
        raise ProbeError("--gramjs-dir must contain the Layer 229 Web client and its installed tsx runtime")
    image = lab.run(["docker", "image", "inspect", args.image, "--format", "{{.Id}}"]).stdout.strip()
    report["image"] = {"name": args.image, "id": image}
    lab.start("postgres", "postgres-migrate", "kafka", "redis", "etcd", "seaweedfs")
    lab.compose("wait", "postgres-migrate")
    if lab.container("postgres-migrate")["State"]["ExitCode"] != 0:
        raise ProbeError("isolated migrations failed")
    report["migration_count"] = int(lab.sql("postgres", "SELECT count(*) FROM schema_migrations;"))
    wait_kafka(lab)
    for topic in ("Inbox-T", "Sync-T", "BotFather-T", "reliability-producer-acceptance"):
        lab.compose("exec", "-T", "kafka", "/opt/kafka/bin/kafka-topics.sh", "--bootstrap-server", "kafka:9092",
                    "--create", "--if-not-exists", "--topic", topic, "--partitions", "3", "--replication-factor", "1")

    if args.go_tests:
        password = lab.env["TEAMGRAM_RELIABILITY_DB_PASSWORD"]
        lab.env.update({"MESSENGER_POSTGRES_DSN": f"postgresql://teamgram_reliability:{password}@127.0.0.1:{lab.port}/teamgram_reliability?sslmode=disable",
                        "RELIABILITY_KAFKA_BROKER": f"127.0.0.1:{args.kafka_port}"})
        packages = ["./pkg/mqproducer", "./pkg/mqconsumer", "./app/messenger/msg/internal/dao",
                    "./app/messenger/msg/inbox/internal/core", "./app/messenger/msg/msg/internal/core",
                    "./app/messenger/sync/internal/dao"]
        result = lab.run(["go", "test", "-race", "-json", "-count=1", *packages], timeout=max(lab.timeout, 300))
        passed = [json.loads(line)["Test"] for line in result.stdout.splitlines()
                  if line.startswith("{") and json.loads(line).get("Action") == "pass" and "Test" in json.loads(line)]
        required = {"TestProducerKafkaIntegration", "TestInboxDeliveryPostgresBacklogIncludesDelayedAndExpiredWork",
                    "TestInboxDeliveryPostgresWorkerDrainsOrderedRowsAfterAsyncAck",
                    "TestInboxDeliveryPostgresCompletionMetricCountsOnlyCommittedAck", "TestPostgresMsgSendEntryBatchAndRetry",
                    "TestPostgresInboxConsumerRetryAfterSyncFailure"}
        if not required.issubset(passed):
            raise ProbeError("required PostgreSQL or Kafka integration checks did not run")
        report["go_tests"] = {"status": "PASS", "packages": packages, "passed_tests": passed}

    seed_accounts(lab)
    for service in ("postgres", "kafka", "redis", "etcd", "seaweedfs"):
        if not lab.container(service)["State"]["Running"]:
            raise ProbeError(f"isolated dependency {service} exited during startup")
    lab.start("backend", "prometheus")
    wait_backend(lab)
    state_file = args.output.parent / f"{lab.run_id}-message-state.json"
    report["baseline"] = probe(lab, gramjs, "single", state_file, args.samples)
    report["local_slo"] = evaluate(report["baseline"])
    if report["local_slo"].get("status") != "PASS":
        raise ProbeError("messaging did not meet the local sample and latency acceptance budget")
    report["before_restart"] = probe(lab, gramjs, "before", state_file, 1)
    report["before_restart_checks"] = evaluate(report["before_restart"])
    if report["before_restart_checks"]["status"] != "PRE_RESTART_ONLY":
        raise ProbeError("pre-restart messaging assertions failed")
    before_id = lab.container("backend")["Id"]
    started = time.monotonic()
    lab.compose("up", "-d", "--no-deps", "--force-recreate", "backend")
    wait_backend(lab)
    healthy_seconds = time.monotonic() - started
    after_id = lab.container("backend")["Id"]
    if before_id == after_id:
        raise ProbeError("backend recreation did not replace its container")
    report["after_restart"] = probe(lab, gramjs, "after", state_file, 1)
    report["after_restart_checks"] = evaluate(report["after_restart"])
    if report["after_restart_checks"]["status"] != "RESTART_RECOVERY_ONLY":
        raise ProbeError("post-restart messaging assertions failed")
    report["backend_restart"] = {"status": "PASS", "container_replaced": True,
                                  "process_health_seconds": round(healthy_seconds, 3),
                                  "restart_and_messaging_verification_seconds": round(time.monotonic() - started, 3)}

    lab.wait_sql("postgres", "SELECT (SELECT count(*) FROM msg_inbox_delivery_outbox WHERE state=0)+(SELECT count(*) FROM msg_state_delivery_outbox WHERE state=0);", "0")
    report["kafka_outage"] = kafka_fault(lab, args, gramjs, state_file)
    pending = lab.wait_sql("postgres", "SELECT (SELECT count(*) FROM msg_inbox_delivery_outbox WHERE state=0)+(SELECT count(*) FROM msg_state_delivery_outbox WHERE state=0);", "0")
    report["pending_after_probes"] = int(pending)
    deadline = time.monotonic() + 30
    while time.monotonic() < deadline:
        try:
            series = prometheus_query(args.metrics_port, 'teamgram_delivery_backlog_sample_timestamp_seconds{job="messenger.msg"}')
            if series and time.time() - float(series[0]["value"][1]) < 30:
                report["monitoring"] = {"status": "PASS", "fresh_database_sampler": True,
                                         "pending": prometheus_query(args.metrics_port, 'teamgram_delivery_pending{job="messenger.msg"}')}
                break
        except (OSError, ValueError, ProbeError):
            pass
        time.sleep(1)
    else:
        raise ProbeError("Prometheus did not receive fresh delivery metrics")
    report["dependencies_at_completion"] = {}
    for service in ("postgres", "kafka", "redis", "etcd", "seaweedfs", "backend", "prometheus"):
        state = lab.container(service)["State"]
        health = state.get("Health", {}).get("Status")
        if not state["Running"] or health not in (None, "healthy"):
            raise ProbeError(f"isolated dependency {service} was not healthy at completion")
        report["dependencies_at_completion"][service] = {"running": True, "health": health}
    state_file.unlink(missing_ok=True)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--execute", action="store_true")
    parser.add_argument("--image", default="teamgram-reliability:20261015")
    parser.add_argument("--gramjs-dir", type=Path, default=ROOT.parent / "telegram-tt-master")
    parser.add_argument("--project-name", default=f"teamgram-reliability-msg-{secrets.token_hex(6)}")
    parser.add_argument("--postgres-port", type=int, default=55436)
    parser.add_argument("--mt-port", type=int, default=21543)
    parser.add_argument("--kafka-port", type=int, default=19094)
    parser.add_argument("--metrics-port", type=int, default=19090)
    parser.add_argument("--samples", type=int, default=20)
    parser.add_argument("--timeout", type=float, default=300)
    parser.add_argument("--go-tests", action="store_true")
    parser.add_argument("--output", type=Path, default=Path("/tmp/teamgram-messaging-reliability.json"))
    args = parser.parse_args()
    report = {"status": "PLAN", "scope": "local seeded Layer 229 sessions; not production SLO or login acceptance",
              "project": args.project_name, "output": str(args.output), "started_at": dt.datetime.now(dt.timezone.utc).isoformat()}
    lab = None
    try:
        if not 20 <= args.samples <= 200:
            raise ProbeError("--samples must be 20..200 per direction")
        validate_ports([args.postgres_port, args.mt_port, args.kafka_port, args.metrics_port])
        lab = Lab(args.project_name, port=args.postgres_port, timeout=args.timeout, wait=150,
                  profiles=["messaging", "backend", "monitoring"], env={
                      "TEAMGRAM_RELIABILITY_IMAGE": args.image, "TEAMGRAM_RELIABILITY_MT_PORT": str(args.mt_port),
                      "TEAMGRAM_RELIABILITY_KAFKA_PORT": str(args.kafka_port), "TEAMGRAM_RELIABILITY_METRICS_PORT": str(args.metrics_port)})
        if args.execute:
            experiment(lab, args, report)
            report["status"] = "PASS"
    except (ProbeError, OSError, ValueError, KeyboardInterrupt) as error:
        report.update(status="FAIL", reason=redact(str(error), lab.secret_values if lab else []))
        if isinstance(error, MessagingProbeError):
            report["failed_probe"] = error.evidence
            try:
                item = lab.container("backend")
                logs = lab.run(["docker", "logs", "--tail", "250", item["Id"]], check=False)
                lines = [line for line in (logs.stdout + logs.stderr).splitlines()
                         if re.search(r"panic|fatal|error|failed|missing|AUTH_KEY", line, re.I)]
                report["backend_failure_details"] = redact("\n".join(lines)[-10000:], lab.secret_values)
            except (ProbeError, OSError, ValueError):
                pass
    finally:
        if lab and args.execute:
            try:
                report["cleanup"] = lab.cleanup()
            except (ProbeError, OSError, ValueError) as error:
                report.update(status="FAIL", cleanup={"status": "FAIL", "reason": redact(str(error), lab.secret_values)})
        if lab:
            (args.output.parent / f"{lab.run_id}-message-state.json").unlink(missing_ok=True)
        report["finished_at"] = dt.datetime.now(dt.timezone.utc).isoformat()
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(json.dumps(report, indent=2, sort_keys=True) + "\n")
    print(json.dumps({key: report[key] for key in ("status", "project", "output", "reason") if key in report}))
    return 0 if report["status"] in {"PLAN", "PASS"} else 1


if __name__ == "__main__":
    sys.exit(main())
