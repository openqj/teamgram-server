#!/usr/bin/env python3
"""Run the focused Layer 229 acceptance matrix without inventing evidence.

The matrix deliberately separates executable probes from gates that need a
backend restart or a real external provider. ``--check`` is read-only. Only
``--run`` executes a probe, and its output is copied verbatim to the report.
"""

from __future__ import annotations

import argparse
import datetime as dt
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import time
from typing import Any


ROOT = Path(__file__).resolve().parents[1]
DEFAULT_MATRIX = ROOT / "tools" / "critical_path_acceptance_matrix.json"
DEFAULT_GRAMJS = ROOT.parent / "telegram-tt-master"


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--matrix", type=Path, default=DEFAULT_MATRIX)
    parser.add_argument("--category", action="append", help="Filter by category (repeatable).")
    parser.add_argument("--id", dest="ids", action="append", help="Run/list only this row (repeatable).")
    mode = parser.add_mutually_exclusive_group()
    mode.add_argument("--list", action="store_true", help="List selected rows (default).")
    mode.add_argument("--check", action="store_true", help="Check local prerequisites without executing probes.")
    mode.add_argument("--run", action="store_true", help="Run every selected executable row.")
    parser.add_argument("--timeout", type=float, default=900.0, help="Per-probe timeout in seconds (default: 900).")
    parser.add_argument("--output", type=Path, help="JSON report path for --run (default: a file under /tmp).")
    parser.add_argument("--stop-on-fail", action="store_true", help="Stop after the first failed executable probe.")
    parser.add_argument("--json", action="store_true", help="Emit machine-readable rows for --list/--check.")
    return parser.parse_args()


def load_rows(path: Path) -> list[dict[str, Any]]:
    with path.open(encoding="utf-8") as handle:
        rows = json.load(handle)
    if not isinstance(rows, list) or not all(isinstance(row, dict) for row in rows):
        raise ValueError(f"matrix must be a JSON array of objects: {path}")
    seen: set[str] = set()
    for row in rows:
        row_id = row.get("id")
        if not isinstance(row_id, str) or not row_id:
            raise ValueError("every matrix row needs a non-empty id")
        if row_id in seen:
            raise ValueError(f"duplicate matrix row id: {row_id}")
        seen.add(row_id)
        if row.get("kind") not in {"tsx", "shell", "manual"}:
            raise ValueError(f"{row_id}: kind must be tsx, shell or manual")
        if not isinstance(row.get("required_env", []), list):
            raise ValueError(f"{row_id}: required_env must be a list")
    return rows


def select_rows(rows: list[dict[str, Any]], args: argparse.Namespace) -> list[dict[str, Any]]:
    selected = rows
    if args.category:
        categories = set(args.category)
        selected = [row for row in selected if row.get("category") in categories]
    if args.ids:
        ids = set(args.ids)
        selected = [row for row in selected if row.get("id") in ids]
        unknown = ids - {str(row["id"]) for row in selected}
        if unknown:
            raise ValueError(f"unknown matrix row(s): {', '.join(sorted(unknown))}")
    if not selected:
        raise ValueError("selection is empty")
    return selected


def effective_env(row: dict[str, Any]) -> dict[str, str]:
    env = dict(os.environ)
    if "TEAMGRAM_GRAMJS_DIR" in row.get("required_env", []) and not env.get("TEAMGRAM_GRAMJS_DIR"):
        env["TEAMGRAM_GRAMJS_DIR"] = str(DEFAULT_GRAMJS)
    for name, value in row.get("env_defaults", {}).items():
        env.setdefault(str(name), str(value))
    return env


def row_path(row: dict[str, Any]) -> Path:
    script = row.get("script")
    if not script:
        return ROOT / "__manual__"
    return ROOT / str(script)


def inspect_row(row: dict[str, Any]) -> dict[str, Any]:
    """Return a non-secret readiness record for a row."""

    env = effective_env(row)
    row_id = str(row["id"])
    missing = [name for name in row.get("required_env", []) if not env.get(str(name))]
    script = row_path(row)
    errors: list[str] = []
    if row.get("kind") != "manual" and not script.is_file():
        errors.append(f"script not found: {script}")
    if row.get("requires_docker") and shutil.which("docker") is None:
        errors.append("docker executable not found")
    if row.get("kind") == "tsx":
        gramjs = Path(env.get("TEAMGRAM_GRAMJS_DIR", ""))
        tsx = gramjs / "node_modules" / ".bin" / "tsx"
        if not gramjs.is_dir():
            errors.append(f"TEAMGRAM_GRAMJS_DIR is not a directory: {gramjs}")
        elif not tsx.is_file() or not os.access(tsx, os.X_OK):
            errors.append(f"tsx executable not found: {tsx}")
    if row.get("kind") == "shell" and script.is_file() and not os.access(script, os.X_OK):
        errors.append(f"script is not executable: {script}")
    if row.get("kind") == "manual":
        status = "MANUAL"
    elif missing or errors:
        status = "BLOCKED"
    else:
        status = "READY"
    return {
        "id": row_id,
        "category": row.get("category"),
        "gate": row.get("gate"),
        "kind": row.get("kind"),
        "script": row.get("script"),
        "status": status,
        "missing_env": missing,
        "errors": errors,
        "evidence": row.get("evidence", ""),
        "known_gap": row.get("known_gap", ""),
        "manual_steps": row.get("manual_steps", []),
        "destructive": bool(row.get("destructive", False)),
    }


def command_for(row: dict[str, Any], env: dict[str, str]) -> list[str]:
    script = row_path(row)
    if row.get("kind") == "tsx":
        tsx = Path(env["TEAMGRAM_GRAMJS_DIR"]) / "node_modules" / ".bin" / "tsx"
        return [str(tsx), str(script)]
    if row.get("kind") == "shell":
        return [str(script)]
    raise ValueError(f"manual row has no executable command: {row['id']}")


def output_text(value: Any) -> str:
    if value is None:
        return ""
    if isinstance(value, (bytes, bytearray)):
        return value.decode(errors="replace")
    return str(value)


def format_check(records: list[dict[str, Any]]) -> None:
    print("id\tcategory\tgate\tstatus\tdetails")
    for record in records:
        details = [*record["missing_env"], *record["errors"]]
        if record["status"] == "MANUAL":
            details.append("manual evidence required")
        print("\t".join([
            str(record["id"]),
            str(record["category"]),
            str(record["gate"]),
            str(record["status"]),
            "; ".join(details) or "ready",
        ]))


def run_row(row: dict[str, Any], timeout: float) -> dict[str, Any]:
    inspected = inspect_row(row)
    if inspected["status"] != "READY":
        return {**inspected, "result": inspected["status"]}
    env = effective_env(row)
    command = command_for(row, env)
    started = time.monotonic()
    try:
        completed = subprocess.run(
            command,
            cwd=ROOT,
            env=env,
            text=True,
            capture_output=True,
            timeout=timeout,
            check=False,
        )
        result = "PASS" if completed.returncode == 0 else "FAIL"
        return {
            **inspected,
            "result": result,
            "exit_code": completed.returncode,
            "duration_seconds": round(time.monotonic() - started, 3),
            "command": command,
            "stdout": completed.stdout,
            "stderr": completed.stderr,
        }
    except subprocess.TimeoutExpired as error:
        return {
            **inspected,
            "result": "TIMEOUT",
            "exit_code": None,
            "duration_seconds": round(time.monotonic() - started, 3),
            "command": command,
            "stdout": output_text(error.stdout),
            "stderr": output_text(error.stderr),
        }
    except OSError as error:
        return {
            **inspected,
            "result": "FAIL",
            "exit_code": None,
            "duration_seconds": round(time.monotonic() - started, 3),
            "command": command,
            "stdout": "",
            "stderr": str(error),
        }


def emit_json(value: Any) -> None:
    print(json.dumps(value, ensure_ascii=False, indent=2, sort_keys=True))


def main() -> int:
    args = parse_args()
    try:
        rows = select_rows(load_rows(args.matrix), args)
    except (OSError, ValueError, json.JSONDecodeError) as error:
        print(f"critical path acceptance: {error}", file=sys.stderr)
        return 2

    if args.run:
        results: list[dict[str, Any]] = []
        for row in rows:
            print(f"[{row['id']}]", file=sys.stderr)
            result = run_row(row, args.timeout)
            results.append(result)
            print(f"  {result.get('result')}", file=sys.stderr)
            if args.stop_on_fail and result.get("result") in {"FAIL", "TIMEOUT"}:
                break
        report = {
            "generated_at": dt.datetime.now(dt.timezone.utc).isoformat(),
            "matrix": str(args.matrix),
            "mode": "run",
            "results": results,
        }
        if args.output:
            output = args.output.expanduser()
        else:
            stamp = dt.datetime.now(dt.timezone.utc).strftime("%Y%m%dT%H%M%SZ")
            output = Path("/tmp") / f"teamgram-critical-path-acceptance-{stamp}.json"
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_text(json.dumps(report, ensure_ascii=False, indent=2, sort_keys=True) + "\n", encoding="utf-8")
        emit_json({"report": str(output), "results": [{"id": item["id"], "result": item["result"]} for item in results]})
        if any(item.get("result") in {"FAIL", "TIMEOUT"} for item in results):
            return 1
        if any(item.get("result") in {"BLOCKED", "MANUAL"} for item in results):
            return 2
        return 0

    records = [inspect_row(row) for row in rows]
    if args.json:
        emit_json(records)
    else:
        format_check(records)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
