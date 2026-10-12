#!/usr/bin/env python3
"""Validate and summarize the Layer 229 method acceptance ledger.

This is intentionally read-only. It does not call a provider or mutate a
database. Production probes remain explicit commands; this tool makes their
resulting ledger evidence easy to audit and repeat.
"""

from __future__ import annotations

import argparse
import csv
import json
import re
import sys
from collections import Counter, defaultdict
from pathlib import Path
from typing import Any


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

FOCUS_GROUPS = {
    "premium": [
        "help.getPremiumPromo",
        "payments.canPurchaseStore",
        "payments.getPaymentForm",
        "payments.validateRequestedInfo",
        "payments.sendPaymentForm",
    ],
    "business": [
        "account.updateBusinessWorkHours",
        "account.updateBusinessLocation",
        "account.updateBusinessGreetingMessage",
        "account.updateBusinessAwayMessage",
        "account.updateBusinessIntro",
        "account.setMainProfileTab",
        "account.createBusinessChatLink",
        "account.getBusinessChatLinks",
        "account.resolveBusinessChatLink",
        "account.editBusinessChatLink",
        "account.deleteBusinessChatLink",
        "messages.getQuickReplies",
        "messages.reorderQuickReplies",
        "messages.checkQuickReplyShortcut",
        "messages.editQuickReplyShortcut",
        "messages.deleteQuickReplyShortcut",
        "messages.getQuickReplyMessages",
        "messages.sendQuickReplyMessages",
        "messages.deleteQuickReplyMessages",
    ],
    "web-ui": [
        "messages.startBot",
        "messages.requestMainWebView",
        "channels.createChannel",
        "channels.deleteChannel",
        "messages.sendMessage",
        "messages.getHistory",
        "channels.getFullChannel",
        "account.updateNotifySettings",
        "account.getNotifySettings",
        "contacts.addContact",
        "contacts.deleteContacts",
        "messages.createChat",
        "messages.getFullChat",
        "messages.editMessage",
        "messages.deleteMessages",
    ],
    "bot-webview": [
        "messages.startBot",
        "messages.getInlineBotResults",
        "messages.setInlineBotResults",
        "messages.sendInlineBotResult",
        "messages.editInlineBotMessage",
        "messages.getBotCallbackAnswer",
        "messages.setBotCallbackAnswer",
        "messages.setBotShippingResults",
        "messages.setBotPrecheckoutResults",
        "messages.requestWebView",
        "messages.prolongWebView",
        "messages.requestSimpleWebView",
        "messages.sendWebViewResultMessage",
        "messages.sendWebViewData",
        "messages.sendBotRequestedPeer",
        "messages.getBotApp",
        "messages.requestAppWebView",
        "messages.requestMainWebView",
        "messages.setBotGuestChatResult",
        "messages.requestChatJoinWebView",
        "messages.savePreparedInlineMessage",
        "messages.getPreparedInlineMessage",
        "bots.sendCustomRequest",
        "bots.answerWebhookJSONQuery",
        "bots.invokeWebViewCustomMethod",
        "bots.getPopularAppBots",
        "bots.checkDownloadFileParams",
        "bots.getBotRecommendations",
        "bots.requestWebViewButton",
        "bots.getRequestedWebViewButton",
        "bots.setJoinChatResults",
        "help.setBotUpdatesStatus",
    ],
}

PROVIDER_GAP_LABELS = (
    "signed_provider_boundary",
    "postgresql_or_component_only",
    "missing_real_provider",
    "missing_session_update_delivery_restart_evidence",
)

COMPONENT_ONLY_STATUSES = {
    "PARTIAL_COMPONENT_ONLY",
    "PARTIAL_ISOLATED_DB_ONLY",
    "ISOLATED_COMPONENT_ONLY",
    "ISOLATED_DB_ONLY",
}
COMPONENT_ONLY_AUDIT_STATUSES = {
    "PARTIAL_COMPONENT_VERIFIED",
    "PARTIAL_ISOLATED_DB_VERIFIED",
    "KNOWN_INCOMPLETE_STATIC",
}
PROVIDER_CAPABILITY_PATTERN = (
    r"provider|authority|registry|catalog(?:ue)?|feed|service|control\s+plane|"
    r"credentials?|secret|authenticator|relay|deployment|ledger"
)
PROVIDER_GAP_PATTERN = re.compile(
    rf"(?:\b(?:no|missing|without|lacks?|needs?|requires?|not|unconfigured|"
    rf"unavailable|absent|still)\b.{{0,100}}\b(?:{PROVIDER_CAPABILITY_PATTERN})\b|"
    rf"\b(?:{PROVIDER_CAPABILITY_PATTERN})\b.{{0,100}}\b(?:missing|not configured|"
    rf"unconfigured|unavailable|absent|remain(?:s)? open|still needs?)\b)"
)
MISSING_DELIVERY_MARKERS = (
    "no live",
    "no production",
    "no cross-client",
    "no cross-device",
    "session acceptance remain",
    "session acceptance unconfigured",
    "live session",
    "restart",
    "update fan-out",
    "update delivery",
    "message delivery",
    "webhook consumer",
    "entitlement delivery",
)

NEXT_STEPS = {
    "account": "Deploy a trusted account/Passport/Bot authority, then run a two-account Layer 229 session with restart and recovery checks.",
    "auth": "Deploy the trusted AuthProvider, application registry, SMS/email or WebAuthn provider, then run login and authsession restart checks.",
    "bots": "Deploy Bot App, webhook, recommendation and update-delivery authorities; exercise a bot session, callback/update fan-out and restart recovery.",
    "messages": "Connect the authoritative Bot/WebView/callback/payment/message producer and verify canonical updates across bot and user sessions, retries and restart.",
    "payments": "Deploy the real payment/receipt/Stars/gift provider and catalog; verify settlement, entitlement, refund/reconciliation, idempotent replay and session delivery.",
    "upload": "Deploy CDN token/public-key/AES/hash/reupload services and verify signed tokens, chunk integrity, expiry, reupload and failure recovery.",
    "ephemeral": "Deploy the callback-query/moderation authority and verify owner binding, canonical BotCallbackAnswer/report handling, delivery and expiry.",
    "fragment": "Deploy the Fragment ownership/price provider and verify signed lookup, ownership, currency/amount, session readback and retry semantics.",
    "help": "Deploy the missing help/CDN or catalog authority and verify signed results, key rotation, session behavior and expiry.",
    "phone": "Deploy non-loopback TURN/media credentials and verify signed call config, relay reachability, expiry and multi-party recovery.",
    "stories": "Deploy the live-story control plane and verify signed session allocation, participant/update delivery, expiry and restart recovery.",
}


def parse_args() -> argparse.Namespace:
    root = Path(__file__).resolve().parents[2]
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--ledger",
        type=Path,
        default=root / "teamgram-server" / "LAYER229_METHOD_LEDGER.csv",
        help="Layer 229 CSV ledger (default: repository ledger)",
    )
    parser.add_argument(
        "--methods",
        type=Path,
        default=root / "telegram-api" / "methods.json",
        help="Layer 229 source methods.json (default: telegram-api/methods.json)",
    )
    parser.add_argument(
        "--focus",
        action="append",
        choices=[*FOCUS_GROUPS, "blocked"],
        help="Print one or more focused method groups (repeatable).",
    )
    parser.add_argument("--json", action="store_true", help="Emit machine-readable JSON.")
    parser.add_argument(
        "--strict",
        action="store_true",
        help="Fail when the ledger is malformed or does not match methods.json.",
    )
    return parser.parse_args()


def load_methods(path: Path) -> dict[str, str]:
    source = json.loads(path.read_text(encoding="utf-8"))
    methods: dict[str, str] = {}
    for category in source.get("categories", []):
        for method in category.get("methods", []):
            name = str(method.get("name", ""))
            constructor_id = str(method.get("id", "")).lower().removeprefix("0x")
            if name:
                methods[name] = constructor_id
    return methods


def load_ledger(path: Path) -> tuple[list[dict[str, str]], list[str]]:
    errors: list[str] = []
    with path.open(newline="", encoding="utf-8") as handle:
        reader = csv.DictReader(handle)
        if reader.fieldnames != EXPECTED_FIELDS:
            errors.append(f"header mismatch: expected {EXPECTED_FIELDS}, got {reader.fieldnames}")
        rows: list[dict[str, str]] = []
        for line_number, row in enumerate(reader, start=2):
            if None in row:
                errors.append(f"line {line_number}: extra CSV fields")
            if any(row.get(field) is None for field in EXPECTED_FIELDS):
                errors.append(f"line {line_number}: missing CSV field")
            rows.append({field: (row.get(field) or "") for field in EXPECTED_FIELDS})
    return rows, errors


def build_report(ledger_path: Path, methods_path: Path) -> dict[str, Any]:
    rows, errors = load_ledger(ledger_path)
    source_methods = load_methods(methods_path)
    ledger_methods = {row["method"]: row["constructor_id"].lower().removeprefix("0x") for row in rows}

    duplicate_methods = sorted(
        method for method, count in Counter(row["method"] for row in rows).items() if count != 1
    )
    duplicate_ids = sorted(
        constructor_id
        for constructor_id, count in Counter(row["constructor_id"].lower().removeprefix("0x") for row in rows).items()
        if count != 1
    )
    missing = sorted(set(source_methods) - set(ledger_methods))
    extra = sorted(set(ledger_methods) - set(source_methods))
    id_mismatches = sorted(
        method
        for method in set(source_methods) & set(ledger_methods)
        if source_methods[method] != ledger_methods[method]
    )
    status_counts = {
        "audit": dict(sorted(Counter(row["audit_status"] for row in rows).items())),
        "production": dict(sorted(Counter(row["production_acceptance_status"] for row in rows).items())),
    }
    blocked_by_namespace: dict[str, list[str]] = defaultdict(list)
    for row in rows:
        if row["production_acceptance_status"] == "BLOCKED_BY_IMPLEMENTATION_GAP":
            blocked_by_namespace[row["namespace"]].append(row["method"])
    blocked_by_namespace = {key: sorted(value) for key, value in sorted(blocked_by_namespace.items())}
    blocked_rows = [
        row for row in rows if row["production_acceptance_status"] == "BLOCKED_BY_IMPLEMENTATION_GAP"
    ]
    provider_gap_summary = summarize_provider_gaps(blocked_rows)

    validation = {
        "row_count": len(rows),
        "unique_methods": len(ledger_methods),
        "unique_constructor_ids": len({row["constructor_id"] for row in rows}),
        "source_method_count": len(source_methods),
        "duplicate_methods": duplicate_methods,
        "duplicate_constructor_ids": duplicate_ids,
        "missing_methods": missing,
        "extra_methods": extra,
        "constructor_id_mismatches": id_mismatches,
        "csv_errors": errors,
        "matches_source": not (errors or duplicate_methods or duplicate_ids or missing or extra or id_mismatches),
    }
    return {
        "ledger": str(ledger_path),
        "methods_source": str(methods_path),
        "validation": validation,
        "status_counts": status_counts,
        "blocked_by_namespace": blocked_by_namespace,
        "blocked_total": sum(len(methods) for methods in blocked_by_namespace.values()),
        "provider_gap_summary": provider_gap_summary,
        "next_steps": {namespace: NEXT_STEPS.get(namespace, "Provide an authoritative provider, live session evidence, restart and failure-recovery checks.") for namespace in blocked_by_namespace},
        "rows": rows,
    }


def classify_provider_gaps(row: dict[str, str]) -> list[str]:
    """Classify explicit evidence gaps for one blocked method.

    These labels describe evidence still missing from the ledger. They are
    intentionally additive: a method may have a signed provider boundary and
    still lack the provider deployment or live session/update evidence.
    """
    text = " ".join(
        (row.get(field) or "")
        for field in ("audit_status", "evidence_or_gap", "production_acceptance_evidence")
    ).lower()
    labels: list[str] = []
    if re.search(
        r"\bsigned\s+(?:(?:[a-z0-9_-]+\s+)*[a-z0-9_-]*provider)\b|\bprovider\s+boundary\b",
        text,
    ):
        labels.append("signed_provider_boundary")
    if (
        row.get("production_acceptance_status") in COMPONENT_ONLY_STATUSES
        or row.get("audit_status") in COMPONENT_ONLY_AUDIT_STATUSES
        or "postgresql" in text
    ):
        labels.append("postgresql_or_component_only")
    if has_missing_provider_evidence(text):
        labels.append("missing_real_provider")
    if any(marker in text for marker in MISSING_DELIVERY_MARKERS):
        labels.append("missing_session_update_delivery_restart_evidence")
    return labels


def has_missing_provider_evidence(text: str) -> bool:
    """Return true only when a capability/provider is explicitly absent.

    A provider boundary can be present while its session or update consumer is
    still missing. Looking within a bounded sentence/context prevents generic
    phrases such as "session acceptance remains unconfigured" from being
    reported as a missing provider.
    """
    for sentence in re.split(r"[.;]", text):
        if PROVIDER_GAP_PATTERN.search(sentence):
            return True
    return False


def summarize_provider_gaps(blocked_rows: list[dict[str, str]]) -> dict[str, Any]:
    methods: dict[str, list[str]] = {label: [] for label in PROVIDER_GAP_LABELS}
    for row in blocked_rows:
        for label in classify_provider_gaps(row):
            methods[label].append(row["method"])
    for values in methods.values():
        values.sort()
    return {
        "scope": "production_acceptance_status == BLOCKED_BY_IMPLEMENTATION_GAP",
        "method_count": len(blocked_rows),
        "counts": {label: len(methods[label]) for label in PROVIDER_GAP_LABELS},
        "methods": methods,
    }


def focused_rows(report: dict[str, Any], focus: list[str]) -> dict[str, list[dict[str, str]]]:
    by_method = {row["method"]: row for row in report["rows"]}
    result: dict[str, list[dict[str, str]]] = {}
    for group in focus:
        names = (
            sorted(method for methods in report["blocked_by_namespace"].values() for method in methods)
            if group == "blocked"
            else FOCUS_GROUPS[group]
        )
        result[group] = [by_method[name] for name in names if name in by_method]
    return result


def print_text(report: dict[str, Any], focus: list[str]) -> None:
    validation = report["validation"]
    print("Layer 229 acceptance summary")
    print(f"ledger rows: {validation['row_count']}; unique methods: {validation['unique_methods']}; source methods: {validation['source_method_count']}")
    print(f"source match: {'PASS' if validation['matches_source'] else 'FAIL'}")
    print("production status counts:")
    for status, count in report["status_counts"]["production"].items():
        print(f"  {status}: {count}")
    print("blocked by namespace:")
    for namespace, methods in report["blocked_by_namespace"].items():
        print(f"  {namespace}: {len(methods)}; next: {report['next_steps'][namespace]}")
    print("provider gap summary (blocked methods):")
    provider_gaps = report["provider_gap_summary"]
    for label, count in provider_gaps["counts"].items():
        print(f"  {label}: {count}")
    if focus:
        for group, rows in focused_rows(report, focus).items():
            print(f"focus {group}: {len(rows)} methods")
            for row in rows:
                print(f"  {row['method']} [{row['production_acceptance_status']}] {row['production_acceptance_evidence']}")


def main() -> int:
    args = parse_args()
    try:
        report = build_report(args.ledger, args.methods)
    except (OSError, json.JSONDecodeError, csv.Error) as error:
        print(f"layer229 acceptance report failed: {error}", file=sys.stderr)
        return 2
    if args.json:
        output = dict(report)
        output["rows"] = focused_rows(report, args.focus or []) if args.focus else None
        print(json.dumps(output, ensure_ascii=True, indent=2, sort_keys=True))
    else:
        print_text(report, args.focus or [])
    if args.strict and not report["validation"]["matches_source"]:
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
