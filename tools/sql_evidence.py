#!/usr/bin/env python3
"""Capture a read-only PostgreSQL evidence query without leaking credentials.

The DSN is read from an environment variable and is never copied into the
JSON report.  The report records the SQL hash, command result, and output so a
reviewer can compare an artifact with the exact generated query later.
"""

from __future__ import annotations

import argparse
import datetime as dt
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import time
from typing import Any


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--sql-file", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument(
        "--dsn-env",
        default="ACCEPTANCE_DATABASE_URL",
        help="Environment variable containing the PostgreSQL DSN (never written to the report).",
    )
    parser.add_argument("--psql", default="psql", help="psql executable (default: psql).")
    parser.add_argument("--timeout", type=float, default=120.0)
    return parser.parse_args()


def file_sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def text_sha256(value: str) -> str:
    return hashlib.sha256(value.encode("utf-8", errors="replace")).hexdigest()


def output_text(value: Any) -> str:
    if value is None:
        return ""
    if isinstance(value, (bytes, bytearray)):
        return value.decode(errors="replace")
    return str(value)


def write_report(path: Path, report: dict[str, Any]) -> None:
    path.expanduser().resolve().parent.mkdir(parents=True, exist_ok=True)
    path.expanduser().resolve().write_text(
        json.dumps(report, ensure_ascii=False, indent=2, sort_keys=True) + "\n", encoding="utf-8"
    )


def main() -> int:
    args = parse_args()
    sql_file = args.sql_file.expanduser().resolve()
    output = args.output.expanduser().resolve()
    generated_at = dt.datetime.now(dt.timezone.utc).isoformat()
    report: dict[str, Any] = {
        "schema_version": 1,
        "generated_at": generated_at,
        "sql_file": str(sql_file),
        "dsn_env": args.dsn_env,
        "status": "BLOCKED",
    }

    if not sql_file.is_file():
        report["reason"] = f"SQL file not found: {sql_file}"
        write_report(output, report)
        print(report["reason"], file=sys.stderr)
        return 2
    sql = sql_file.read_text(encoding="utf-8")
    report["sql_sha256"] = file_sha256(sql_file)
    report["sql_bytes"] = len(sql.encode("utf-8"))

    dsn = os.environ.get(args.dsn_env, "").strip()
    has_pg_environment = any(
        os.environ.get(name)
        for name in ("PGHOST", "PGPORT", "PGDATABASE", "PGUSER", "PGSERVICE")
    )
    report["dsn_configured"] = bool(dsn or has_pg_environment)
    if not dsn and not has_pg_environment:
        report["reason"] = (
            f"{args.dsn_env} is unset and no PGHOST/PGDATABASE/PGUSER/PGSERVICE environment is configured"
        )
        write_report(output, report)
        print(report["reason"], file=sys.stderr)
        return 2
    psql = shutil.which(args.psql) if not Path(args.psql).is_file() else args.psql
    if not psql:
        report["reason"] = f"psql executable not found: {args.psql}"
        write_report(output, report)
        print(report["reason"], file=sys.stderr)
        return 2

    command = [psql]
    if dsn:
        command.append(dsn)
    command.extend([
        "--no-psqlrc",
        "--set=ON_ERROR_STOP=1",
        "--quiet",
        "--file",
        str(sql_file),
    ])
    # Do not store the DSN in the report or print it.  The displayed command
    # replaces the positional DSN with an explicit redaction marker.
    display_command = [psql]
    if dsn:
        display_command.extend(["<redacted-dsn>", *command[2:]])
    else:
        display_command.extend(command[1:])
    report["command"] = display_command
    report["output"] = str(output)
    started = time.monotonic()
    try:
        completed = subprocess.run(
            command,
            text=True,
            capture_output=True,
            timeout=args.timeout,
            check=False,
        )
        stdout = completed.stdout or ""
        stderr = completed.stderr or ""
        report.update({
            "status": "PASS" if completed.returncode == 0 else "FAIL",
            "exit_code": completed.returncode,
            "duration_seconds": round(time.monotonic() - started, 3),
            "stdout": stdout,
            "stderr": stderr,
            "stdout_sha256": text_sha256(stdout),
            "stderr_sha256": text_sha256(stderr),
        })
    except subprocess.TimeoutExpired as error:
        stdout = output_text(error.stdout)
        stderr = output_text(error.stderr)
        report.update({
            "status": "TIMEOUT",
            "exit_code": None,
            "duration_seconds": round(time.monotonic() - started, 3),
            "stdout": stdout,
            "stderr": stderr,
            "stdout_sha256": text_sha256(stdout),
            "stderr_sha256": text_sha256(stderr),
        })
    except OSError as error:
        report.update({
            "status": "FAIL",
            "exit_code": None,
            "duration_seconds": round(time.monotonic() - started, 3),
            "stdout": "",
            "stderr": str(error),
            "stderr_sha256": text_sha256(str(error)),
        })

    write_report(output, report)
    print(json.dumps({key: report[key] for key in ("status", "output" ) if key in report}, sort_keys=True))
    return 0 if report["status"] == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
