#!/usr/bin/env python3
"""Render the production Alertmanager webhook URL without logging the secret."""
from __future__ import annotations

import os
from pathlib import Path
import tempfile
from urllib.parse import urlsplit


PLACEHOLDER = "url: '${TEAMGRAM_ALERT_WEBHOOK_URL}'"


def render(template: str, webhook_url: str) -> str:
    parsed = urlsplit(webhook_url)
    if parsed.scheme != "https" or not parsed.netloc or "\n" in webhook_url or "\r" in webhook_url:
        raise ValueError("webhook URL must be a single-line HTTPS URL")
    if template.count(PLACEHOLDER) != 1:
        raise ValueError("Alertmanager template must contain one webhook URL placeholder")
    scalar = "'" + webhook_url.replace("'", "''") + "'"
    return template.replace(PLACEHOLDER, f"url: {scalar}")


def write_config(template_path: Path, output_path: Path, webhook_url: str) -> None:
    contents = render(template_path.read_text(encoding="utf-8"), webhook_url)
    output_path.parent.mkdir(parents=True, exist_ok=True)
    descriptor, temporary = tempfile.mkstemp(prefix=f".{output_path.name}.", dir=output_path.parent, text=True)
    try:
        os.fchmod(descriptor, 0o600)
        with os.fdopen(descriptor, "w", encoding="utf-8") as stream:
            stream.write(contents)
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(temporary, output_path)
        directory = os.open(output_path.parent, os.O_RDONLY)
        try:
            os.fsync(directory)
        finally:
            os.close(directory)
    except BaseException:
        try:
            os.unlink(temporary)
        except FileNotFoundError:
            pass
        raise


def main() -> int:
    template = Path(__file__).resolve().parents[2] / "teamgramd/deploy/alertmanager/alertmanager.production.yml"
    output = os.environ.get("TEAMGRAM_ALERTMANAGER_CONFIG")
    webhook_url = os.environ.get("TEAMGRAM_ALERT_WEBHOOK_URL")
    if not output or not webhook_url:
        print("TEAMGRAM_ALERTMANAGER_CONFIG and TEAMGRAM_ALERT_WEBHOOK_URL are required")
        return 1
    try:
        write_config(template, Path(output), webhook_url)
    except (OSError, ValueError):
        print("Alertmanager configuration rendering failed")
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
