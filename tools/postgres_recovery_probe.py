#!/usr/bin/env python3
"""Prove PostgreSQL 18 recovery in a new, disposable local Compose project.

Without --execute this prints the plan. Execute mode never reuses an existing
project, container, network or volume, and removes only resources with this
run's ownership label. Passwords and rendered Compose environments are not
included in reports. Lab is also used by the isolated messaging probe.
"""

from __future__ import annotations

import argparse
import datetime as dt
import hashlib
import json
import os
from pathlib import Path
import re
import secrets
import signal
import socket
import subprocess
import sys
import time
import uuid
from typing import Any


ROOT = Path(__file__).resolve().parents[1]
COMPOSE_FILE = ROOT / "docker/reliability/compose.yaml"
RUN_LABEL = "org.teamgram.reliability.run"
PROJECT_PATTERN = re.compile(r"teamgram-reliability-[a-z0-9][a-z0-9-]{4,62}\Z")


class ProbeError(RuntimeError):
    pass


def redact(value: str, secret_values: list[str]) -> str:
    for secret in sorted(set(secret_values), key=len, reverse=True):
        if len(secret) >= 4:
            value = value.replace(secret, "<redacted>")
    return re.sub(r"(postgres(?:ql)?://)[^\s/@]+(?::[^\s/@]*)?@", r"\1<redacted>@", value)


def validate_project(project: str) -> None:
    if not PROJECT_PATTERN.fullmatch(project):
        raise ProbeError("project must be a new teamgram-reliability-* name (lowercase letters, digits and hyphens)")


class Lab:
    def __init__(self, project: str, *, port: int = 55435, timeout: float = 180,
                 wait: float = 120, env: dict[str, str] | None = None,
                 profiles: list[str] | None = None, compose_file: Path = COMPOSE_FILE):
        validate_project(project)
        if not 1024 <= port <= 65535 or timeout <= 0 or wait <= 0:
            raise ProbeError("port must be 1024..65535 and timeouts must be positive")
        self.project, self.port, self.timeout, self.wait = project, port, timeout, wait
        self.compose_file = compose_file.resolve()
        self.run_id = uuid.uuid4().hex
        self.env = dict(os.environ)
        self.env.update(env or {})
        for key in ("COMPOSE_FILE", "COMPOSE_PROJECT_NAME", "COMPOSE_PROFILES", "COMPOSE_ENV_FILES"):
            self.env.pop(key, None)
        self.env.update({
            "TEAMGRAM_RELIABILITY_RUN_ID": self.run_id,
            "TEAMGRAM_RELIABILITY_PG_PORT": str(port),
            "TEAMGRAM_RELIABILITY_DB": "teamgram_reliability",
            "TEAMGRAM_RELIABILITY_DB_PASSWORD": secrets.token_urlsafe(32),
            "TEAMGRAM_RELIABILITY_REPLICATION_PASSWORD": secrets.token_urlsafe(32),
            "TEAMGRAM_RELIABILITY_RECOVERY_TIMEOUT": str(max(1, int(min(timeout, wait)))),
        })
        self.secret_values = [value for key, value in self.env.items()
                              if re.search(r"PASSWORD|SECRET|TOKEN|AUTH|(?:^|_)KEY(?:$|_)", key, re.I)]
        self.profiles = profiles or []
        self.config: dict[str, Any] = {}
        self.started = False
        self.checked = False

    def run(self, command: list[str], *, timeout: float | None = None,
            input_text: str | None = None, check: bool = True) -> subprocess.CompletedProcess[str]:
        process = subprocess.Popen(command, cwd=ROOT, env=self.env, text=True,
                                   stdin=subprocess.PIPE if input_text is not None else subprocess.DEVNULL,
                                   stdout=subprocess.PIPE, stderr=subprocess.PIPE, start_new_session=True)
        try:
            stdout, stderr = process.communicate(input_text, timeout=timeout or self.timeout)
        except (subprocess.TimeoutExpired, KeyboardInterrupt) as error:
            try:
                os.killpg(process.pid, signal.SIGTERM)
            except ProcessLookupError:
                pass
            try:
                process.communicate(timeout=5)
            except subprocess.TimeoutExpired:
                try:
                    os.killpg(process.pid, signal.SIGKILL)
                except ProcessLookupError:
                    pass
                process.communicate(timeout=5)
            if isinstance(error, KeyboardInterrupt):
                raise
            raise ProbeError("lab command exceeded its timeout") from None
        result = subprocess.CompletedProcess(command, process.returncode, stdout, stderr)
        if check and result.returncode:
            detail = redact((stderr or stdout)[-3000:], self.secret_values).strip()
            raise ProbeError(f"lab command failed (exit {result.returncode}): {detail}")
        return result

    def command(self, *args: str) -> list[str]:
        command = ["docker", "compose", "--project-name", self.project,
                   "--project-directory", str(self.compose_file.parent), "--env-file", "/dev/null",
                   "-f", str(self.compose_file)]
        for profile in self.profiles:
            command += ["--profile", profile]
        return command + list(args)

    def resources(self) -> dict[str, list[str]]:
        filters = ["--filter", f"label=com.docker.compose.project={self.project}"]
        return {
            "container": self.run(["docker", "ps", "-aq", *filters]).stdout.split(),
            "volume": self.run(["docker", "volume", "ls", "-q", *filters]).stdout.split(),
            "network": self.run(["docker", "network", "ls", "-q", *filters]).stdout.split(),
        }

    def validate_config(self, config: dict[str, Any]) -> None:
        for kind in ("volumes", "networks"):
            for name, definition in config.get(kind, {}).items():
                if definition.get("external") or definition.get("name") != f"{self.project}_{name}":
                    raise ProbeError(f"lab {kind} must have project-scoped names and cannot be external")
                if definition.get("labels", {}).get(RUN_LABEL) != self.run_id:
                    raise ProbeError(f"lab {kind} ownership label is missing")
        for service in config.get("services", {}).values():
            if service.get("container_name") or service.get("network_mode") or service.get("privileged"):
                raise ProbeError("lab services cannot use container_name, network_mode or privileged mode")
            if service.get("labels", {}).get(RUN_LABEL) != self.run_id:
                raise ProbeError("lab service ownership label is missing")
            for binding in service.get("ports", []):
                if binding.get("host_ip") != "127.0.0.1":
                    raise ProbeError("lab published ports must bind only 127.0.0.1")
            for mount in service.get("volumes", []):
                if mount.get("type") != "bind":
                    continue
                source = Path(mount["source"]).resolve()
                try:
                    relative = source.relative_to(ROOT)
                except ValueError:
                    raise ProbeError("lab bind mounts must be read-only repository source files") from None
                if not mount.get("read_only") or "data" in relative.parts:
                    raise ProbeError("lab bind mounts cannot contain writable or existing data directories")

    def preflight(self) -> dict[str, Any]:
        if self.started:
            raise ProbeError("cannot preflight a running lab")
        self.run(["docker", "info", "--format", "{{.ServerVersion}}"])
        if any(self.resources().values()):
            raise ProbeError("refusing to reuse a project with existing containers, volumes or networks")
        rendered = self.run(self.command("--profile", "*", "config", "--format", "json")).stdout
        self.config = json.loads(rendered)
        self.validate_config(self.config)
        # Also reject unlabelled resources occupying the exact derived names.
        for kind in ("volume", "network"):
            for definition in self.config.get(f"{kind}s", {}).values():
                if self.run(["docker", kind, "inspect", definition["name"]], check=False).returncode == 0:
                    raise ProbeError(f"refusing to reuse an existing lab {kind} name")
        for service in self.config.get("services", {}).values():
            enabled_profiles = service.get("profiles", [])
            if enabled_profiles and not set(enabled_profiles).intersection(self.profiles):
                continue
            for binding in service.get("ports", []):
                with socket.socket() as connection:
                    try:
                        connection.bind(("127.0.0.1", int(binding["published"])))
                    except OSError:
                        raise ProbeError(f"lab port {binding['published']} is already in use") from None
        self.checked = True
        return {"status": "PASS", "compose_sha256": hashlib.sha256(self.compose_file.read_bytes()).hexdigest(),
                "project_was_empty": True, "published_postgres_port": self.port}

    def assert_owned(self) -> dict[str, list[str]]:
        resources = self.resources()
        for kind, identifiers in resources.items():
            if not identifiers:
                continue
            command = ["docker", "inspect"] if kind == "container" else ["docker", kind, "inspect"]
            for item in json.loads(self.run([*command, *identifiers]).stdout):
                labels = item.get("Config", {}).get("Labels", {}) if kind == "container" else item.get("Labels", {})
                if labels.get(RUN_LABEL) != self.run_id or labels.get("com.docker.compose.project") != self.project:
                    raise ProbeError(f"refusing to mutate a {kind} not created by this lab run")
        return resources

    def compose(self, *args: str, timeout: float | None = None) -> str:
        if not self.checked:
            raise ProbeError("preflight must pass before lab mutation")
        self.assert_owned()
        self.started = True
        return self.run(self.command(*args), timeout=timeout).stdout

    def start(self, *services: str) -> None:
        if not services:
            raise ProbeError("start requires explicit lab service names")
        self.compose("up", "-d", "--no-recreate", *services)

    def sql(self, service: str, sql: str) -> str:
        if service not in {"postgres", "postgres-standby", "postgres-pitr"}:
            raise ProbeError("SQL is restricted to lab PostgreSQL services")
        self.assert_owned()
        return self.run(self.command("exec", "-T", "-u", "postgres", "-e",
                                     "PGOPTIONS=-c statement_timeout=10000 -c lock_timeout=5000", service,
                                     "psql", "--no-psqlrc", "--no-password", "--set=ON_ERROR_STOP=1",
                                     "-U", "teamgram_reliability", "-d", "teamgram_reliability", "-Atq"),
                        input_text=sql, timeout=min(self.timeout, 20)).stdout.strip()

    def wait_sql(self, service: str, sql: str, expected: str) -> str:
        deadline = time.monotonic() + self.wait
        last_error = ""
        while time.monotonic() < deadline:
            try:
                value = self.sql(service, sql)
                if value == expected:
                    return value
                last_error = f"expected {expected!r}, received {value!r}"
            except ProbeError as error:
                last_error = str(error)
            time.sleep(0.5)
        raise ProbeError(f"{service} did not reach the expected SQL state: {last_error}")

    def container(self, service: str) -> dict[str, Any]:
        self.assert_owned()
        identifiers = self.run(self.command("ps", "--all", "-q", service)).stdout.split()
        if len(identifiers) != 1:
            raise ProbeError(f"expected one lab container for {service}")
        return json.loads(self.run(["docker", "inspect", identifiers[0]]).stdout)[0]

    def stop(self, service: str) -> None:
        self.compose("stop", "--timeout", "10", service)

    def fence_primary(self) -> dict[str, Any]:
        self.stop("postgres")
        item = self.container("postgres")
        if item["State"]["Running"] or item["HostConfig"]["RestartPolicy"]["Name"] not in {"", "no"}:
            raise ProbeError("old primary is not stopped with automatic restart disabled; refusing promotion")
        return {"old_primary_stopped": True, "automatic_restart_disabled": True,
                "old_primary_container_id": item["Id"]}

    def cleanup(self) -> dict[str, Any]:
        if not self.started:
            return {"status": "SKIPPED", "reason": "this run created no resources"}
        self.assert_owned()
        self.run(self.command("--profile", "*", "down", "--volumes", "--remove-orphans", "--timeout", "10"))
        remaining = self.resources()
        if any(remaining.values()):
            raise ProbeError("lab cleanup left project resources behind")
        self.started = False
        return {"status": "PASS", "remaining_resources": remaining}


def marker_snapshot(lab: Lab, service: str) -> dict[str, str]:
    value = lab.sql(service, "SELECT coalesce(jsonb_object_agg(id,payload),'{}'::jsonb) FROM reliability_marker;")
    return json.loads(value)


def require_snapshot(actual: dict[str, str], expected: dict[str, str], boundary: str) -> None:
    if actual != expected:
        raise ProbeError(f"{boundary} did not preserve the exact committed marker set")


def recovery_experiment(lab: Lab, report: dict[str, Any]) -> None:
    report["preflight"] = lab.preflight()
    lab.start("postgres", "postgres-migrate")
    lab.wait_sql("postgres", "SELECT current_setting('server_version_num')::integer/10000=18;", "t")
    lab.compose("wait", "postgres-migrate")
    migration = lab.container("postgres-migrate")
    if migration["State"]["ExitCode"] != 0:
        raise ProbeError("PostgreSQL migration runner failed")
    report["migration_count"] = int(lab.sql("postgres", "SELECT count(*) FROM schema_migrations;"))
    lab.sql("postgres", "CREATE TABLE reliability_marker(id text PRIMARY KEY,payload text NOT NULL);"
            "INSERT INTO reliability_marker VALUES ('baseline','base-backup-state');")
    lab.start("postgres-standby")
    lab.wait_sql("postgres-standby", "SELECT pg_is_in_recovery();", "t")
    lab.wait_sql("postgres", "SELECT count(*) FROM pg_stat_replication WHERE state='streaming';", "1")

    started = time.monotonic()
    lab.compose("exec", "-T", "postgres", "/bin/bash", "/reliability/basebackup.sh")
    report["base_backup"] = {"status": "PASS", "manifest_verified": True,
                             "duration_seconds": round(time.monotonic() - started, 3)}
    lab.sql("postgres", "INSERT INTO reliability_marker VALUES ('before_cutoff','recover-this-row');")
    expected_pitr = marker_snapshot(lab, "postgres")
    cutoff = lab.sql("postgres", "SELECT pg_create_restore_point('teamgram_reliability_cutoff');")
    lab.sql("postgres", "INSERT INTO reliability_marker VALUES ('after_cutoff','exclude-this-row');")
    archived_wal = lab.sql("postgres", "SELECT pg_walfile_name(pg_switch_wal());")
    if not re.fullmatch(r"[0-9A-F]{24}", archived_wal):
        raise ProbeError("unexpected archived WAL filename")
    deadline = time.monotonic() + lab.wait
    while True:
        archived = lab.run(lab.command("exec", "-T", "postgres", "test", "-s", f"/archive/{archived_wal}"), check=False)
        if archived.returncode == 0:
            break
        if time.monotonic() >= deadline:
            raise ProbeError("WAL did not reach the archive before its deadline")
        time.sleep(0.5)
    started = time.monotonic()
    lab.start("postgres-pitr")
    lab.wait_sql("postgres-pitr", "SELECT pg_is_in_recovery();", "f")
    restored = marker_snapshot(lab, "postgres-pitr")
    require_snapshot(restored, expected_pitr, "PITR")
    report["pitr"] = {"status": "PASS", "target_name": "teamgram_reliability_cutoff", "target_lsn": cutoff,
                       "archived_wal": archived_wal, "recovered_markers": sorted(restored),
                       "later_committed_marker_excluded": "after_cutoff" not in restored,
                       "rto_seconds": round(time.monotonic() - started, 3),
                       "rpo_scope": "exact named restore point; later commits intentionally excluded"}

    lab.sql("postgres", "INSERT INTO reliability_marker VALUES ('before_recreate','persistent-volume-state');")
    expected_restart = marker_snapshot(lab, "postgres")
    old_id = lab.container("postgres")["Id"]
    started = time.monotonic()
    lab.compose("up", "-d", "--no-deps", "--force-recreate", "postgres")
    lab.wait_sql("postgres", "SELECT pg_is_in_recovery();", "f")
    new_id = lab.container("postgres")["Id"]
    if old_id == new_id:
        raise ProbeError("primary recreation did not replace its container")
    require_snapshot(marker_snapshot(lab, "postgres"), expected_restart, "new-process recovery")
    report["new_process_recovery"] = {"status": "PASS", "container_replaced": True,
                                       "recovered_markers": sorted(expected_restart),
                                       "rto_seconds": round(time.monotonic() - started, 3), "lost_markers": 0}

    lab.sql("postgres", "INSERT INTO reliability_marker VALUES ('before_failover','acknowledged-before-fence');")
    acknowledged = marker_snapshot(lab, "postgres")
    acknowledged_lsn = lab.sql("postgres", "SELECT pg_current_wal_flush_lsn();")
    # This explicit replay barrier makes the experiment deterministic. It does
    # not turn asynchronous replication into a zero-loss crash guarantee.
    lab.wait_sql("postgres-standby", f"SELECT pg_last_wal_replay_lsn()>='{acknowledged_lsn}'::pg_lsn;", "t")
    require_snapshot(marker_snapshot(lab, "postgres-standby"), acknowledged, "pre-fence standby")
    started = time.monotonic()
    fence = lab.fence_primary()
    promoted = lab.sql("postgres-standby", f"SELECT pg_promote(true,{max(1, int(min(lab.wait, 60)))});")
    if promoted != "t":
        raise ProbeError("standby promotion failed")
    lab.wait_sql("postgres-standby", "SELECT pg_is_in_recovery();", "f")
    require_snapshot(marker_snapshot(lab, "postgres-standby"), acknowledged, "promoted standby")
    if lab.container("postgres")["State"]["Running"]:
        raise ProbeError("old primary unexpectedly restarted after promotion")
    lab.sql("postgres-standby", "INSERT INTO reliability_marker VALUES ('after_promotion','new-primary-writable');")
    report["manual_failover"] = {"status": "PASS", "fencing": fence, "acknowledged_lsn": acknowledged_lsn,
                                 "acknowledged_markers": sorted(acknowledged), "lost_markers": 0,
                                 "observed_rpo_markers": 0, "rto_seconds": round(time.monotonic() - started, 3),
                                 "new_primary_writable": True, "replication": "asynchronous with a verified replay barrier",
                                 "failure_model": "planned stop; old primary remains stopped before and after manual promotion"}


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--execute", action="store_true")
    parser.add_argument("--project-name", default=f"teamgram-reliability-{dt.datetime.now(dt.timezone.utc):%Y%m%d%H%M%S}-{secrets.token_hex(3)}")
    parser.add_argument("--port", type=int, default=55435)
    parser.add_argument("--timeout", type=float, default=180)
    parser.add_argument("--wait-seconds", type=float, default=120)
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    output = args.output or Path("/tmp") / f"{args.project_name}-postgres.json"
    report: dict[str, Any] = {"status": "PLAN", "project": args.project_name, "output": str(output),
                              "scope": "disposable local PostgreSQL 18; not production HA or MTProto acceptance",
                              "scenarios": ["verified physical base backup", "named-point PITR", "new-process recovery",
                                            "fenced manual standby promotion"],
                              "started_at": dt.datetime.now(dt.timezone.utc).isoformat()}
    lab: Lab | None = None
    try:
        lab = Lab(args.project_name, port=args.port, timeout=args.timeout, wait=args.wait_seconds, profiles=["recovery"])
        if args.execute:
            recovery_experiment(lab, report)
            report["status"] = "PASS"
    except (ProbeError, OSError, ValueError, KeyboardInterrupt) as error:
        report.update(status="FAIL", reason=redact(str(error), lab.secret_values if lab else []))
    finally:
        if lab and args.execute:
            try:
                report["cleanup"] = lab.cleanup()
            except (ProbeError, OSError, ValueError) as error:
                report.update(status="FAIL", cleanup={"status": "FAIL", "reason": redact(str(error), lab.secret_values)})
        report["finished_at"] = dt.datetime.now(dt.timezone.utc).isoformat()
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_text(json.dumps(report, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    print(json.dumps({key: report[key] for key in ("status", "project", "output", "reason") if key in report}, sort_keys=True))
    return 0 if report["status"] in {"PLAN", "PASS"} else 1


if __name__ == "__main__":
    sys.exit(main())
