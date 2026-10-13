#!/usr/bin/env python3
"""Run the focused Layer 229 acceptance matrix without inventing evidence.

The matrix deliberately separates executable probes from gates that need a
backend restart or a real external provider. ``--check`` is read-only. Only
``--run`` executes a probe, and its output is copied verbatim to the report.
"""

from __future__ import annotations

import argparse
from collections import Counter
import datetime as dt
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import time
from typing import Any
import xml.etree.ElementTree as ET


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
    parser.add_argument(
        "--merge-report",
        type=Path,
        help="Merge external evidence into an existing JSON report without rerunning probes.",
    )
    parser.add_argument("--timeout", type=float, default=900.0, help="Per-probe timeout in seconds (default: 900).")
    parser.add_argument("--output", type=Path, help="JSON report path for --run (default: a file under /tmp).")
    parser.add_argument(
        "--playwright-junit",
        type=Path,
        action="append",
        help="JUnit XML report produced by the real Web UI Playwright suite (repeatable).",
    )
    parser.add_argument(
        "--probe-evidence",
        action="append",
        metavar="PATH|ROW_ID=PATH",
        help=(
            "JSON acceptance/probe evidence to merge with --merge-report. "
            "Acceptance reports are matched by results[].id; a raw Premium fixture "
            "must use ROW_ID=PATH explicitly. (repeatable)"
        ),
    )
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
        external_evidence = row.get("external_evidence")
        if external_evidence is not None:
            if not isinstance(external_evidence, dict):
                raise ValueError(f"{row_id}: external_evidence must be an object")
            if external_evidence.get("type") != "playwright_junit":
                raise ValueError(f"{row_id}: unsupported external_evidence type")
            tests = external_evidence.get("tests")
            if not isinstance(tests, list) or not tests or not all(isinstance(test, str) and test for test in tests):
                raise ValueError(f"{row_id}: external_evidence.tests must be a non-empty string list")
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
        "external_evidence": row.get("external_evidence"),
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


def junit_test_id(testcase: ET.Element) -> str:
    classname = testcase.get("classname", "").strip()
    name = testcase.get("name", "").strip()
    return f"{classname}::{name}" if classname else name


def junit_duration(testcase: ET.Element) -> float | None:
    value = testcase.get("time")
    if value is None:
        return None
    try:
        return float(value)
    except ValueError:
        return None


def load_playwright_junit(path: Path) -> dict[str, Any]:
    path = path.expanduser()
    if not path.is_file():
        raise OSError(f"Playwright JUnit report not found: {path}")
    root = ET.parse(path).getroot()
    tests: list[dict[str, Any]] = []
    for testcase in root.findall(".//testcase"):
        if testcase.find("./failure") is not None or testcase.find("./error") is not None:
            result = "FAIL"
        elif testcase.find("./skipped") is not None:
            result = "SKIP"
        else:
            result = "PASS"
        tests.append({
            "id": junit_test_id(testcase),
            "classname": testcase.get("classname", ""),
            "name": testcase.get("name", ""),
            "result": result,
            "duration_seconds": junit_duration(testcase),
        })
    if not tests:
        raise ValueError(f"Playwright JUnit report contains no testcases: {path}")
    return {
        "path": str(path),
        "tests": tests,
        "counts": dict(sorted(Counter(test["result"] for test in tests).items())),
    }


def load_playwright_evidence(paths: list[Path] | None) -> dict[str, Any] | None:
    if not paths:
        return None
    reports = [load_playwright_junit(path) for path in paths]
    tests_by_id: dict[str, list[dict[str, Any]]] = {}
    for report in reports:
        for test in report["tests"]:
            tests_by_id.setdefault(test["id"], []).append(test)
    return {
        "reports": reports,
        "tests_by_id": tests_by_id,
    }


PROBE_RESULTS = {"PASS", "FAIL", "TIMEOUT", "MANUAL", "BLOCKED"}
PREMIUM_FIXTURE_ROW = "premium.entitlement-restart-fixture"


def _probe_result(value: Any, source: Path, row_id: str) -> str:
    result = str(value.get("result") or value.get("status") or "").upper()
    if result not in PROBE_RESULTS:
        raise ValueError(f"{source}: {row_id}: evidence result must be one of {sorted(PROBE_RESULTS)}")
    return result


def _premium_fixture_errors(evidence: dict[str, Any]) -> list[str]:
    errors: list[str] = []
    if evidence.get("restartRequested") is not True:
        errors.append("restartRequested is not true")
    purchase = evidence.get("purchase")
    if not isinstance(purchase, dict):
        errors.append("purchase object is missing")
    else:
        for field in ("first", "replay"):
            if purchase.get(field) not in {"payments.PaymentResult", "PaymentResult"}:
                errors.append(f"purchase.{field} is not payments.PaymentResult")
    transaction_id = str(evidence.get("transactionId") or "")
    if not transaction_id:
        errors.append("transactionId is missing")
    for label in ("ledgerBeforeRestart", "ledgerAfterRestart"):
        ledger = evidence.get(label)
        if not isinstance(ledger, dict):
            errors.append(f"{label} object is missing")
            continue
        if ledger.get("requestState") != "settled":
            errors.append(f"{label}.requestState is not settled")
        if ledger.get("outboxState") != "complete":
            errors.append(f"{label}.outboxState is not complete")
        if ledger.get("providerEntitlementState") != "granted":
            errors.append(f"{label}.providerEntitlementState is not granted")
        if transaction_id and str(ledger.get("transactionId")) != transaction_id:
            errors.append(f"{label}.transactionId does not match transactionId")
    for label in ("userRowBeforeRestart", "userRowAfterRestart"):
        user_row = evidence.get(label)
        if not isinstance(user_row, dict) or user_row.get("premium") is not True:
            errors.append(f"{label}.premium is not true")
    for label in ("sessionsBeforeRestart", "sessionsAfterRestart"):
        sessions = evidence.get(label)
        observed = sessions.get("bObservedUser") if isinstance(sessions, dict) else None
        if not isinstance(sessions, dict) or sessions.get("aGetMePremium") is not True:
            errors.append(f"{label}.aGetMePremium is not true")
        if not isinstance(observed, dict) or observed.get("premium") is not True:
            errors.append(f"{label}.bObservedUser.premium is not true")
    return errors


def _probe_record_summary(record: dict[str, Any]) -> dict[str, Any]:
    """Keep acceptance report provenance without duplicating stdout/stderr."""

    fields = ("id", "result", "status", "category", "gate", "script", "exit_code", "duration_seconds")
    return {field: record[field] for field in fields if field in record}


def load_probe_evidence(
    specifications: list[str] | None,
    rows: list[dict[str, Any]],
) -> dict[str, Any] | None:
    """Load structured probe evidence for an explicit report merge.

    A normal acceptance report carries row IDs in ``results``. Raw Premium
    fixture output has no row ID, so it must be bound explicitly as
    ``premium.entitlement-restart-fixture=/path/to/probe.json`` and is
    validated before it can change a row to PASS.
    """

    if not specifications:
        return None
    known_ids = {str(row["id"]) for row in rows}
    by_id: dict[str, list[dict[str, Any]]] = {}
    reports: list[dict[str, Any]] = []
    for specification in specifications:
        explicit_id: str | None = None
        raw_path = specification
        if "=" in specification:
            candidate, raw_path = specification.split("=", 1)
            if candidate and raw_path:
                explicit_id = candidate
        path = Path(raw_path).expanduser()
        try:
            artifact = json.loads(path.read_text(encoding="utf-8"))
        except (OSError, json.JSONDecodeError) as error:
            raise ValueError(f"could not load probe evidence {path}: {error}") from error
        if not isinstance(artifact, dict):
            raise ValueError(f"probe evidence must be a JSON object: {path}")

        records: list[tuple[str, str, dict[str, Any]]] = []
        if isinstance(artifact.get("results"), list):
            for record in artifact["results"]:
                if not isinstance(record, dict) or not record.get("id"):
                    raise ValueError(f"{path}: every acceptance result needs an id")
                row_id = str(record["id"])
                result = _probe_result(record, path, row_id)
                records.append((row_id, result, {"record": _probe_record_summary(record)}))
            report_format = "acceptance_report"
        elif explicit_id == PREMIUM_FIXTURE_ROW:
            errors = _premium_fixture_errors(artifact)
            if errors:
                raise ValueError(f"{path}: invalid Premium fixture evidence: {'; '.join(errors)}")
            records.append((explicit_id, "PASS", {
                "format": "premium_fixture_probe",
                "provenance": "local_signed_payment_fixture",
                "known_limitations": [
                    "real Apple/Google receipt verification is not covered",
                    "real refund/revoke webhook and entitlement removal are not covered",
                    "production provider reconciliation is not covered",
                ],
                "artifact": artifact,
            }))
            report_format = "premium_fixture_probe"
        elif explicit_id:
            if explicit_id not in known_ids:
                raise ValueError(f"{path}: unknown matrix row id: {explicit_id}")
            result = _probe_result(artifact, path, explicit_id)
            records.append((explicit_id, result, {"record": _probe_record_summary(artifact)}))
            report_format = "single_probe_record"
        elif artifact.get("id"):
            row_id = str(artifact["id"])
            result = _probe_result(artifact, path, row_id)
            records.append((row_id, result, {"record": _probe_record_summary(artifact)}))
            report_format = "single_probe_record"
        else:
            raise ValueError(
                f"{path}: unsupported probe JSON; use an acceptance report or explicit ROW_ID=PATH"
            )

        for row_id, result, detail in records:
            if row_id not in known_ids:
                raise ValueError(f"{path}: unknown matrix row id: {row_id}")
            if row_id in by_id:
                raise ValueError(f"duplicate probe evidence for matrix row: {row_id}")
            by_id[row_id] = [{
                "source": str(path),
                "result": result,
                **detail,
            }]
        reports.append({"path": str(path), "format": report_format, "row_ids": [row_id for row_id, _, _ in records]})
    return {"reports": reports, "by_id": by_id}


def apply_external_evidence(
    result: dict[str, Any],
    row: dict[str, Any],
    evidence: dict[str, Any] | None,
) -> dict[str, Any]:
    specification = row.get("external_evidence")
    if not specification:
        return result

    required_tests = list(specification["tests"])
    details: dict[str, Any] = {
        "type": specification["type"],
        "required_tests": required_tests,
        "status": "MANUAL",
    }
    if evidence is None:
        result["external_evidence"] = details
        return result

    details["reports"] = [report["path"] for report in evidence["reports"]]
    missing_tests: list[str] = []
    matched_tests: list[dict[str, Any]] = []
    failed_tests: list[str] = []
    skipped_tests: list[str] = []
    for test_id in required_tests:
        matches = evidence["tests_by_id"].get(test_id, [])
        if not matches:
            missing_tests.append(test_id)
            continue
        matched_tests.extend(matches)
        statuses = {match["result"] for match in matches}
        if "FAIL" in statuses:
            failed_tests.append(test_id)
        elif "PASS" not in statuses:
            skipped_tests.append(test_id)

    details["matched_tests"] = matched_tests
    if missing_tests:
        details["status"] = "MISSING"
        details["missing_tests"] = missing_tests
        result["result"] = "MANUAL"
    elif failed_tests:
        details["status"] = "FAIL"
        details["failed_tests"] = failed_tests
        result["result"] = "FAIL"
    elif skipped_tests:
        details["status"] = "INCOMPLETE"
        details["skipped_tests"] = skipped_tests
        result["result"] = "MANUAL"
    else:
        details["status"] = "PASS"
        result["result"] = "PASS"
        result["status"] = "PASS"
    result["external_evidence"] = details
    return result


def apply_probe_evidence(
    result: dict[str, Any],
    evidence: dict[str, Any] | None,
) -> dict[str, Any]:
    """Attach explicit probe provenance and promote only a validated PASS."""

    if evidence is None:
        return result
    matches = evidence["by_id"].get(str(result.get("id")))
    if not matches:
        return result
    result["probe_evidence"] = matches
    statuses = {str(match.get("result", "")).upper() for match in matches}
    current = str(result.get("result", "")).upper()
    if "FAIL" in statuses:
        result["result"] = "FAIL"
    elif "TIMEOUT" in statuses and current not in {"FAIL"}:
        result["result"] = "TIMEOUT"
    elif "PASS" in statuses and current not in {"FAIL", "TIMEOUT"}:
        result["result"] = "PASS"
        result["status"] = "PASS"
    return result


def merge_report(
    source_path: Path,
    output_path: Path | None,
    rows: list[dict[str, Any]],
    evidence: dict[str, Any] | None,
    probe_evidence: dict[str, Any] | None = None,
) -> tuple[Path, dict[str, Any]]:
    source_path = source_path.expanduser()
    report = json.loads(source_path.read_text(encoding="utf-8"))
    if not isinstance(report, dict) or not isinstance(report.get("results"), list):
        raise ValueError(f"acceptance report has no results array: {source_path}")
    rows_by_id = {str(row["id"]): row for row in rows}
    result_ids = {str(item.get("id")) for item in report["results"]}
    for item in report["results"]:
        row = rows_by_id.get(str(item.get("id")))
        if row is not None:
            apply_external_evidence(item, row, evidence)
        apply_probe_evidence(item, probe_evidence)
    for row in rows:
        row_id = str(row["id"])
        if row_id in result_ids:
            continue
        item = apply_external_evidence(inspect_row(row), row, evidence)
        apply_probe_evidence(item, probe_evidence)
        report["results"].append(item)
    report["merged_at"] = dt.datetime.now(dt.timezone.utc).isoformat()
    if evidence is not None:
        report["external_evidence"] = {
            "playwright_junit": evidence["reports"],
        }
    if probe_evidence is not None:
        report["probe_evidence"] = probe_evidence["reports"]
    output = output_path.expanduser() if output_path else source_path.with_name(f"{source_path.stem}-with-playwright.json")
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(report, ensure_ascii=False, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    return output, report


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
    if args.run and args.merge_report:
        print("critical path acceptance: --run and --merge-report are mutually exclusive", file=sys.stderr)
        return 2
    try:
        rows = select_rows(load_rows(args.matrix), args)
        playwright_evidence = load_playwright_evidence(args.playwright_junit)
        probe_evidence = load_probe_evidence(args.probe_evidence, rows)
    except (OSError, ValueError, json.JSONDecodeError) as error:
        print(f"critical path acceptance: {error}", file=sys.stderr)
        return 2

    if args.merge_report:
        if playwright_evidence is None and probe_evidence is None:
            print(
                "critical path acceptance: --merge-report requires --playwright-junit or --probe-evidence",
                file=sys.stderr,
            )
            return 2
        try:
            output, report = merge_report(
                args.merge_report,
                args.output,
                rows,
                playwright_evidence,
                probe_evidence,
            )
        except (OSError, ValueError, json.JSONDecodeError) as error:
            print(f"critical path acceptance: {error}", file=sys.stderr)
            return 2
        results = report["results"]
        emit_json({
            "report": str(output),
            "results": [{"id": item["id"], "result": item["result"]} for item in results],
        })
        if any(item.get("result") in {"FAIL", "TIMEOUT"} for item in results):
            return 1
        if any(item.get("result") in {"BLOCKED", "MANUAL"} for item in results):
            return 2
        return 0

    if args.run:
        results: list[dict[str, Any]] = []
        for row in rows:
            print(f"[{row['id']}]", file=sys.stderr)
            result = run_row(row, args.timeout)
            result = apply_external_evidence(result, row, playwright_evidence)
            result = apply_probe_evidence(result, probe_evidence)
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
        if playwright_evidence is not None:
            report["external_evidence"] = {
                "playwright_junit": playwright_evidence["reports"],
            }
        if probe_evidence is not None:
            report["probe_evidence"] = probe_evidence["reports"]
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
