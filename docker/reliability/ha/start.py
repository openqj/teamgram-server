"""Generate disposable Patroni configuration without putting secrets in argv."""
import json
import os
from pathlib import Path
import pwd

node = os.environ["PATRONI_NAME"]
password = os.environ["TEAMGRAM_RELIABILITY_DB_PASSWORD"]
replication = os.environ["TEAMGRAM_RELIABILITY_REPLICATION_PASSWORD"]
config = {
    "scope": "teamgram-lab", "namespace": "/teamgram/", "name": node,
    "restapi": {"listen": "0.0.0.0:8008", "connect_address": f"{node}:8008"},
    "etcd3": {"hosts": "dcs1:2379,dcs2:2379,dcs3:2379"},
    "bootstrap": {
        "dcs": {
            "ttl": 20, "loop_wait": 3, "retry_timeout": 3,
            "primary_start_timeout": 0, "primary_stop_timeout": 5, "check_timeline": True,
            "synchronous_mode": True, "synchronous_mode_strict": True,
            "synchronous_node_count": 1, "failsafe_mode": False,
            "postgresql": {"use_pg_rewind": True, "parameters": {
                "max_connections": 100, "shared_buffers": "128MB",
                "wal_log_hints": "on", "synchronous_commit": "on",
            }},
        },
        "initdb": [{"encoding": "UTF8"}, "data-checksums"],
        "pg_hba": ["local all all trust", "host replication replicator all scram-sha-256",
                   "host all all all scram-sha-256"],
    },
    "postgresql": {
        "listen": "0.0.0.0:5432", "connect_address": f"{node}:5432",
        "data_dir": "/var/lib/postgresql/18/patroni", "pgpass": "/tmp/patroni.pgpass",
        "bin_dir": "/usr/lib/postgresql/18/bin",
        "authentication": {
            "superuser": {"username": "teamgram_reliability", "password": password},
            "replication": {"username": "replicator", "password": replication},
        },
    },
    # No /dev/watchdog in a shared Docker Desktop VM. The probe tests a live
    # Patroni process demoting PostgreSQL; it cannot attest hardware fencing.
    "watchdog": {"mode": "off"},
}
user = pwd.getpwnam("postgres")
data = Path(config["postgresql"]["data_dir"])
data.mkdir(parents=True, exist_ok=True, mode=0o700)
os.chown(data, user.pw_uid, user.pw_gid)
path = Path("/tmp/patroni.json")
path.write_text(json.dumps(config))
path.chmod(0o600)
os.chown(path, user.pw_uid, user.pw_gid)
os.execvp("gosu", ["gosu", "postgres", "/opt/patroni/bin/patroni", str(path)])
