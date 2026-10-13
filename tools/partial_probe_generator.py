#!/usr/bin/env python3
"""Generate an auditable execution plan for the remaining partial API rows.

The Layer 229 ledger contains more methods than the focused acceptance matrix.
This tool joins the two sources without claiming that a generated command is a
passing test.  It emits three reviewable artifacts:

* ``manifest.json`` maps each partial method to the evidence it still needs;
* ``session-probes.json`` and ``run-session-probes.sh`` map available matrix
  probes to those methods;
* ``sql-evidence.sql`` is a read-only PostgreSQL evidence query for use by
  :mod:`sql_evidence` or a reviewer supplied ``psql`` command.

Generation is side-effect free apart from the requested output directory.
Executing the generated session or SQL runner remains explicit.
"""

from __future__ import annotations

import argparse
import csv
import datetime as dt
import hashlib
import json
import re
from pathlib import Path
import sys
from typing import Any


ROOT = Path(__file__).resolve().parents[1]
DEFAULT_LEDGER = ROOT / "LAYER229_METHOD_LEDGER.csv"
DEFAULT_MATRIX = ROOT / "tools" / "critical_path_acceptance_matrix.json"

PARTIAL_PREFIXES = ("PARTIAL", "ISOLATED", "BLOCKED", "UNKNOWN")
RESTART_MARKERS = (
    "restart",
    "replay",
    "recovery",
    "process restart",
    "backend restart",
)
SQL_MARKERS = (
    "postgres",
    "database",
    "row",
    "durable",
    "persistence",
    "persist",
    "sql",
    "ledger",
    "reconciliation",
)
GAP_MARKERS = (
    "no ",
    "not ",
    "unverified",
    "remain",
    "pending",
    "open",
    "without",
    "unavailable",
    "still ",
    "not tested",
    "not exercised",
    "unaccepted",
)

# A matrix row is a category-level candidate unless it declares the exact
# Layer 229 method(s) it exercises.  Keeping the accepted key names here lets
# the generator work with the existing matrix while making future mappings
# explicit and reviewable.
MATRIX_METHOD_KEYS = ("method", "methods", "tl_method", "tl_methods", "covers")


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--ledger", type=Path, default=DEFAULT_LEDGER)
    parser.add_argument("--matrix", type=Path, default=DEFAULT_MATRIX)
    parser.add_argument(
        "--output-dir",
        type=Path,
        default=Path("/tmp/teamgram-partial-probes"),
        help="Directory for generated artifacts (default: /tmp/teamgram-partial-probes).",
    )
    parser.add_argument("--namespace", action="append", help="Only generate rows in this namespace.")
    parser.add_argument(
        "--check",
        action="store_true",
        help="Validate the ledger and matrix without writing generated artifacts.",
    )
    parser.add_argument("--json", action="store_true", help="Print a machine-readable summary.")
    return parser.parse_args()


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def load_ledger(path: Path, namespaces: set[str] | None) -> list[dict[str, str]]:
    with path.open(newline="", encoding="utf-8") as handle:
        reader = csv.DictReader(handle)
        required = {
            "method",
            "constructor_id",
            "namespace",
            "audit_status",
            "evidence_or_gap",
            "production_acceptance_status",
            "production_acceptance_evidence",
        }
        missing = required - set(reader.fieldnames or [])
        if missing:
            raise ValueError(f"ledger is missing columns: {', '.join(sorted(missing))}")
        rows = []
        for row in reader:
            status = (row.get("production_acceptance_status") or "").strip()
            namespace = (row.get("namespace") or "").strip()
            if namespaces and namespace not in namespaces:
                continue
            if not status.startswith(PARTIAL_PREFIXES):
                continue
            rows.append({key: (value or "").strip() for key, value in row.items()})
    rows.sort(key=lambda row: row["method"])
    return rows


def load_matrix(path: Path) -> list[dict[str, Any]]:
    source = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(source, list) or not all(isinstance(row, dict) for row in source):
        raise ValueError(f"matrix must be a JSON array of objects: {path}")
    return source


def declared_matrix_methods(item: dict[str, Any]) -> set[str]:
    """Return explicit method names declared by a matrix row.

    Existing rows intentionally omit these fields and therefore remain
    category candidates.  A row must opt into exact coverage by naming the
    TL method (or its rpc route) in one of the supported fields.
    """

    methods: set[str] = set()
    for key in MATRIX_METHOD_KEYS:
        value = item.get(key)
        if isinstance(value, str) and value.strip():
            methods.add(value.strip())
        elif isinstance(value, list):
            methods.update(str(entry).strip() for entry in value if str(entry).strip())
    return methods


def matrix_method_matches(row: dict[str, str], item: dict[str, Any]) -> bool:
    declared = declared_matrix_methods(item)
    return bool(declared & {row.get("method", ""), row.get("rpc_route", "")})


def source_validation_errors(
    ledger: Path,
    matrix: Path,
    rows: list[dict[str, str]],
    matrix_rows: list[dict[str, Any]],
) -> list[str]:
    """Validate source shape and explicit mappings before generating evidence."""

    errors: list[str] = []
    all_ledger_rows = load_ledger(ledger, None)
    methods = [row["method"] for row in all_ledger_rows]
    if len(methods) != len(set(methods)):
        errors.append("ledger contains duplicate method names")
    constructors = [row["constructor_id"] for row in all_ledger_rows]
    if len(constructors) != len(set(constructors)):
        errors.append("ledger contains duplicate constructor IDs")

    matrix_ids: list[str] = []
    known_methods = {row["method"] for row in all_ledger_rows}
    known_routes = {row["rpc_route"] for row in all_ledger_rows if row.get("rpc_route")}
    for item in matrix_rows:
        row_id = item.get("id")
        if not isinstance(row_id, str) or not row_id.strip():
            errors.append("matrix row has no non-empty id")
            continue
        matrix_ids.append(row_id)
        if not isinstance(item.get("category"), str) or not item.get("category", "").strip():
            errors.append(f"{row_id}: unsupported or missing category")
        if item.get("kind") not in {"tsx", "shell", "manual"}:
            errors.append(f"{row_id}: kind must be tsx, shell or manual")
        required_env = item.get("required_env", [])
        if not isinstance(required_env, list) or not all(isinstance(name, str) for name in required_env):
            errors.append(f"{row_id}: required_env must be a list of strings")
        declared = declared_matrix_methods(item)
        unknown = sorted(
            method for method in declared
            if method not in known_methods and method not in known_routes
        )
        if unknown:
            errors.append(f"{row_id}: explicit method mapping is unknown: {', '.join(unknown)}")
        script = item.get("script")
        if item.get("kind") != "manual":
            if not isinstance(script, str) or not script:
                errors.append(f"{row_id}: executable row has no script")
            elif not (ROOT / script).is_file():
                errors.append(f"{row_id}: script not found: {ROOT / script}")
    if len(matrix_ids) != len(set(matrix_ids)):
        errors.append("matrix contains duplicate ids")
    if not rows:
        errors.append("ledger selection is empty")
    return errors


def category_for_namespace(namespace: str) -> set[str]:
    aliases = {
        "bot": {"bot_miniapp"},
        "bots": {"bot_miniapp"},
        "business": {"business"},
        "channel": {"channel"},
        "channels": {"channel"},
        "notification": {"settings"},
        "privacy": {"settings"},
        "account": {"settings"},
        "auth": {"settings"},
        "premium": {"premium"},
    }
    return aliases.get(namespace, {namespace})


def required_evidence(row: dict[str, str]) -> list[str]:
    text = row.get("production_acceptance_evidence", "").lower()
    implementation_text = row.get("evidence_or_gap", "").lower()

    def marker_is_an_open_gap(markers: tuple[str, ...]) -> bool:
        for source in (text, implementation_text):
            for marker in markers:
                start = 0
                while True:
                    position = source.find(marker, start)
                    if position < 0:
                        break
                    context = source[max(0, position - 90) : position + len(marker) + 90]
                    if any(gap in context for gap in GAP_MARKERS):
                        return True
                    start = position + len(marker)
        return False

    evidence = ["session"]
    if marker_is_an_open_gap(RESTART_MARKERS):
        evidence.append("backend_restart")
    if row.get("production_acceptance_status", "").endswith("DB_ONLY") or marker_is_an_open_gap(SQL_MARKERS):
        evidence.append("sql")
    return evidence


def matrix_candidates(row: dict[str, str], matrix: list[dict[str, Any]]) -> list[dict[str, Any]]:
    categories = category_for_namespace(row["namespace"])
    candidates = [item for item in matrix if item.get("category") in categories]
    return sorted(candidates, key=lambda item: str(item.get("id", "")))


def matrix_coverage(row: dict[str, str], matrix: list[dict[str, Any]]) -> dict[str, list[dict[str, Any]]]:
    candidates = matrix_candidates(row, matrix)
    exact = [item for item in candidates if matrix_method_matches(row, item)]
    return {"candidates": candidates, "exact": sorted(exact, key=lambda item: str(item.get("id", "")))}


def shell_quote(value: str) -> str:
    # POSIX single-quote escaping is sufficient for generated shell literals.
    return "'" + value.replace("'", "'\\''") + "'"


def sql_literal(value: str) -> str:
    return "'" + value.replace("'", "''") + "'"


def build_sql(rows: list[dict[str, str]], generated_at: str, ledger_sha256: str) -> str:
    values = ",\n".join(
        "  (" + ", ".join(
            sql_literal(row[field])
            for field in ("method", "namespace", "constructor_id", "audit_status", "production_acceptance_status")
        ) + ")"
        for row in rows
    )
    if not values:
        values = "  (NULL::text, NULL::text, NULL::text, NULL::text, NULL::text)"
    return f"""-- Generated by tools/partial_probe_generator.py at {generated_at}
-- Read-only evidence only.  This does not mutate application tables.
-- Source ledger SHA-256: {ledger_sha256}
BEGIN;
SET TRANSACTION READ ONLY;
SET LOCAL statement_timeout = '30s';
SET LOCAL lock_timeout = '5s';

SELECT current_database() AS database_name,
       current_user AS database_user,
       current_setting('server_version') AS server_version,
       pg_is_in_recovery() AS in_recovery,
       clock_timestamp() AT TIME ZONE 'UTC' AS captured_at_utc;

-- The tracked method list is deliberately data-only.  It ties this snapshot
-- to the exact ledger revision without guessing an implementation table.
WITH tracked(method, namespace, constructor_id, audit_status, production_acceptance_status) AS (
  VALUES
{values}
)
SELECT tracked.method,
       tracked.namespace,
       tracked.constructor_id,
       tracked.audit_status,
       tracked.production_acceptance_status
FROM tracked
WHERE tracked.method IS NOT NULL
ORDER BY tracked.method;

WITH tracked(method, namespace, constructor_id, audit_status, production_acceptance_status) AS (
  VALUES
{values}
)
SELECT count(*)::integer AS tracked_partial_methods,
       {len(rows)}::integer AS expected_partial_methods,
       CASE WHEN count(*) = {len(rows)} THEN 'PASS' ELSE 'FAIL' END AS coverage_status
FROM tracked
WHERE tracked.method IS NOT NULL;

-- Approximate row estimates are metadata evidence, not authoritative business
-- counts.  A method-specific probe should attach exact keyed counts separately.
SELECT schemaname, relname AS table_name, n_live_tup, n_dead_tup,
       last_analyze, last_autoanalyze
FROM pg_catalog.pg_stat_user_tables
ORDER BY schemaname, relname;

COMMIT;
"""


def build_session_entries(rows: list[dict[str, str]], matrix: list[dict[str, Any]]) -> list[dict[str, Any]]:
    entries: list[dict[str, Any]] = []
    for row in rows:
        coverage = matrix_coverage(row, matrix)
        candidates = [item for item in coverage["candidates"] if item.get("kind") in {"tsx", "shell"}]
        exact = [item for item in coverage["exact"] if item.get("kind") in {"tsx", "shell"}]
        candidate_ids = [str(item["id"]) for item in candidates if item.get("id")]
        exact_ids = [str(item["id"]) for item in exact if item.get("id")]
        required_env = sorted({
            str(name)
            for item in exact
            for name in item.get("required_env", [])
        })
        status = (
            "METHOD_PROBE_AVAILABLE"
            if exact_ids
            else "CATEGORY_CANDIDATE_ONLY"
            if candidate_ids
            else "MANUAL_SESSION_PROBE_REQUIRED"
        )
        entries.append({
            "method": row["method"],
            "namespace": row["namespace"],
            "constructor_id": row["constructor_id"],
            "audit_status": row["audit_status"],
            "production_acceptance_status": row["production_acceptance_status"],
            "required_evidence": required_evidence(row),
            "candidate_matrix_ids": candidate_ids,
            "exact_matrix_ids": exact_ids,
            "required_env": required_env,
            "status": status,
            "coverage_scope": "method" if exact_ids else "category_only" if candidate_ids else "none",
            "command": (
                ["python3", "tools/critical_path_acceptance.py", "--run", "--id", exact_ids[0]]
                if exact_ids else None
            ),
        })
    return entries


def build_restart_entries(rows: list[dict[str, str]], matrix: list[dict[str, Any]]) -> list[dict[str, Any]]:
    entries: list[dict[str, Any]] = []
    for row in rows:
        coverage = matrix_coverage(row, matrix)
        all_restart_candidates = [
            item
            for item in coverage["candidates"]
            if item.get("gate") == "backend_restart"
        ]
        executable_candidates = [item for item in all_restart_candidates if item.get("kind") in {"tsx", "shell"}]
        candidates = [
            str(item["id"])
            for item in executable_candidates
            if item.get("id")
        ]
        plan_ids = [str(item["id"]) for item in all_restart_candidates if item.get("id")]
        exact_ids = [
            str(item["id"])
            for item in coverage["exact"]
            if item.get("gate") == "backend_restart" and item.get("kind") in {"tsx", "shell"}
        ]
        status = (
            "METHOD_RESTART_GATE_AVAILABLE"
            if exact_ids
            else "CATEGORY_RESTART_CANDIDATE_ONLY"
            if candidates
            else "CATEGORY_RESTART_PLAN_ONLY"
            if plan_ids
            else "MANUAL_RESTART_PROBE_REQUIRED"
        )
        entries.append({
            "method": row["method"],
            "namespace": row["namespace"],
            "restart_gap_detected": "backend_restart" in required_evidence(row),
            "required_evidence": required_evidence(row),
            "candidate_matrix_ids": candidates,
            "plan_matrix_ids": plan_ids,
            "exact_matrix_ids": exact_ids,
            "status": status,
            "coverage_scope": (
                "method" if exact_ids else "category_only" if candidates else "category_plan" if plan_ids else "none"
            ),
            "command": (
                'python3 tools/restart_probe.py --compose-file "$ACCEPTANCE_COMPOSE_FILE" '
                '--service "$ACCEPTANCE_SERVICE" --before-command "$RESTART_BEFORE_COMMAND" '
                '--after-command "$RESTART_AFTER_COMMAND" --execute --output restart-evidence.json'
                if exact_ids else None
            ),
        })
    return entries


def write_session_runner(path: Path, entries: list[dict[str, Any]], root: Path) -> None:
    exact_ids = sorted({matrix_id for entry in entries for matrix_id in entry["exact_matrix_ids"]})
    category_ids = sorted({
        matrix_id
        for entry in entries
        for matrix_id in entry["candidate_matrix_ids"]
        if matrix_id not in entry["exact_matrix_ids"]
    })
    lines = [
        "#!/usr/bin/env bash",
        "set -euo pipefail",
        "",
        "if [[ ${RUN_PARTIAL_PROBES:-0} != 1 ]]; then",
        "  echo 'Set RUN_PARTIAL_PROBES=1 to execute generated session probes.' >&2",
        "  exit 2",
        "fi",
        "",
        f"ROOT={shell_quote(str(root))}",
        'OUT_DIR="${1:-$(dirname "$0")/runs}"',
        'mkdir -p "$OUT_DIR"',
        "",
    ]
    if not exact_ids:
        lines.extend([
            "if [[ ${RUN_PARTIAL_CATEGORY_PROBES:-0} != 1 ]]; then",
            "  echo 'No method-exact session probes are mapped; set RUN_PARTIAL_CATEGORY_PROBES=1 to run category candidates.' >&2",
            "  exit 2",
            "fi",
            "",
        ])
    if not exact_ids and not category_ids:
        lines.extend([
            "echo 'No executable matrix session probes map to the selected ledger rows.' >&2",
            "exit 2",
            "",
        ])
    run_ids = exact_ids + category_ids
    if category_ids:
        lines.extend([
            "if [[ ${RUN_PARTIAL_CATEGORY_PROBES:-0} != 1 ]]; then",
            "  CATEGORY_PROBES_DISABLED=1",
            "else",
            "  CATEGORY_PROBES_DISABLED=0",
            "fi",
            "",
        ])
    for matrix_id in run_ids:
        if matrix_id in category_ids:
            lines.extend([
                'if [[ ${CATEGORY_PROBES_DISABLED:-0} == 1 ]]; then',
                f"  echo '[{matrix_id}] category candidate skipped; no exact method mapping' >&2",
                "else",
            ])
        safe = re.sub(r"[^A-Za-z0-9_.-]+", "_", matrix_id)
        lines.extend([
            f"echo '[{matrix_id}]' >&2",
            "python3 \"$ROOT/tools/critical_path_acceptance.py\" --run \\",
            f"  --id {shell_quote(matrix_id)} \\",
            f"  --output \"$OUT_DIR/{safe}.json\"",
            "",
        ])
        if matrix_id in category_ids:
            lines.extend(["fi", ""])
    path.write_text("\n".join(lines) + "\n", encoding="utf-8")
    path.chmod(0o755)


def write_sql_runner(path: Path, root: Path) -> None:
    path.write_text(
        """#!/usr/bin/env bash
set -euo pipefail

if [[ ${RUN_SQL_EVIDENCE:-0} != 1 ]]; then
  echo 'Set RUN_SQL_EVIDENCE=1 to execute the read-only SQL evidence query.' >&2
  exit 2
fi

ROOT=%s
OUT_DIR="${1:-$(dirname "$0")/runs}"
mkdir -p "$OUT_DIR"
python3 "$ROOT/tools/sql_evidence.py" \\
  --sql-file "$(dirname "$0")/sql-evidence.sql" \\
  --output "$OUT_DIR/sql-evidence.json"
""" % shell_quote(str(root)),
        encoding="utf-8",
    )
    path.chmod(0o755)


def write_restart_runner(path: Path, root: Path) -> None:
    path.write_text(
        """#!/usr/bin/env bash
set -euo pipefail

if [[ ${RUN_RESTART_PROBE:-0} != 1 ]]; then
  echo 'Set RUN_RESTART_PROBE=1 to execute the guarded restart probe.' >&2
  exit 2
fi
if [[ $# -lt 2 ]]; then
  echo 'usage: run-restart-probe.sh COMPOSE_FILE SERVICE [OUTPUT_DIR]' >&2
  exit 2
fi
if [[ -z ${RESTART_BEFORE_COMMAND:-} || -z ${RESTART_AFTER_COMMAND:-} ]]; then
  echo 'RESTART_BEFORE_COMMAND and RESTART_AFTER_COMMAND are required.' >&2
  exit 2
fi

ROOT=%s
COMPOSE_FILE=$1
SERVICE=$2
OUT_DIR="${3:-$(dirname "$0")/runs}"
mkdir -p "$OUT_DIR"
ARGS=(
  --compose-file "$COMPOSE_FILE"
  --service "$SERVICE"
  --before-command "$RESTART_BEFORE_COMMAND"
  --after-command "$RESTART_AFTER_COMMAND"
  --output "$OUT_DIR/restart-evidence.json"
)
if [[ ${RESTART_DRY_RUN:-0} != 1 ]]; then
  ARGS+=(--execute)
fi
if [[ -n ${RESTART_CLEANUP_COMMAND:-} ]]; then
  ARGS+=(--cleanup-command "$RESTART_CLEANUP_COMMAND")
fi
if [[ -n ${RESTART_SQL_FILE:-} ]]; then
  ARGS+=(--sql-file "$RESTART_SQL_FILE")
fi
if [[ -n ${RESTART_DSN_ENV:-} ]]; then
  ARGS+=(--dsn-env "$RESTART_DSN_ENV")
fi
python3 "$ROOT/tools/restart_probe.py" "${ARGS[@]}"
""" % shell_quote(str(root)),
        encoding="utf-8",
    )
    path.chmod(0o755)


def main() -> int:
    args = parse_args()
    namespaces = set(args.namespace or [])
    ledger = args.ledger.expanduser().resolve()
    matrix = args.matrix.expanduser().resolve()
    try:
        rows = load_ledger(ledger, namespaces or None)
        matrix_rows = load_matrix(matrix)
        validation_errors = source_validation_errors(ledger, matrix, rows, matrix_rows)
    except (OSError, ValueError, json.JSONDecodeError) as error:
        print(f"partial probe generator: {error}", file=sys.stderr)
        return 2
    if args.check:
        summary = {
            "status": "FAIL" if validation_errors else "PASS",
            "ledger_rows_selected": len(rows),
            "matrix_rows": len(matrix_rows),
            "validation_errors": validation_errors,
        }
        if args.json:
            print(json.dumps(summary, ensure_ascii=False, indent=2, sort_keys=True))
        else:
            print("source validation: " + summary["status"])
            for error in validation_errors:
                print(f"  - {error}")
        return 1 if validation_errors else 0
    if validation_errors:
        print("partial probe generator: source validation failed", file=sys.stderr)
        for error in validation_errors:
            print(f"  - {error}", file=sys.stderr)
        return 2
    generated_at = dt.datetime.now(dt.timezone.utc).isoformat()
    output_dir = args.output_dir.expanduser().resolve()
    output_dir.mkdir(parents=True, exist_ok=True)

    session_entries = build_session_entries(rows, matrix_rows)
    restart_entries = build_restart_entries(rows, matrix_rows)
    ledger_hash = sha256(ledger)
    sql_text = build_sql(rows, generated_at, ledger_hash)

    session_counts = {
        "method_probe_available": sum(item["status"] == "METHOD_PROBE_AVAILABLE" for item in session_entries),
        "category_candidate_only": sum(item["status"] == "CATEGORY_CANDIDATE_ONLY" for item in session_entries),
        "manual_session_probe_required": sum(item["status"] == "MANUAL_SESSION_PROBE_REQUIRED" for item in session_entries),
    }
    restart_counts = {
        "method_restart_gate_available": sum(item["status"] == "METHOD_RESTART_GATE_AVAILABLE" for item in restart_entries),
        "category_restart_candidate_only": sum(item["status"] == "CATEGORY_RESTART_CANDIDATE_ONLY" for item in restart_entries),
        "category_restart_plan_only": sum(item["status"] == "CATEGORY_RESTART_PLAN_ONLY" for item in restart_entries),
        "manual_restart_probe_required": sum(item["status"] == "MANUAL_RESTART_PROBE_REQUIRED" for item in restart_entries),
    }
    explicit_mappings = sum(bool(declared_matrix_methods(item)) for item in matrix_rows)

    manifest = {
        "schema_version": 2,
        "generated_at": generated_at,
        "sources": {
            "ledger": str(ledger),
            "ledger_sha256": ledger_hash,
            "matrix": str(matrix),
            "matrix_sha256": sha256(matrix),
            "ledger_rows": len(load_ledger(ledger, None)),
            "matrix_rows": len(matrix_rows),
        },
        "selection": {"namespaces": sorted(namespaces)},
        "summary": {
            "partial_methods": len(rows),
            "session_entries": len(session_entries),
            **session_counts,
            "restart_entries": len(restart_entries),
            **restart_counts,
            "explicit_matrix_method_mappings": explicit_mappings,
            "sql_entries": len(rows),
        },
        "rows": [
            {
                "method": row["method"],
                "namespace": row["namespace"],
                "constructor_id": row["constructor_id"],
                "audit_status": row["audit_status"],
                "production_acceptance_status": row["production_acceptance_status"],
                "required_evidence": required_evidence(row),
                "candidate_matrix_ids": [str(item["id"]) for item in matrix_coverage(row, matrix_rows)["candidates"] if item.get("id")],
                "exact_matrix_ids": [str(item["id"]) for item in matrix_coverage(row, matrix_rows)["exact"] if item.get("id")],
            }
            for row in rows
        ],
    }

    (output_dir / "manifest.json").write_text(
        json.dumps(manifest, ensure_ascii=False, indent=2, sort_keys=True) + "\n", encoding="utf-8"
    )
    (output_dir / "session-probes.json").write_text(
        json.dumps(session_entries, ensure_ascii=False, indent=2, sort_keys=True) + "\n", encoding="utf-8"
    )
    (output_dir / "restart-probes.json").write_text(
        json.dumps(restart_entries, ensure_ascii=False, indent=2, sort_keys=True) + "\n", encoding="utf-8"
    )
    (output_dir / "sql-evidence.sql").write_text(sql_text, encoding="utf-8")
    write_session_runner(output_dir / "run-session-probes.sh", session_entries, ROOT)
    write_restart_runner(output_dir / "run-restart-probe.sh", ROOT)
    write_sql_runner(output_dir / "run-sql-evidence.sh", ROOT)

    summary = {
        "output_dir": str(output_dir),
        **manifest["summary"],
        "restart_methods": len(restart_entries),
    }
    if args.json:
        print(json.dumps(summary, ensure_ascii=False, indent=2, sort_keys=True))
    else:
        print("generated partial probe artifacts:")
        for key, value in summary.items():
            print(f"  {key}: {value}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
