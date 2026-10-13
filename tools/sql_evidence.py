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
import re
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
    parser.add_argument(
        "--docker-container",
        help="Run psql inside this PostgreSQL container and stream the SQL over stdin.",
    )
    parser.add_argument("--docker-bin", default="docker", help="Docker executable for --docker-container.")
    parser.add_argument("--docker-user", default=os.environ.get("PGUSER", "teamgram"))
    parser.add_argument("--docker-database", default=os.environ.get("PGDATABASE", "teamgram"))
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


def _mask_sql_comments_and_literals(sql: str) -> str:
    """Mask comments and quoted values while preserving statement structure."""

    chars = list(sql)
    length = len(chars)
    i = 0

    def blank(start: int, end: int) -> None:
        for index in range(start, min(end, length)):
            if chars[index] != "\n":
                chars[index] = " "

    while i < length:
        if chars[i : i + 2] == ["-", "-"]:
            end = sql.find("\n", i)
            end = length if end < 0 else end
            blank(i, end)
            i = end
            continue
        if chars[i : i + 2] == ["/", "*"]:
            end = sql.find("*/", i + 2)
            end = length if end < 0 else end + 2
            blank(i, end)
            i = end
            continue
        if chars[i] in {"'", '"'}:
            quote = chars[i]
            start = i
            i += 1
            while i < length:
                if chars[i] == quote:
                    if i + 1 < length and chars[i + 1] == quote:
                        i += 2
                        continue
                    i += 1
                    break
                if chars[i] == "\\" and quote == "'" and i + 1 < length:
                    i += 2
                    continue
                i += 1
            blank(start, i)
            continue
        if chars[i] == "$":
            match = re.match(r"\$[A-Za-z_][A-Za-z0-9_]*\$|\$\$", sql[i:])
            if match:
                tag = match.group(0)
                end_marker = sql.find(tag, i + len(tag))
                end = length if end_marker < 0 else end_marker + len(tag)
                blank(i, end)
                i = end
                continue
        i += 1
    return "".join(chars)


def read_only_sql_issues(sql: str) -> list[str]:
    """Return conservative safety violations for an evidence SQL file.

    Evidence files are expected to be self-contained transactions.  The
    validator rejects data/schema mutations and psql shell escapes before a
    connection is opened.  Quoted strings and comments are masked first so a
    tracked method named ``update`` does not trigger the guard.
    """

    issues: list[str] = []
    masked = _mask_sql_comments_and_literals(sql)
    if re.search(r"(?m)^\s*\\[^\n]*", masked):
        issues.append("psql meta-commands are not allowed")
    if not re.search(r"\b(?:BEGIN|START\s+TRANSACTION)\b", masked, re.IGNORECASE):
        issues.append("a BEGIN or START TRANSACTION statement is required")
    if not re.search(
        r"\bSET\s+TRANSACTION\b[^;]*\bREAD\s+ONLY\b"
        r"|\bSET\s+(?:(?:LOCAL|SESSION)\s+)?DEFAULT_TRANSACTION_READ_ONLY\s*=\s*(?:ON|TRUE|1)\b",
        masked,
        re.IGNORECASE,
    ):
        issues.append("SET TRANSACTION READ ONLY (or default_transaction_read_only) is required")
    forbidden = (
        "INSERT", "UPDATE", "DELETE", "MERGE", "TRUNCATE", "ALTER", "DROP", "CREATE",
        "GRANT", "REVOKE", "VACUUM", "ANALYZE", "REFRESH", "REINDEX", "CLUSTER", "COPY",
        "CALL", "DO", "LOAD",
    )
    for keyword in forbidden:
        if re.search(rf"\b{keyword}\b", masked, re.IGNORECASE):
            issues.append(f"{keyword} statements are not allowed")
    if re.search(r"\bSELECT\b[^;]*\bINTO\b", masked, re.IGNORECASE | re.DOTALL):
        issues.append("SELECT INTO is not allowed")
    if re.search(r"\b(?:SET\s+(?:ROLE|SESSION\s+AUTHORIZATION))\b", masked, re.IGNORECASE):
        issues.append("role/session authorization changes are not allowed")
    return issues


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
        "output": str(output),
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
    read_only_issues = read_only_sql_issues(sql)
    report["read_only_guard"] = {
        "status": "PASS" if not read_only_issues else "FAIL",
        "issues": read_only_issues,
    }
    if read_only_issues:
        report["reason"] = "SQL file failed the read-only evidence guard"
        write_report(output, report)
        print(report["reason"], file=sys.stderr)
        return 2

    dsn = os.environ.get(args.dsn_env, "").strip()
    has_pg_environment = any(
        os.environ.get(name)
        for name in ("PGHOST", "PGPORT", "PGDATABASE", "PGUSER", "PGSERVICE")
    )
    if args.docker_container:
        docker = shutil.which(args.docker_bin) if not Path(args.docker_bin).is_file() else args.docker_bin
        if not docker:
            report["reason"] = f"docker executable not found: {args.docker_bin}"
            write_report(output, report)
            print(report["reason"], file=sys.stderr)
            return 2
        report["dsn_configured"] = True
        report["execution_target"] = {
            "kind": "docker_container",
            "container": args.docker_container,
            "database": args.docker_database,
            "user": args.docker_user,
        }
        command = [
            docker,
            "exec",
            "-i",
            args.docker_container,
            args.psql,
            "--username",
            args.docker_user,
            "--dbname",
            args.docker_database,
            "--no-psqlrc",
            "--set=ON_ERROR_STOP=1",
            "--quiet",
        ]
        display_command = command
        input_sql: str | None = sql
    else:
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
        input_sql = None
    report["command"] = display_command
    report["output"] = str(output)
    started = time.monotonic()
    try:
        completed = subprocess.run(
            command,
            input=input_sql,
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
