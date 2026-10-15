#!/usr/bin/env python3
"""Exercise Patroni single-node failover and DCS quorum loss in a disposable lab."""
from __future__ import annotations

import argparse
import datetime as dt
import hashlib
import json
from pathlib import Path
import re
import secrets
import time

from postgres_recovery_probe import Lab, ProbeError, ROOT, redact

COMPOSE = ROOT / "docker/reliability/ha/compose.yaml"
NODES = ("pg1", "pg2", "pg3")


def api(lab: Lab, node: str, endpoint: str = "patroni") -> dict:
    code = ("import json,urllib.request; "
            f"print(urllib.request.urlopen('http://127.0.0.1:8008/{endpoint}',timeout=2).read().decode())")
    result = lab.run(lab.command("exec", "-T", node, "python3", "-c", code), check=False, timeout=5)
    return json.loads(result.stdout) if result.returncode == 0 else {}


def roles(lab: Lab) -> dict:
    return {node: api(lab, node) for node in NODES}


def leader(lab: Lab, exclude: str | None = None) -> str | None:
    states = roles(lab)
    leaders = [node for node, state in states.items() if state.get("role") in ("master", "primary") and state.get("state") == "running"]
    if len(leaders) > 1:
        raise ProbeError("multiple writable Patroni leaders observed")
    for node, state in states.items():
        if lab.container(node)["State"]["Running"] and not state:
            return None
    candidate = leaders[0] if len(leaders) == 1 and leaders[0] != exclude else None
    return candidate if candidate and api(lab, candidate, "primary") else None


def sql(lab: Lab, host: str, statement: str, *, check: bool = True) -> str | None:
    executor = next((node for node in NODES if lab.container(node)["State"]["Running"]
                     and lab.container(node)["NetworkSettings"]["Networks"]), None)
    if executor is None:
        raise ProbeError("no running lab PostgreSQL client")
    command = 'PGPASSWORD="$TEAMGRAM_RELIABILITY_DB_PASSWORD" PGCONNECT_TIMEOUT=2 PGOPTIONS="-c statement_timeout=3000 -c lock_timeout=2000" exec psql --no-psqlrc --no-password -v ON_ERROR_STOP=1 -U teamgram_reliability -d postgres -Atq -h "$1"'
    result = lab.run(lab.command("exec", "-T", executor, "bash", "-c", command, "psql", host),
                     input_text=statement, timeout=7, check=False)
    if result.returncode:
        if check:
            raise ProbeError("HA database SQL failed or exceeded its bounded statement timeout")
        return None
    return result.stdout.strip()


def local_write_rejected(lab: Lab, node: str) -> bool:
    # Execute inside the isolated node; a proxy routing change alone is not fencing.
    command = lab.command("exec", "-T", "-u", "postgres", node, "psql", "-h", "/var/run/postgresql",
        "-U", "teamgram_reliability", "-d", "postgres", "-v", "ON_ERROR_STOP=1", "-v", "VERBOSITY=verbose", "-Atq")
    state = lab.run(command, input_text="SELECT pg_is_in_recovery();", check=False, timeout=5)
    if state.returncode or state.stdout.strip() != "t":
        return False
    result = lab.run(command,
        input_text="SET statement_timeout='1500ms'; BEGIN; INSERT INTO ha_marker VALUES ('fence_violation','must-not-commit') ON CONFLICT DO NOTHING; ROLLBACK;",
        check=False, timeout=5)
    if result.returncode == 0:
        return False
    # A missing table, authentication failure, or timeout must fail the probe;
    # only PostgreSQL's read-only SQLSTATE proves the old primary is fenced.
    if result.returncode != 3 or not re.search(r"ERROR:\s+25006:", result.stderr):
        raise ProbeError("direct fencing check failed for a reason other than read-only SQLSTATE 25006")
    return True


def demoted(lab: Lab, node: str) -> bool:
    state = api(lab, node)
    return bool(state) and state.get("role") == "replica" and local_write_rejected(lab, node)


def wait_for(lab: Lab, description: str, predicate):
    deadline = time.monotonic() + lab.wait
    while time.monotonic() < deadline:
        value = predicate()
        if value:
            return value
        time.sleep(1)
    raise ProbeError(f"deadline exceeded waiting for {description}")


def sync_state(lab: Lab) -> dict:
    result = lab.run(lab.command("exec", "-T", "dcs1", "etcdctl", "--endpoints=http://dcs1:2379",
                                "get", "/teamgram/teamgram-lab/sync", "--print-value-only"), check=False, timeout=5)
    return json.loads(result.stdout) if result.returncode == 0 and result.stdout.strip() else {}


def synchronous(lab: Lab, primary: str) -> bool:
    # PostgreSQL can select a sync standby before Patroni has registered that
    # standby as eligible for safe automatic promotion in the DCS.
    state = sync_state(lab)
    eligible = str(state.get("sync_standby", "")).split(",")
    if state.get("leader") != primary or not any(node in NODES and node != primary for node in eligible):
        return False
    names = sql(lab, "proxy", "SELECT coalesce(json_agg(application_name),'[]'::json) FROM pg_stat_replication WHERE state='streaming' AND sync_state IN ('sync','quorum');", check=False)
    return bool(names) and any(node in eligible for node in json.loads(names))


def commit(lab: Lab, name: str) -> dict:
    value = sql(lab, "proxy", f"BEGIN; SET LOCAL synchronous_commit=remote_apply; INSERT INTO ha_marker VALUES ('{name}','{lab.run_id}'); COMMIT; SELECT json_build_object('server',inet_server_addr()::text,'lsn',pg_current_wal_flush_lsn()::text,'synchronous_standby_names',current_setting('synchronous_standby_names')); ")
    return {"id": name, "commit_ack": True, **json.loads(value)}


def snapshot(lab: Lab) -> dict[str, str]:
    return json.loads(sql(lab, "proxy", "SELECT coalesce(json_object_agg(id,payload),'{}'::json) FROM ha_marker;"))


def experiment(lab: Lab, report: dict) -> None:
    report["preflight"] = lab.preflight()
    report["images"] = {}
    for image in (lab.env["TEAMGRAM_RELIABILITY_HA_IMAGE"], "haproxy:3.2-alpine"):
        report["images"][image] = lab.run(["docker", "image", "inspect", image, "--format", "{{.Id}}"]).stdout.strip()
    report["haproxy_config_sha256"] = hashlib.sha256((COMPOSE.parent / "haproxy.cfg").read_bytes()).hexdigest()
    lab.start("dcs1", "dcs2", "dcs3", *NODES, "proxy")
    initial = wait_for(lab, "one initial leader", lambda: leader(lab))
    wait_for(lab, "initial synchronous standby registered for promotion", lambda: synchronous(lab, initial))
    wait_for(lab, "all three PostgreSQL members", lambda: all(v.get("state") == "running" for v in roles(lab).values()))
    wait_for(lab, "both initial streaming replicas", lambda: sql(lab, "proxy", "SELECT count(*)=2 FROM pg_stat_replication WHERE state='streaming';", check=False) == "t")
    if sql(lab, "proxy", "SELECT current_setting('server_version_num')::int/10000;") != "18":
        raise ProbeError("HA probe requires PostgreSQL 18")
    config = api(lab, initial, "config")
    if (config.get("synchronous_mode") is not True or config.get("synchronous_mode_strict") is not True
            or config.get("failsafe_mode") is not False):
        raise ProbeError("synchronous strict mode and DCS demotion must be active")
    sql(lab, "proxy", "CREATE TABLE ha_marker(id text PRIMARY KEY,payload text NOT NULL);")
    report["configuration"] = {"synchronous_mode": True, "synchronous_mode_strict": True,
        "synchronous_node_count": config.get("synchronous_node_count"), "failsafe_mode": False,
        "ttl": config.get("ttl"), "watchdog": "off; shared Docker Desktop VM"}
    report["patroni_version"] = api(lab, initial).get("patroni", {}).get("version")
    report["initial_sync_state"] = sync_state(lab)
    report["commits"] = [commit(lab, "before_crash")]
    expected = {"before_crash": lab.run_id}
    old = lab.container(initial)
    lab.assert_owned()
    started = time.monotonic()
    lab.run(["docker", "kill", old["Id"]])
    new = wait_for(lab, "automatic leader after container crash", lambda: leader(lab, initial))
    election_seconds = round(time.monotonic()-started, 3)
    sync_started = time.monotonic()
    wait_for(lab, "synchronous standby and proxy after crash", lambda: synchronous(lab, new))
    sync_seconds = round(time.monotonic()-sync_started, 3)
    if snapshot(lab) != expected:
        raise ProbeError("acknowledged marker lost after primary crash")
    report["commits"].append(commit(lab, "after_crash"))
    expected["after_crash"] = lab.run_id
    report["automatic_failover"] = {"status": "PASS", "old_leader": initial, "new_leader": new,
        "leader_observed_seconds": election_seconds, "sync_ready_seconds": sync_seconds,
        "observed_write_recovery_seconds": round(time.monotonic()-started, 3),
        "acknowledged_markers_lost": 0,
        "fault": "whole primary container SIGKILL; no manual promote"}
    lab.start(initial)
    wait_for(lab, "former leader to rejoin as replica", lambda: api(lab, initial).get("role") == "replica")
    wait_for(lab, "both replicas streaming before the next fault", lambda: sql(lab, "proxy", "SELECT count(*)=2 FROM pg_stat_replication WHERE state='streaming';", check=False) == "t")
    wait_for(lab, "registered sync standby before partition", lambda: synchronous(lab, new))
    report["rejoin"] = {"status": "PASS", "former_leader_role": "replica"}

    report["commits"].append(commit(lab, "before_partition"))
    expected["before_partition"] = lab.run_id
    old = lab.container(new)
    network = f"{lab.project}_default"
    address = old["NetworkSettings"]["Networks"][network]["IPAddress"]
    lab.assert_owned()
    started = time.monotonic()
    lab.run(["docker", "network", "disconnect", network, old["Id"]])
    try:
        # Directly inspect the old primary before accepting a new leader.
        demotion_started = time.monotonic()
        wait_for(lab, "isolated old primary to demote and reject local writes", lambda: demoted(lab, new))
        demotion_seconds = round(time.monotonic()-demotion_started, 3)
        sync_started = time.monotonic()
        third = wait_for(lab, "automatic failover after network isolation", lambda: leader(lab, new))
        election_seconds = round(time.monotonic()-sync_started, 3)
        sync_ready_started = time.monotonic()
        wait_for(lab, "synchronous standby and proxy after isolation", lambda: synchronous(lab, third))
        sync_seconds = round(time.monotonic()-sync_ready_started, 3)
        if not local_write_rejected(lab, new) or snapshot(lab) != expected:
            raise ProbeError("partition fencing or acknowledged data verification failed")
        report["commits"].append(commit(lab, "after_partition"))
        expected["after_partition"] = lab.run_id
        report["network_partition"] = {"status": "PASS", "old_leader": new, "new_leader": third,
            "old_primary_local_write_rejected": True, "acknowledged_markers_lost": 0,
            "demotion_seconds": demotion_seconds, "leader_observed_seconds": election_seconds,
            "sync_ready_seconds": sync_seconds,
            "observed_write_recovery_seconds": round(time.monotonic()-started, 3), "fencing": "live Patroni software demotion; recovery=true and SQLSTATE 25006"}
    finally:
        lab.assert_owned()
        lab.run(["docker", "network", "connect", "--alias", new, "--ip", address, network, old["Id"]])
    wait_for(lab, "partitioned member to rejoin", lambda: api(lab, new).get("role") == "replica")
    wait_for(lab, "both replicas streaming before quorum fault", lambda: sql(lab, "proxy", "SELECT count(*)=2 FROM pg_stat_replication WHERE state='streaming';", check=False) == "t")
    report["commits"].append(commit(lab, "before_quorum_loss"))
    expected["before_quorum_loss"] = lab.run_id
    started = time.monotonic()
    lab.stop("dcs2")
    dcs2_stopped = time.monotonic()
    lab.stop("dcs3")
    dcs_quorum_stopped = round(time.monotonic()-started, 3)
    try:
        wait_for(lab, "all leaders to demote on DCS quorum loss", lambda: all(
            value and value.get("role") == "replica" for value in roles(lab).values()))
        if not all(local_write_rejected(lab, node) for node in NODES):
            raise ProbeError("a PostgreSQL node accepted writes without DCS quorum")
        if sql(lab, "proxy", "BEGIN; INSERT INTO ha_marker VALUES ('proxy_fence_violation','bad'); ROLLBACK;", check=False) is not None:
            raise ProbeError("HAProxy accepted writes without DCS quorum")
        report["quorum_loss"] = {"status": "PASS", "all_local_writes_rejected": True,
            "proxy_write_rejected": True, "dcs_stop_seconds": dcs_quorum_stopped,
            "dcs2_stop_to_dcs3_stop_seconds": round(time.monotonic()-dcs2_stopped, 3),
            "demotion_seconds": round(time.monotonic()-started, 3)}
    finally:
        lab.start("dcs2", "dcs3")
    restored = wait_for(lab, "leader after quorum restoration", lambda: leader(lab))
    wait_for(lab, "synchronous standby and proxy after quorum restoration", lambda: synchronous(lab, restored))
    if snapshot(lab) != expected:
        raise ProbeError("marker set changed after quorum recovery")
    report["commits"].append(commit(lab, "after_quorum_restore"))
    report["recovery"] = {"status": "PASS", "single_leader": restored, "acknowledged_markers_lost": 0}


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--execute", action="store_true")
    parser.add_argument("--image", default="teamgram-patroni:4.1.5-pg18")
    parser.add_argument("--port", type=int, default=55437)
    parser.add_argument("--output", type=Path, default=Path("/tmp/teamgram-postgres-ha.json"))
    args = parser.parse_args()
    report = {"status": "PLAN", "scope": "local PG18 automatic HA; single node and live-Patroni DCS faults",
        "productionHaAccepted": False, "hardFencingTested": False,
        "limitations": ["one Docker host", "one HAProxy", "no watchdog/STONITH or frozen-Patroni fencing",
                        "not communication-stack HA", "synchronous standby plus primary double failure not covered"],
        "started_at": dt.datetime.now(dt.timezone.utc).isoformat()}
    lab = None
    try:
        lab = Lab("teamgram-reliability-ha-"+secrets.token_hex(6), port=args.port, timeout=180, wait=100,
                  compose_file=COMPOSE, env={"TEAMGRAM_RELIABILITY_HA_IMAGE": args.image})
        report["project"] = lab.project
        if args.execute:
            experiment(lab, report)
            report["status"] = "PASS"
    except (ProbeError, OSError, ValueError, KeyboardInterrupt) as error:
        report.update(status="FAIL", reason=redact(str(error), lab.secret_values if lab else []))
        if lab and lab.started:
            try:
                report["failure_states"] = roles(lab)
                report["failure_sync_state"] = sync_state(lab)
                report["failure_logs"] = {}
                for node in (*NODES, "proxy", "dcs1"):
                    result = lab.run(lab.command("logs", "--tail", "70", node), check=False)
                    report["failure_logs"][node] = redact((result.stdout + result.stderr)[-14000:], lab.secret_values)
            except (ProbeError, OSError, ValueError):
                pass
    finally:
        if lab and args.execute:
            try:
                report["cleanup"] = lab.cleanup()
            except (ProbeError, OSError, ValueError) as error:
                report.update(status="FAIL", cleanup={"status":"FAIL", "reason":redact(str(error), lab.secret_values)})
        report["finished_at"] = dt.datetime.now(dt.timezone.utc).isoformat()
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(json.dumps(report, indent=2, sort_keys=True)+"\n")
    print(json.dumps({"status":report["status"], "output":str(args.output), "reason":report.get("reason")}))
    return 0 if report["status"] in ("PLAN", "PASS") else 1


if __name__ == "__main__":
    raise SystemExit(main())
