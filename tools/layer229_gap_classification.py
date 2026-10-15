#!/usr/bin/env python3
"""Classify Layer 229 evidence gaps without changing the method ledger.

The ledger remains authoritative. This report adds derived scope and gap
labels so a reviewer can distinguish isolated fixture evidence from missing
session, update, restart, provider, and production-traffic acceptance.
"""

from __future__ import annotations

import argparse
import csv
import datetime as dt
import hashlib
import json
from pathlib import Path
import re
from typing import Any


ROOT = Path(__file__).resolve().parents[1]
DEFAULT_LEDGER = ROOT / "LAYER229_METHOD_LEDGER.csv"
DEFAULT_AUDIT = ROOT / "LAYER229_FUNCTIONAL_AUDIT.md"
DEFAULT_OUTPUT = Path("/tmp/teamgram-layer229-gap-classification.json")

EXPECTED_FIELDS = [
    "method",
    "constructor_id",
    "namespace",
    "rpc_route",
    "audit_status",
    "evidence_or_gap",
    "production_acceptance_status",
    "production_acceptance_evidence",
]

COMPONENT_STATUSES = {"PARTIAL_COMPONENT_ONLY", "ISOLATED_COMPONENT_ONLY"}
DATABASE_STATUSES = {"PARTIAL_ISOLATED_DB_ONLY", "ISOLATED_DB_ONLY"}
SESSION_STATUS = "PARTIAL_ISOLATED_SESSION_E2E"
TRANSPORT_STATUS = "TRANSPORT_ONLY"
PRODUCTION_ACCEPTED_STATUSES = {"PRODUCTION_ACCEPTED", "ACCEPTED", "PASS"}

NEGATIVE_WORD = re.compile(
    r"\b(?:no|not|without|missing|unverified|remain(?:s)?|still|yet|open|"
    r"unavailable|unsupported|pending|lack(?:s|ing)?|cannot|can't|only|"
    r"predates|pre[- ]dates|unaccepted|未验证|仍未|缺少|没有)\b",
    re.IGNORECASE,
)
SESSION_TERM = re.compile(
    r"\b(?:session|sessions|websocket|websocket/session|bff/session|layer 229)\b",
    re.IGNORECASE,
)
UPDATE_TERM = re.compile(
    r"\b(?:updates?|update[- ]fan[- ]out|fan[- ]out|recipient|delivery|"
    r"push|difference|pts|qts|seq|date)\b",
    re.IGNORECASE,
)
RESTART_TERM = re.compile(r"\b(?:restarts?|recovery|重启|恢复)\b", re.IGNORECASE)
PROVIDER_TERM = re.compile(
    r"\b(?:provider|authority|registry|catalog(?:ue)?|credentials?|secret|"
    r"service|control[- ]plane|relay|inventory|receipt|webhook|TURN|CDN)\b",
    re.IGNORECASE,
)
PRODUCTION_TERM = re.compile(
    r"\b(?:production|traffic|staging|external|real|deployment|live)\b",
    re.IGNORECASE,
)
FIXTURE_TERM = re.compile(
    r"\b(?:fixture|isolated|stub|mock|seed(?:ed)?|projection|local|focused|"
    r"component|postgres(?:ql)?|mysql|bufconn|test account|test-only)\b",
    re.IGNORECASE,
)


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--ledger", type=Path, default=DEFAULT_LEDGER)
    parser.add_argument("--audit", type=Path, default=DEFAULT_AUDIT)
    parser.add_argument("--output", type=Path, default=DEFAULT_OUTPUT)
    return parser.parse_args()


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def load_ledger(path: Path) -> list[dict[str, str]]:
    with path.open(newline="", encoding="utf-8") as handle:
        reader = csv.DictReader(handle)
        if reader.fieldnames != EXPECTED_FIELDS:
            raise ValueError(f"ledger header mismatch: expected {EXPECTED_FIELDS}, got {reader.fieldnames}")
        rows = [{field: row.get(field, "") for field in EXPECTED_FIELDS} for row in reader]
    methods = [row["method"] for row in rows]
    duplicates = sorted(method for method in set(methods) if methods.count(method) != 1)
    if duplicates:
        raise ValueError(f"duplicate ledger methods: {', '.join(duplicates)}")
    return rows


def audit_context(path: Path, methods: list[str]) -> tuple[dict[str, int], dict[str, list[str]]]:
    lines = path.read_text(encoding="utf-8").splitlines()
    pattern_by_method = {
        method: re.compile(rf"(?<![A-Za-z0-9_.]){re.escape(method)}(?![A-Za-z0-9_.])")
        for method in methods
    }
    mentions = {method: 0 for method in methods}
    sections: dict[str, set[str]] = {method: set() for method in methods}
    current_section = "(preamble)"
    for line in lines:
        heading = re.match(r"^#{1,6}\s+(.+?)\s*$", line)
        if heading:
            current_section = heading.group(1)
        for method, pattern in pattern_by_method.items():
            if pattern.search(line):
                mentions[method] += len(pattern.findall(line))
                sections[method].add(current_section)
    return mentions, {method: sorted(values) for method, values in sections.items()}


def sentences(text: str) -> list[str]:
    return [part.strip() for part in re.split(r"[\n.;!?]+", text) if part.strip()]


def has_negative_gap(text: str, term: re.Pattern[str]) -> bool:
    return any(NEGATIVE_WORD.search(sentence) and term.search(sentence) for sentence in sentences(text))


def has_positive_session(text: str) -> bool:
    return bool(
        re.search(
            r"(?:real|live|isolated|production|actual|complete|full)?\s*"
            r"(?:layer 229\s+)?(?:websocket/)?session",
            text,
            re.IGNORECASE,
        )
    )


def has_positive_update(text: str) -> bool:
    positive = re.compile(
        r"(?:returned|received|verified|read|persisted|pushed|delivered|hydrated|"
        r"fan[- ]out|updates?\s+(?:get|difference|flow|stream))",
        re.IGNORECASE,
    )
    return any(UPDATE_TERM.search(sentence) and positive.search(sentence) for sentence in sentences(text))


def has_positive_restart(text: str, method: str) -> bool:
    method_pattern = re.compile(re.escape(method), re.IGNORECASE)
    for sentence in sentences(text):
        if not RESTART_TERM.search(sentence) or NEGATIVE_WORD.search(sentence):
            continue
        if not re.search(
            r"(?:after|across|survive[sd]?|post|process|backend|service)\s+"
            r"(?:[^,]{0,60}\s+)?(?:restarts?|recovery)|"
            r"(?:restarts?|recovery)\s+(?:passed|verified|succeed|persistence|recovery)",
            sentence,
            re.IGNORECASE,
        ):
            continue
        positions = [match.start() for match in method_pattern.finditer(sentence)]
        restart_positions = [match.start() for match in RESTART_TERM.finditer(sentence)]
        if positions and restart_positions and min(abs(a - b) for a in positions for b in restart_positions) <= 120:
            return True
    return False


def scope_for(row: dict[str, str], text: str) -> tuple[str, list[str]]:
    production = row["production_acceptance_status"]
    audit = row["audit_status"]
    labels: list[str] = []
    if production == TRANSPORT_STATUS or audit == "SESSION_TRANSPORT_WRAPPER_NOT_BUSINESS_METHOD":
        return "transport_wrapper", ["transport_wrapper"]
    if production in {"BLOCKED_BY_IMPLEMENTATION_GAP"}:
        return "implementation_gap", ["implementation_gap"]
    if production in COMPONENT_STATUSES or audit in {"PARTIAL_COMPONENT_ONLY", "KNOWN_INCOMPLETE_STATIC"}:
        labels.append("component_only")
    if production in DATABASE_STATUSES or audit in {"PARTIAL_ISOLATED_DB_ONLY", "ISOLATED_DB_VERIFIED"}:
        labels.append("database_only")
    if production == SESSION_STATUS:
        labels.append("isolated_session")
    if FIXTURE_TERM.search(text):
        labels.append("fixture_scoped")
    if "isolated_session" in labels:
        return "isolated_session_fixture" if "fixture_scoped" in labels else "isolated_session", labels
    if "database_only" in labels:
        return "database_only", labels
    if "component_only" in labels:
        return "component_only", labels
    return "unclassified", labels


def classify_row(
    row: dict[str, str],
    audit_mentions: int,
    audit_sections: list[str],
) -> dict[str, Any]:
    text = " ".join(row[field] for field in ("audit_status", "evidence_or_gap", "production_acceptance_evidence"))
    scope, scope_labels = scope_for(row, text)
    not_applicable = scope == "transport_wrapper"
    if not_applicable:
        session_gap = update_gap = restart_gap = provider_gap = traffic_gap = False
    else:
        session_gap = (
            scope not in {"isolated_session", "isolated_session_fixture"}
            or not has_positive_session(row["production_acceptance_evidence"])
            or has_negative_gap(text, SESSION_TERM)
        )
        update_gap = (
            scope not in {"isolated_session", "isolated_session_fixture"}
            or not has_positive_update(row["production_acceptance_evidence"])
            or has_negative_gap(text, UPDATE_TERM)
        )
        restart_gap = (
            not has_positive_restart(row["production_acceptance_evidence"], row["method"])
            or has_negative_gap(text, RESTART_TERM)
        )
        provider_gap = has_negative_gap(text, PROVIDER_TERM) or (
            bool(PROVIDER_TERM.search(text))
            and bool(PRODUCTION_TERM.search(text))
            and not re.search(r"(?:passed|verified|returned|configured|available)", row["production_acceptance_evidence"], re.IGNORECASE)
        )
        traffic_gap = has_negative_gap(text, PRODUCTION_TERM)
    gaps = []
    if session_gap:
        gaps.append("session")
    if update_gap:
        gaps.append("update_delivery")
    if restart_gap:
        gaps.append("restart_recovery")
    if provider_gap:
        gaps.append("provider_authority")
    if traffic_gap:
        gaps.append("production_traffic")
    if not_applicable:
        gaps.append("not_applicable_transport")

    implementation_block = row["production_acceptance_status"] == "BLOCKED_BY_IMPLEMENTATION_GAP" or scope == "implementation_gap"
    if implementation_block:
        current_recommendation = "BLOCKED"
    elif scope in {"isolated_session", "isolated_session_fixture"}:
        current_recommendation = "PASS"
    else:
        current_recommendation = "MANUAL"
    if implementation_block:
        production_recommendation = "BLOCKED"
    elif row["production_acceptance_status"] in PRODUCTION_ACCEPTED_STATUSES:
        production_recommendation = "PASS"
    else:
        production_recommendation = "MANUAL"
    return {
        **row,
        "audit_context": {
            "mentions": audit_mentions,
            "sections": audit_sections,
        },
        "evidence_scope": scope,
        "evidence_scope_labels": scope_labels,
        "fixture_or_component_only": scope in {"component_only", "database_only", "isolated_session_fixture"},
        "not_applicable": not_applicable,
        "gaps": {
            "session": session_gap,
            "update_delivery": update_gap,
            "restart_recovery": restart_gap,
            "provider_authority": provider_gap,
            "production_traffic": traffic_gap,
        },
        "gap_labels": gaps,
        "recommendation": {
            "current_evidence": current_recommendation,
            "production_gate": production_recommendation,
        },
    }


def count_values(rows: list[dict[str, Any]], selector: Any) -> dict[str, int]:
    values: dict[str, int] = {}
    for row in rows:
        value = selector(row)
        values[value] = values.get(value, 0) + 1
    return dict(sorted(values.items()))


def build_report(ledger_path: Path, audit_path: Path) -> dict[str, Any]:
    rows = load_ledger(ledger_path)
    mentions, sections = audit_context(audit_path, [row["method"] for row in rows])
    classified = [classify_row(row, mentions[row["method"]], sections[row["method"]]) for row in rows]
    gap_counts = {
        label: sum(1 for row in classified if row["gaps"][label])
        for label in ("session", "update_delivery", "restart_recovery", "provider_authority", "production_traffic")
    }
    by_namespace: dict[str, dict[str, Any]] = {}
    for namespace in sorted({row["namespace"] for row in classified}):
        scoped = [row for row in classified if row["namespace"] == namespace]
        by_namespace[namespace] = {
            "method_count": len(scoped),
            "evidence_scope": count_values(scoped, lambda row: row["evidence_scope"]),
            "gap_counts": {
                label: sum(1 for row in scoped if row["gaps"][label])
                for label in ("session", "update_delivery", "restart_recovery", "provider_authority", "production_traffic")
            },
            "current_recommendation": count_values(scoped, lambda row: row["recommendation"]["current_evidence"]),
            "production_gate": count_values(scoped, lambda row: row["recommendation"]["production_gate"]),
        }
    return {
        "generated_at": dt.datetime.now(dt.timezone.utc).isoformat(),
        "scope_note": (
            "current_evidence PASS means the ledger row has isolated Layer 229 session evidence; "
            "production_gate is the release recommendation and remains MANUAL until real provider, "
            "session/update, restart/recovery, and production traffic gates are accepted."
        ),
        "sources": {
            "ledger": str(ledger_path),
            "ledger_sha256": sha256(ledger_path),
            "audit": str(audit_path),
            "audit_sha256": sha256(audit_path),
            "ledger_row_count": len(rows),
            "audit_line_count": len(audit_path.read_text(encoding="utf-8").splitlines()),
        },
        "summary": {
            "method_count": len(classified),
            "audit_mentioned_methods": sum(1 for row in classified if row["audit_context"]["mentions"]),
            "audit_unmentioned_methods": sum(1 for row in classified if not row["audit_context"]["mentions"]),
            "audit_mentions_total": sum(row["audit_context"]["mentions"] for row in classified),
            "not_applicable_methods": sum(1 for row in classified if row["not_applicable"]),
            "audit_status": count_values(classified, lambda row: row["audit_status"]),
            "production_acceptance_status": count_values(classified, lambda row: row["production_acceptance_status"]),
            "evidence_scope": count_values(classified, lambda row: row["evidence_scope"]),
            "gap_counts": gap_counts,
            "current_evidence_recommendation": count_values(classified, lambda row: row["recommendation"]["current_evidence"]),
            "production_gate_recommendation": count_values(classified, lambda row: row["recommendation"]["production_gate"]),
        },
        "by_namespace": by_namespace,
        "methods": classified,
    }


def main() -> int:
    args = parse_args()
    report = build_report(args.ledger.expanduser(), args.audit.expanduser())
    output = args.output.expanduser()
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(report, ensure_ascii=True, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    print(json.dumps({"report": str(output), "summary": report["summary"]}, ensure_ascii=True, indent=2, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
