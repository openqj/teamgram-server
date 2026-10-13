#!/usr/bin/env python3
"""Run a guarded backend restart barrier and save reviewable evidence.

The default mode is a plan/check and never calls Docker.  ``--execute`` is
required before a Compose restart is attempted.  A restart by itself is
reported as ``BARRIER_ONLY``; it becomes ``PASS`` only when supplied before
and after commands succeed (and an optional cleanup/SQL capture also passes).
"""

from __future__ import annotations

import argparse
import datetime as dt
import hashlib
import json
import os
from pathlib import Path
import shlex
import shutil
import subprocess
import sys
import time
from typing import Any


ROOT = Path(__file__).resolve().parents[1]


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--compose-file", default=os.environ.get("ACCEPTANCE_COMPOSE_FILE", ""))
    parser.add_argument("--service", default=os.environ.get("ACCEPTANCE_SERVICE", ""))
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument(
        "--before-command",
        help="Shell command that creates/tag-checks state before the restart.",
    )
    parser.add_argument(
        "--after-command",
        help="Shell command that reconnects/read-checks state after the restart.",
    )
    parser.add_argument(
        "--cleanup-command",
        help="Shell command that restores/deletes probe-owned state; always attempted after execution.",
    )
    parser.add_argument(
        "--sql-file",
        type=Path,
        help="Optional read-only SQL file to capture before and after restart.",
    )
    parser.add_argument(
        "--dsn-env",
        default="ACCEPTANCE_DATABASE_URL",
        help="Environment variable containing the PostgreSQL DSN; never written to evidence.",
    )
    parser.add_argument("--wait-seconds", type=float, default=90.0)
    parser.add_argument("--poll-seconds", type=float, default=2.0)
    parser.add_argument("--timeout", type=float, default=300.0)
    parser.add_argument("--execute", action="store_true", help="Actually invoke Docker Compose restart.")
    parser.add_argument("--json", action="store_true", help="Print the full evidence JSON after writing it.")
    return parser.parse_args()


def sha256_text(value: str) -> str:
    return hashlib.sha256(value.encode("utf-8", errors="replace")).hexdigest()


def output_text(value: Any) -> str:
    if value is None:
        return ""
    if isinstance(value, (bytes, bytearray)):
        return value.decode(errors="replace")
    return str(value)


def record_command(
    command: list[str] | str,
    *,
    shell: bool = False,
    timeout: float,
    env: dict[str, str] | None = None,
) -> dict[str, Any]:
    started = time.monotonic()
    display = command if isinstance(command, str) else list(command)
    record: dict[str, Any] = {
        "command": display,
        "shell": shell,
    }
    try:
        completed = subprocess.run(
            command,
            cwd=ROOT,
            env=env,
            shell=shell,
            text=True,
            capture_output=True,
            timeout=timeout,
            check=False,
        )
        stdout = completed.stdout or ""
        stderr = completed.stderr or ""
        record.update({
            "exit_code": completed.returncode,
            "ok": completed.returncode == 0,
            "duration_seconds": round(time.monotonic() - started, 3),
            "stdout": stdout,
            "stderr": stderr,
            "stdout_sha256": sha256_text(stdout),
            "stderr_sha256": sha256_text(stderr),
        })
    except subprocess.TimeoutExpired as error:
        stdout = output_text(error.stdout)
        stderr = output_text(error.stderr)
        record.update({
            "exit_code": None,
            "ok": False,
            "timeout": True,
            "duration_seconds": round(time.monotonic() - started, 3),
            "stdout": stdout,
            "stderr": stderr,
            "stdout_sha256": sha256_text(stdout),
            "stderr_sha256": sha256_text(stderr),
        })
    except OSError as error:
        record.update({
            "exit_code": None,
            "ok": False,
            "duration_seconds": round(time.monotonic() - started, 3),
            "stdout": "",
            "stderr": str(error),
            "stderr_sha256": sha256_text(str(error)),
        })
    return record


def compose_command(compose_file: str, *args: str) -> list[str]:
    compose_bin = os.environ.get("ACCEPTANCE_COMPOSE_BIN", "docker compose")
    return [*shlex.split(compose_bin), "-f", compose_file, *args]


def parse_ps_output(value: str) -> list[dict[str, Any]]:
    text = value.strip()
    if not text:
        return []
    try:
        parsed = json.loads(text)
        if isinstance(parsed, list):
            return [item for item in parsed if isinstance(item, dict)]
        if isinstance(parsed, dict):
            return [parsed]
    except json.JSONDecodeError:
        pass
    # Older Compose versions emit one JSON object per line.
    records = []
    for line in text.splitlines():
        try:
            parsed = json.loads(line)
        except json.JSONDecodeError:
            continue
        if isinstance(parsed, dict):
            records.append(parsed)
    return records


def service_ready(ps_record: dict[str, Any]) -> tuple[bool, str]:
    state = str(ps_record.get("State", ps_record.get("state", ""))).lower()
    health = str(ps_record.get("Health", ps_record.get("health", ""))).lower()
    if state not in {"running", "up"}:
        return False, f"service state is {state or 'unknown'}"
    if health and health not in {"healthy", "running", "up"}:
        return False, f"service health is {health}"
    return True, "running"


def capture_sql(
    sql_file: Path,
    dsn_env: str,
    label: str,
    timeout: float,
) -> dict[str, Any]:
    record: dict[str, Any] = {
        "label": label,
        "sql_file": str(sql_file),
        "status": "BLOCKED",
    }
    if not sql_file.is_file():
        record["reason"] = f"SQL file not found: {sql_file}"
        return record
    sql = sql_file.read_text(encoding="utf-8")
    dsn = os.environ.get(dsn_env, "").strip()
    has_pg_environment = any(
        os.environ.get(name)
        for name in ("PGHOST", "PGPORT", "PGDATABASE", "PGUSER", "PGSERVICE")
    )
    record.update({
        "sql_sha256": sha256_text(sql),
        "dsn_env": dsn_env,
        "dsn_configured": bool(dsn or has_pg_environment),
    })
    if not dsn and not has_pg_environment:
        record["reason"] = f"{dsn_env} is unset and no PG* connection environment is configured"
        return record
    psql = shutil.which("psql")
    if not psql:
        record["reason"] = "psql executable not found"
        return record
    command = [psql]
    if dsn:
        command.append(dsn)
    command.extend(["--no-psqlrc", "--set=ON_ERROR_STOP=1", "--quiet", "--file", str(sql_file)])
    display = [psql]
    if dsn:
        display.extend(["<redacted-dsn>", *command[2:]])
    else:
        display.extend(command[1:])
    record["command"] = display
    started = time.monotonic()
    try:
        completed = subprocess.run(
            command,
            cwd=ROOT,
            text=True,
            capture_output=True,
            timeout=timeout,
            check=False,
        )
        stdout = completed.stdout or ""
        stderr = completed.stderr or ""
        record.update({
            "status": "PASS" if completed.returncode == 0 else "FAIL",
            "exit_code": completed.returncode,
            "duration_seconds": round(time.monotonic() - started, 3),
            "stdout": stdout,
            "stderr": stderr,
            "stdout_sha256": sha256_text(stdout),
            "stderr_sha256": sha256_text(stderr),
        })
    except subprocess.TimeoutExpired as error:
        stdout = output_text(error.stdout)
        stderr = output_text(error.stderr)
        record.update({
            "status": "TIMEOUT",
            "exit_code": None,
            "duration_seconds": round(time.monotonic() - started, 3),
            "stdout": stdout,
            "stderr": stderr,
            "stdout_sha256": sha256_text(stdout),
            "stderr_sha256": sha256_text(stderr),
        })
    except OSError as error:
        record.update({"status": "FAIL", "exit_code": None, "stderr": str(error)})
    return record


def write_report(path: Path, report: dict[str, Any]) -> None:
    path.expanduser().resolve().parent.mkdir(parents=True, exist_ok=True)
    path.expanduser().resolve().write_text(
        json.dumps(report, ensure_ascii=False, indent=2, sort_keys=True) + "\n", encoding="utf-8"
    )


def main() -> int:
    args = parse_args()
    output = args.output.expanduser().resolve()
    generated_at = dt.datetime.now(dt.timezone.utc).isoformat()
    report: dict[str, Any] = {
        "schema_version": 1,
        "generated_at": generated_at,
        "mode": "execute" if args.execute else "plan",
        "status": "PLAN",
        "compose_file": str(Path(args.compose_file).expanduser().resolve()) if args.compose_file else "",
        "service": args.service,
        "before_command": args.before_command or "",
        "after_command": args.after_command or "",
        "cleanup_command": args.cleanup_command or "",
        "steps": [],
    }

    if not args.compose_file or not args.service:
        report["status"] = "BLOCKED"
        report["reason"] = "--compose-file and --service (or ACCEPTANCE_* variables) are required"
        write_report(output, report)
        print(report["reason"], file=sys.stderr)
        return 2
    compose_file = Path(args.compose_file).expanduser().resolve()
    if not compose_file.is_file():
        report["status"] = "BLOCKED"
        report["reason"] = f"Compose file not found: {compose_file}"
        write_report(output, report)
        print(report["reason"], file=sys.stderr)
        return 2

    config_command = compose_command(str(compose_file), "config", "--services")
    report["planned_commands"] = {
        "config": config_command,
        "ps": compose_command(str(compose_file), "ps", "--all", "--format", "json", args.service),
        "restart": compose_command(str(compose_file), "restart", args.service),
    }
    if not args.execute:
        report["reason"] = "dry-run; pass --execute to invoke Docker Compose"
        write_report(output, report)
        if args.json:
            print(json.dumps(report, ensure_ascii=False, indent=2, sort_keys=True))
        else:
            print(json.dumps({"status": report["status"], "output": str(output)}, sort_keys=True))
        return 2

    if not shutil.which(shlex.split(os.environ.get("ACCEPTANCE_COMPOSE_BIN", "docker compose"))[0]):
        report["status"] = "BLOCKED"
        report["reason"] = "Docker Compose executable not found"
        write_report(output, report)
        print(report["reason"], file=sys.stderr)
        return 2

    config = record_command(config_command, timeout=args.timeout)
    report["steps"].append({"name": "compose_config", **config})
    services = config.get("stdout", "").splitlines()
    if not config.get("ok") or args.service not in services:
        report["status"] = "FAIL"
        report["reason"] = f"service {args.service!r} is not present in compose config"
        write_report(output, report)
        return 1

    if args.before_command:
        before = record_command(args.before_command, shell=True, timeout=args.timeout)
        report["steps"].append({"name": "before", **before})
        if not before.get("ok"):
            report["status"] = "FAIL"
            report["reason"] = "before command failed; restart was not attempted"
            if args.cleanup_command:
                report["steps"].append({
                    "name": "cleanup",
                    **record_command(args.cleanup_command, shell=True, timeout=args.timeout),
                })
            write_report(output, report)
            return 1

    if args.sql_file:
        report["steps"].append(capture_sql(args.sql_file.expanduser().resolve(), args.dsn_env, "before", args.timeout))

    ps_before = record_command(
        compose_command(str(compose_file), "ps", "--all", "--format", "json", args.service),
        timeout=args.timeout,
    )
    report["steps"].append({"name": "ps_before", **ps_before})

    restart = record_command(
        compose_command(str(compose_file), "restart", args.service),
        timeout=args.timeout,
    )
    report["steps"].append({"name": "restart", **restart})
    if not restart.get("ok"):
        report["status"] = "FAIL"
        report["reason"] = "compose restart failed"
    else:
        deadline = time.monotonic() + max(0.0, args.wait_seconds)
        barrier: dict[str, Any] = {"name": "restart_barrier", "ok": False, "polls": []}
        while True:
            poll = record_command(
                compose_command(str(compose_file), "ps", "--all", "--format", "json", args.service),
                timeout=min(args.timeout, max(1.0, args.poll_seconds)),
            )
            parsed = parse_ps_output(poll.get("stdout", ""))
            ready = False
            reason = "no service record"
            for item in parsed:
                ready, reason = service_ready(item)
                if ready:
                    break
            barrier["polls"].append({"command": poll, "services": parsed, "ready": ready, "reason": reason})
            if ready:
                barrier["ok"] = True
                barrier["reason"] = reason
                break
            if time.monotonic() >= deadline:
                barrier["reason"] = reason
                break
            time.sleep(max(0.1, args.poll_seconds))
        report["steps"].append(barrier)
        if not barrier["ok"]:
            report["status"] = "FAIL"

    if args.sql_file:
        report["steps"].append(capture_sql(args.sql_file.expanduser().resolve(), args.dsn_env, "after", args.timeout))

    if args.after_command:
        after = record_command(args.after_command, shell=True, timeout=args.timeout)
        report["steps"].append({"name": "after", **after})
        if not after.get("ok"):
            report["status"] = "FAIL"

    if args.cleanup_command:
        cleanup = record_command(args.cleanup_command, shell=True, timeout=args.timeout)
        report["steps"].append({"name": "cleanup", **cleanup})
        if not cleanup.get("ok"):
            report["status"] = "FAIL"

    has_business_assertion = bool(args.before_command and args.after_command)
    if report["status"] != "FAIL":
        report["status"] = "PASS" if has_business_assertion else "BARRIER_ONLY"
        if not has_business_assertion:
            report["reason"] = "restart barrier passed, but before/after business assertions were not supplied"
    write_report(output, report)
    if args.json:
        print(json.dumps(report, ensure_ascii=False, indent=2, sort_keys=True))
    else:
        print(json.dumps({"status": report["status"], "output": str(output)}, sort_keys=True))
    return 0 if report["status"] == "PASS" else (2 if report["status"] in {"PLAN", "BLOCKED", "BARRIER_ONLY"} else 1)


if __name__ == "__main__":
    raise SystemExit(main())
