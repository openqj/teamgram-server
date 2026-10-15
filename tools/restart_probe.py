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
import signal
import shutil
import subprocess
import sys
import time
from typing import Any

try:
    from sql_evidence import read_only_sql_issues
except ImportError:  # pragma: no cover - supports importing this file directly
    read_only_sql_issues = None


ROOT = Path(__file__).resolve().parents[1]


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--compose-file",
        dest="compose_files",
        action="append",
        help="Compose file to pass to Docker Compose; repeat for layered files.",
    )
    parser.add_argument(
        "--project-name",
        default=os.environ.get("ACCEPTANCE_COMPOSE_PROJECT", ""),
        help="Existing Compose project name (or ACCEPTANCE_COMPOSE_PROJECT).",
    )
    parser.add_argument(
        "--project-directory",
        default=os.environ.get("ACCEPTANCE_COMPOSE_PROJECT_DIRECTORY", ""),
        help="Existing Compose working directory (or ACCEPTANCE_COMPOSE_PROJECT_DIRECTORY).",
    )
    parser.add_argument("--service", default=os.environ.get("ACCEPTANCE_SERVICE", ""))
    parser.add_argument(
        "--expected-image",
        default=os.environ.get("TEAMGRAM_BACKEND_IMAGE", "").strip(),
        help="Expected image reference; execute mode fails closed when it differs from Compose or the running container.",
    )
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
    parser.add_argument(
        "--allow-barrier-only",
        action="store_true",
        help="Explicitly allow --execute without business before/after assertions (reports BARRIER_ONLY).",
    )
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
    process: subprocess.Popen[str] | None = None
    try:
        process = subprocess.Popen(
            command,
            cwd=ROOT,
            env=env,
            shell=shell,
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            start_new_session=True,
        )
        stdout, stderr = process.communicate(timeout=timeout)
        record.update({
            "exit_code": process.returncode,
            "ok": process.returncode == 0,
            "duration_seconds": round(time.monotonic() - started, 3),
            "stdout": stdout or "",
            "stderr": stderr or "",
            "stdout_sha256": sha256_text(stdout or ""),
            "stderr_sha256": sha256_text(stderr or ""),
        })
    except subprocess.TimeoutExpired as error:
        stdout = output_text(error.stdout)
        stderr = output_text(error.stderr)
        if process is not None:
            try:
                os.killpg(process.pid, signal.SIGTERM)
            except (ProcessLookupError, PermissionError):
                pass
            try:
                process_stdout, process_stderr = process.communicate(timeout=5)
                stdout = process_stdout or stdout
                stderr = process_stderr or stderr
            except subprocess.TimeoutExpired:
                try:
                    os.killpg(process.pid, signal.SIGKILL)
                except (ProcessLookupError, PermissionError):
                    pass
                process_stdout, process_stderr = process.communicate()
                stdout = process_stdout or stdout
                stderr = process_stderr or stderr
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


def compose_command(
    compose_files: list[str],
    *args: str,
    project_name: str = "",
    project_directory: str = "",
) -> list[str]:
    compose_bin = os.environ.get("ACCEPTANCE_COMPOSE_BIN", "docker compose")
    command = [*shlex.split(compose_bin)]
    if project_name:
        command.extend(["--project-name", project_name])
    if project_directory:
        command.extend(["--project-directory", project_directory])
    for compose_file in compose_files:
        command.extend(["-f", compose_file])
    command.extend(args)
    return command


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


def verify_backend_image(
    config_command: list[str],
    ps_command: list[str],
    service: str,
    expected_image: str,
    timeout: float,
) -> dict[str, Any]:
    """Verify the requested image against Compose and the running container."""
    result: dict[str, Any] = {
        "name": "backend_image_check",
        "expected_image": expected_image,
        "status": "FAIL",
        "ok": False,
    }
    if not expected_image:
        result["reason"] = "expected backend image is empty"
        return result

    config = record_command(config_command, timeout=timeout)
    config_stdout = config.get("stdout", "")
    # Compose config contains the complete environment, including provider
    # secrets. Keep only its hash and parsed image in the evidence report.
    config["stdout"] = ""
    config["stdout_redacted"] = True
    config["stdout_length"] = len(config_stdout)
    result["compose_config"] = config
    if not config.get("ok"):
        result["reason"] = "Compose config could not be rendered"
        return result
    try:
        config_value = json.loads(config_stdout)
    except json.JSONDecodeError as error:
        result["reason"] = f"Compose config was not valid JSON: {error}"
        return result
    config_services = config_value.get("services") if isinstance(config_value, dict) else None
    service_config = config_services.get(service) if isinstance(config_services, dict) else None
    compose_image = service_config.get("image", "") if isinstance(service_config, dict) else ""
    result["compose_image"] = compose_image
    if not compose_image:
        result["reason"] = f"Compose config has no image for service {service!r}"

    ps = record_command(ps_command, timeout=timeout)
    ps_services = parse_ps_output(ps.get("stdout", ""))
    ps["services"] = ps_services
    result["ps"] = ps
    container = next(
        (
            item
            for item in ps_services
            if str(item.get("Service", item.get("service", ""))) == service
        ),
        ps_services[0] if len(ps_services) == 1 else None,
    )
    container_name = ""
    if isinstance(container, dict):
        container_name = str(container.get("Name") or container.get("name") or "").strip()
    result["container_name"] = container_name
    if not ps.get("ok") or not container_name:
        result["reason"] = result.get("reason") or "running Compose service container could not be identified"
        return result

    inspect = record_command(
        ["docker", "inspect", "--format", "{{.Config.Image}}", container_name],
        timeout=timeout,
    )
    result["container_inspect"] = inspect
    actual_images = [line.strip() for line in inspect.get("stdout", "").splitlines() if line.strip()]
    container_image = actual_images[0] if len(actual_images) == 1 else ""
    result["container_image"] = container_image
    result["images"] = {
        "expected": expected_image,
        "compose": compose_image,
        "container_config": container_image,
    }
    if not inspect.get("ok") or not container_image:
        result["reason"] = result.get("reason") or "running container Config.Image could not be read"
        return result

    mismatches = [
        name
        for name, image in (
            ("expected", expected_image),
            ("compose", compose_image),
            ("container_config", container_image),
        )
        if image != expected_image
    ]
    if mismatches:
        result["reason"] = "backend image references do not match"
        result["mismatches"] = mismatches
        return result

    result["status"] = "PASS"
    result["ok"] = True
    result["reason"] = "expected image matches Compose and running container Config.Image"
    return result


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
    if read_only_sql_issues is not None:
        read_only_issues = read_only_sql_issues(sql)
        record["read_only_guard"] = {
            "status": "PASS" if not read_only_issues else "FAIL",
            "issues": read_only_issues,
        }
        if read_only_issues:
            record["reason"] = "SQL file failed the read-only evidence guard"
            return record
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
    compose_values = list(args.compose_files or [])
    if not compose_values:
        env_value = os.environ.get("ACCEPTANCE_COMPOSE_FILES", "").strip()
        if env_value:
            compose_values = shlex.split(env_value)
        elif os.environ.get("ACCEPTANCE_COMPOSE_FILE", "").strip():
            compose_values = [os.environ["ACCEPTANCE_COMPOSE_FILE"].strip()]
    output = args.output.expanduser().resolve()
    generated_at = dt.datetime.now(dt.timezone.utc).isoformat()
    report: dict[str, Any] = {
        "schema_version": 1,
        "generated_at": generated_at,
        "mode": "execute" if args.execute else "plan",
        "status": "PLAN",
        "compose_file": str(Path(compose_values[0]).expanduser().resolve()) if compose_values else "",
        "compose_files": [str(Path(value).expanduser().resolve()) for value in compose_values],
        "compose_project": args.project_name,
        "compose_project_directory": (
            str(Path(args.project_directory).expanduser().resolve()) if args.project_directory else ""
        ),
        "service": args.service,
        "expected_image": args.expected_image,
        "before_command": args.before_command or "",
        "after_command": args.after_command or "",
        "cleanup_command": args.cleanup_command or "",
        "allow_barrier_only": bool(args.allow_barrier_only),
        "business_assertions_supplied": bool(args.before_command and args.after_command),
        "steps": [],
    }

    if not compose_values or not args.service:
        report["status"] = "BLOCKED"
        report["reason"] = "at least one --compose-file and --service (or ACCEPTANCE_* variables) are required"
        write_report(output, report)
        print(report["reason"], file=sys.stderr)
        return 2
    compose_paths = [Path(value).expanduser().resolve() for value in compose_values]
    missing_compose = [str(path) for path in compose_paths if not path.is_file()]
    if missing_compose:
        report["status"] = "BLOCKED"
        report["reason"] = f"Compose file(s) not found: {', '.join(missing_compose)}"
        write_report(output, report)
        print(report["reason"], file=sys.stderr)
        return 2

    if args.execute and not (args.before_command and args.after_command) and not args.allow_barrier_only:
        report["status"] = "BLOCKED"
        report["reason"] = (
            "--execute requires both --before-command and --after-command; "
            "use --allow-barrier-only only for an explicitly acknowledged barrier-only check"
        )
        write_report(output, report)
        print(report["reason"], file=sys.stderr)
        return 2

    if args.execute and not args.expected_image:
        report["status"] = "BLOCKED"
        report["reason"] = "--execute requires --expected-image or TEAMGRAM_BACKEND_IMAGE"
        write_report(output, report)
        print(report["reason"], file=sys.stderr)
        return 2

    compose_files = [str(path) for path in compose_paths]
    project_directory = (
        str(Path(args.project_directory).expanduser().resolve()) if args.project_directory else ""
    )

    def command(*command_args: str) -> list[str]:
        return compose_command(
            compose_files,
            *command_args,
            project_name=args.project_name,
            project_directory=project_directory,
        )

    config_command = command("config", "--services")
    config_json_command = command("config", "--format", "json")
    ps_command = command("ps", "--all", "--format", "json", args.service)
    report["planned_commands"] = {
        "config": config_command,
        "config_json": config_json_command,
        "ps": ps_command,
        "restart": command("restart", args.service),
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

    if args.expected_image:
        image_check = verify_backend_image(
            config_json_command,
            ps_command,
            args.service,
            args.expected_image,
            args.timeout,
        )
        report["steps"].append(image_check)
        if not image_check.get("ok"):
            report["status"] = "FAIL"
            report["reason"] = image_check.get("reason", "backend image check failed")
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
        sql_before = capture_sql(args.sql_file.expanduser().resolve(), args.dsn_env, "before", args.timeout)
        report["steps"].append(sql_before)
        if sql_before.get("status") != "PASS":
            report["status"] = "FAIL"
            report["reason"] = "pre-restart SQL evidence capture failed; restart was not attempted"
            if args.cleanup_command:
                report["steps"].append({
                    "name": "cleanup",
                    **record_command(args.cleanup_command, shell=True, timeout=args.timeout),
                })
            write_report(output, report)
            return 1

    ps_before = record_command(
        command("ps", "--all", "--format", "json", args.service),
        timeout=args.timeout,
    )
    ps_before_services = parse_ps_output(ps_before.get("stdout", ""))
    ps_before["services"] = ps_before_services
    report["steps"].append({"name": "ps_before", **ps_before})
    before_ready = False
    before_reason = "no service record"
    for service in ps_before_services:
        before_ready, before_reason = service_ready(service)
        if before_ready:
            break
    if not ps_before.get("ok") or not before_ready:
        report["status"] = "FAIL"
        report["reason"] = f"service is not ready before restart: {before_reason}"
        if args.cleanup_command:
            report["steps"].append({
                "name": "cleanup",
                **record_command(args.cleanup_command, shell=True, timeout=args.timeout),
            })
        write_report(output, report)
        return 1

    restart = record_command(
        command("restart", args.service),
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
                command("ps", "--all", "--format", "json", args.service),
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
        sql_after = capture_sql(args.sql_file.expanduser().resolve(), args.dsn_env, "after", args.timeout)
        report["steps"].append(sql_after)
        if sql_after.get("status") != "PASS":
            report["status"] = "FAIL"

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
