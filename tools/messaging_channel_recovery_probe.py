#!/usr/bin/env python3
"""Validate Layer 229 channel difference recovery in a disposable local lab."""

from __future__ import annotations

import argparse
import datetime as dt
import json
from pathlib import Path
import secrets
import sys

from messaging_reliability_probe import ROOT, seed_accounts, validate_ports, wait_backend, wait_kafka
from postgres_recovery_probe import Lab, ProbeError, redact


def run_probe(lab: Lab, gramjs: Path, timeout: int, report: dict) -> dict:
    executable = gramjs / "node_modules/.bin/tsx"
    if not executable.is_file() or not (gramjs / "src/lib/gramjs/client/TelegramClient.ts").is_file():
        raise ProbeError("--gramjs-dir must contain the Layer 229 Web client and installed tsx runtime")
    lab.env.update({
        "TEAMGRAM_GRAMJS_DIR": str(gramjs),
        "TG_DC_ID": "2",
        "TG_DC_HOST": "127.0.0.1",
        "TG_DC_PORT": lab.env["TEAMGRAM_RELIABILITY_MT_PORT"],
        "CHANNEL_RECOVERY_TIMEOUT_MS": str(timeout * 1000),
    })
    result = lab.run([str(executable), str(ROOT / "docker/production/probe-channel-difference-recovery.ts")],
                     timeout=timeout + 60, check=False)
    evidence = None
    for line in result.stdout.splitlines():
        try:
            candidate = json.loads(line)
        except json.JSONDecodeError:
            continue
        if isinstance(candidate, dict) and candidate.get("probe") == "messaging.channel-difference-recovery":
            evidence = candidate
    if evidence is None:
        report["recovery_probe_process"] = {
            "returncode": result.returncode,
            "stdout_tail": redact(result.stdout[-4000:], lab.secret_values),
            "stderr_tail": redact(result.stderr[-4000:], lab.secret_values),
        }
        raise ProbeError("channel recovery probe returned no structured evidence")
    report["recovery"] = evidence
    if result.returncode or evidence.get("ok") is not True:
        code = evidence.get("errorCode")
        if not isinstance(code, str) or not code.isupper():
            code = "UNKNOWN_PROBE_FAILURE"
        raise ProbeError(f"channel recovery probe failed: {code}")
    return evidence


def cleanup_snapshot(lab: Lab, channel_id: str) -> dict:
    channel = int(channel_id)
    value = lab.sql("postgres", f"""SELECT json_build_object(
      'channels',(SELECT count(*) FROM apifull_channel WHERE id={channel}),
      'members',(SELECT count(*) FROM apifull_channel_member WHERE channel_id={channel}),
      'messages',(SELECT count(*) FROM apifull_channel_message WHERE channel_id={channel}),
      'requests',(SELECT count(*) FROM apifull_channel_message_request WHERE channel_id={channel}),
      'sequence',(SELECT count(*) FROM apifull_channel_message_seq WHERE channel_id={channel}),
      'events',(SELECT count(*) FROM apifull_channel_event WHERE channel_id={channel}),
      'delivery_outbox',(SELECT count(*) FROM apifull_channel_delivery_outbox WHERE channel_id={channel}),
      'delivery_recipients',(SELECT count(*) FROM apifull_channel_delivery_recipient r
        JOIN apifull_channel_delivery_outbox o ON o.id=r.delivery_id WHERE o.channel_id={channel})
    );""")
    result = json.loads(value)
    if any(result.values()):
        raise ProbeError("temporary channel cleanup left persisted rows")
    return result


def experiment(lab: Lab, args: argparse.Namespace, report: dict) -> None:
    report["preflight"] = lab.preflight()
    report["image"] = {"name": args.image,
                       "id": lab.run(["docker", "image", "inspect", args.image, "--format", "{{.Id}}"], timeout=30).stdout.strip()}
    lab.start("postgres", "postgres-migrate", "kafka", "redis", "etcd", "minio")
    lab.compose("wait", "postgres-migrate")
    if lab.container("postgres-migrate")["State"].get("ExitCode") != 0:
        raise ProbeError("isolated PostgreSQL migrations failed")
    report["migration_count"] = int(lab.sql("postgres", "SELECT count(*) FROM schema_migrations;"))
    wait_kafka(lab)
    for topic in ("Inbox-T", "Sync-T", "BotFather-T"):
        lab.compose("exec", "-T", "kafka", "/opt/kafka/bin/kafka-topics.sh", "--bootstrap-server", "kafka:9092",
                    "--create", "--if-not-exists", "--topic", topic, "--partitions", "6", "--replication-factor", "1")
    seed_accounts(lab)
    lab.start("backend")
    wait_backend(lab)
    run_probe(lab, args.gramjs_dir.resolve(), args.probe_timeout, report)
    report["database_cleanup"] = cleanup_snapshot(lab, report["recovery"]["channelId"])
    for service in ("postgres", "kafka", "redis", "etcd", "minio", "backend"):
        state = lab.container(service)["State"]
        health = state.get("Health", {}).get("Status")
        if not state["Running"] or health not in (None, "healthy"):
            raise ProbeError(f"isolated dependency {service} was not healthy at completion")
    report["dependencies_at_completion"] = {service: {"running": True,
            "health": lab.container(service)["State"].get("Health", {}).get("Status")}
            for service in ("postgres", "kafka", "redis", "etcd", "minio", "backend")}


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--execute", action="store_true")
    parser.add_argument("--image", default="teamgram-reliability:20261015-channel-recovery")
    parser.add_argument("--gramjs-dir", type=Path, default=ROOT.parent / "telegram-tt-master")
    parser.add_argument("--project-name", default=f"teamgram-reliability-channel-{secrets.token_hex(6)}")
    parser.add_argument("--postgres-port", type=int, default=55442)
    parser.add_argument("--mt-port", type=int, default=21548)
    parser.add_argument("--kafka-port", type=int, default=19100)
    parser.add_argument("--probe-timeout", type=int, default=60)
    parser.add_argument("--output", type=Path, default=Path("/tmp/teamgram-channel-difference-recovery.json"))
    args = parser.parse_args()
    report = {"status": "PLAN", "scope": "isolated Layer 229 channel gap recovery; not production SLO",
              "project": args.project_name, "output": str(args.output),
              "started_at": dt.datetime.now(dt.timezone.utc).isoformat()}
    lab = None
    try:
        if not 10 <= args.probe_timeout <= 300:
            raise ProbeError("probe-timeout must be 10..300 seconds")
        validate_ports([args.postgres_port, args.mt_port, args.kafka_port])
        lab = Lab(args.project_name, port=args.postgres_port, timeout=max(300, args.probe_timeout + 180), wait=180,
                  profiles=["messaging", "backend"], env={
                      "TEAMGRAM_RELIABILITY_IMAGE": args.image,
                      "TEAMGRAM_RELIABILITY_MT_PORT": str(args.mt_port),
                      "TEAMGRAM_RELIABILITY_KAFKA_PORT": str(args.kafka_port),
                  })
        if args.execute:
            experiment(lab, args, report)
            report["status"] = "PASS"
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
        report["finished_at"] = dt.datetime.now(dt.timezone.utc).isoformat()
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(json.dumps(report, indent=2, sort_keys=True) + "\n")
    print(json.dumps({key: report[key] for key in ("status", "project", "output", "reason") if key in report}))
    return 0 if report["status"] in {"PLAN", "PASS"} else 1


if __name__ == "__main__":
    sys.exit(main())
