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
    text = " ".join(
        row.get(field, "")
        for field in ("audit_status", "evidence_or_gap", "production_acceptance_evidence")
    ).lower()
    evidence = ["session"]
    if any(marker in text for marker in RESTART_MARKERS):
        evidence.append("backend_restart")
    if any(marker in text for marker in SQL_MARKERS):
        evidence.append("sql")
    return evidence


def matrix_candidates(row: dict[str, str], matrix: list[dict[str, Any]]) -> list[dict[str, Any]]:
    categories = category_for_namespace(row["namespace"])
    candidates = [item for item in matrix if item.get("category") in categories]
    return sorted(candidates, key=lambda item: str(item.get("id", "")))


def shell_quote(value: str) -> str:
    # POSIX single-quote escaping is sufficient for generated shell literals.
    return "'" + value.replace("'", "'\\''") + "'"


def sql_literal(value: str) -> str:
    return "'" + value.replace("'", "''") + "'"


def build_sql(rows: list[dict[str, str]], generated_at: str) -> str:
    values = ",\n".join(
        "  (" + ", ".join(
            sql_literal(row[field])
            for field in ("method", "namespace", "production_acceptance_status")
        ) + ")"
        for row in rows
    )
    return f"""-- Generated by tools/partial_probe_generator.py at {generated_at}
-- Read-only evidence only.  This does not mutate application tables.
BEGIN;
SET TRANSACTION READ ONLY;

SELECT current_database() AS database_name,
       current_user AS database_user,
       current_setting('server_version') AS server_version,
       pg_is_in_recovery() AS in_recovery,
       clock_timestamp() AT TIME ZONE 'UTC' AS captured_at_utc;

-- The tracked method list is deliberately data-only.  It ties this snapshot
-- to the exact ledger revision without guessing an implementation table.
SELECT tracked.method, tracked.namespace, tracked.production_acceptance_status
FROM (VALUES
{values}
) AS tracked(method, namespace, production_acceptance_status)
ORDER BY tracked.method;

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
        candidates = matrix_candidates(row, matrix)
        executable = [item for item in candidates if item.get("kind") in {"tsx", "shell"}]
        ids = [str(item["id"]) for item in executable if item.get("id")]
        required_env = sorted({
            str(name)
            for item in executable
            for name in item.get("required_env", [])
        })
        entries.append({
            "method": row["method"],
            "namespace": row["namespace"],
            "constructor_id": row["constructor_id"],
            "candidate_matrix_ids": ids,
            "required_env": required_env,
            "status": "MAPPED" if ids else "MANUAL_SESSION_PROBE_REQUIRED",
            "command": (
                ["python3", "tools/critical_path_acceptance.py", "--run", "--id", ids[0]]
                if ids else None
            ),
        })
    return entries


def build_restart_entries(rows: list[dict[str, str]], matrix: list[dict[str, Any]]) -> list[dict[str, Any]]:
    entries: list[dict[str, Any]] = []
    for row in rows:
        if "backend_restart" not in required_evidence(row):
            continue
        candidates = matrix_candidates(row, matrix)
        restart_ids = [
            str(item["id"])
            for item in candidates
            if item.get("gate") == "backend_restart"
        ]
        entries.append({
            "method": row["method"],
            "namespace": row["namespace"],
            "candidate_matrix_ids": restart_ids,
            "status": "MAPPED" if restart_ids else "MANUAL_RESTART_PROBE_REQUIRED",
            "command": (
                'python3 tools/restart_probe.py --compose-file "$ACCEPTANCE_COMPOSE_FILE" '
                '--service "$ACCEPTANCE_SERVICE" --execute --output restart-evidence.json'
            ),
        })
    return entries


def write_session_runner(path: Path, entries: list[dict[str, Any]], root: Path) -> None:
    ids = sorted({matrix_id for entry in entries for matrix_id in entry["candidate_matrix_ids"]})
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
    if not ids:
        lines.append("echo 'No executable matrix session probes map to the selected ledger rows.' >&2")
    for matrix_id in ids:
        safe = re.sub(r"[^A-Za-z0-9_.-]+", "_", matrix_id)
        lines.extend([
            f"echo '[{matrix_id}]' >&2",
            "python3 \"$ROOT/tools/critical_path_acceptance.py\" --run \\",
            f"  --id {shell_quote(matrix_id)} \\",
            f"  --output \"$OUT_DIR/{safe}.json\"",
            "",
        ])
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
python3 "$ROOT/tools/sql_evidence.py" \
  --sql-file "$(dirname "$0")/sql-evidence.sql" \
  --output "$OUT_DIR/sql-evidence.json"
""" % shell_quote(str(root)),
        encoding="utf-8",
    )
    path.chmod(0o755)


def main() -> int:
    args = parse_args()
    namespaces = set(args.namespace or [])
    ledger = args.ledger.expanduser().resolve()
    matrix = args.matrix.expanduser().resolve()
    rows = load_ledger(ledger, namespaces or None)
    matrix_rows = load_matrix(matrix)
    generated_at = dt.datetime.now(dt.timezone.utc).isoformat()
    output_dir = args.output_dir.expanduser().resolve()
    output_dir.mkdir(parents=True, exist_ok=True)

    session_entries = build_session_entries(rows, matrix_rows)
    restart_entries = build_restart_entries(rows, matrix_rows)
    sql_text = build_sql(rows, generated_at)

    manifest = {
        "schema_version": 1,
        "generated_at": generated_at,
        "sources": {
            "ledger": str(ledger),
            "ledger_sha256": sha256(ledger),
            "matrix": str(matrix),
            "matrix_sha256": sha256(matrix),
        },
        "selection": {"namespaces": sorted(namespaces)},
        "summary": {
            "partial_methods": len(rows),
            "session_entries": len(session_entries),
            "mapped_session_methods": sum(item["status"] == "MAPPED" for item in session_entries),
            "restart_entries": len(restart_entries),
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
                "candidate_matrix_ids": [
                    str(item["id"])
                    for item in matrix_candidates(row, matrix_rows)
                    if item.get("id")
                ],
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
