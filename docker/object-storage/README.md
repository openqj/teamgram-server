# Teamgram SeaweedFS S3 deployment

Teamgram talks to one stable S3 origin. The application never uses SeaweedFS
Master, Volume, or Filer APIs directly. The production shape is:

```text
Teamgram DFS -> TLS LB/VIP -> 2 S3 Gateways -> 2 Filers -> external HA PostgreSQL metadata
                                      \-> 3 Masters -> Volume Servers on 3 failure domains
```

`seaweedfs-ha-stack.yml` is a Docker Swarm production example. It has three
Raft masters, one volume server in each of three failure domains, two Filers
using an external HA PostgreSQL cluster, two S3 Gateways, and an HAProxy TLS
terminator. The overlay network is encrypted. Volumes, secrets, and configs
are external so a stack deploy cannot silently create a single-host database,
default credentials, or a plaintext certificate.

## Node labels and volumes

Label at least three independent nodes before deploying. Each label must map to
a real host, rack, and disk failure domain:

```bash
docker node update --label-add s3.master_domain=dc1 node-a
docker node update --label-add s3.volume_domain=dc1 --label-add s3.volume_disk=disk1 node-a
docker node update --label-add s3.filer_domain=dc1 --label-add s3.gateway_domain=dc1 node-a
docker node update --label-add s3.master_domain=dc2 node-b
docker node update --label-add s3.volume_domain=dc2 --label-add s3.volume_disk=disk1 node-b
docker node update --label-add s3.filer_domain=dc2 --label-add s3.gateway_domain=dc2 node-b
docker node update --label-add s3.master_domain=dc3 node-c
docker node update --label-add s3.volume_domain=dc3 --label-add s3.volume_disk=disk1 node-c
docker node update --label-add s3.gateway_domain=dc3 node-c
docker node update --label-add s3.lb=true node-lb
```

Create six node-local volumes on the nodes selected by the constraints. The
volume driver must point each name at a dedicated persistent disk; Docker
volumes on one host do not form independent failure domains:

```bash
for name in seaweedfs_master1 seaweedfs_master2 seaweedfs_master3 \
  seaweedfs_volume1 seaweedfs_volume2 seaweedfs_volume3; do
  docker volume create "$name"
done
```

Use your storage plugin or host mount policy to bind each volume to the label's
physical disk. Verify the mapping before the first object write.

## Secrets, TLS, and deploy

Create an HA PostgreSQL database and apply the SeaweedFS `filemeta` schema on
that cluster. Render `examples/filer.toml` with its private CA, host, user, and
password. Render `examples/s3.json` with the same access key and secret that
are stored in the two one-line credential files. The certificate bundle must
contain the LB DNS name in SAN and the private key, with mode `0600` before it
is handed to Docker. Obtain/renew it from the organization's ACME or internal
CA process; do not put the private key in Git.

Provision the external objects from a protected operator host:

```bash
FILER_CONFIG_FILE=/secure/rendered/filer.toml \
S3_CONFIG_FILE=/secure/rendered/s3.json \
S3_ACCESS_KEY_FILE=/secure/rendered/s3-access-key \
S3_SECRET_KEY_FILE=/secure/rendered/s3-secret-key \
TLS_BUNDLE_FILE=/secure/certs/teamgram-s3.pem \
HAPROXY_CONFIG_FILE=$PWD/docker/object-storage/examples/haproxy.cfg \
  ./docker/object-storage/provision-secrets.sh
```

Deploy after verifying the external HA PostgreSQL endpoint, certificate chain,
and node placement:

```bash
docker stack deploy -c docker/object-storage/seaweedfs-ha-stack.yml teamgram-s3
docker service ls --filter name=teamgram-s3
```

Set Teamgram's production environment to the LB DNS name and TLS port:

```dotenv
S3_ENDPOINT=s3.teamgram.example:443
S3_ACCESS_KEY=<secret-manager-value>
S3_SECRET_KEY=<secret-manager-value>
S3_USE_SSL=true
S3_REGION=us-east-1
```

The `200` replication policy is selected for three data centers. Confirm the
exact policy semantics against the pinned SeaweedFS release and capacity plan
before production acceptance; do not claim three physical copies from this
file alone.

## Backup and restore drill

Use the standard S3 migration tool. It copies current object versions and
verifies content, size, SHA-256, Content-Type, cache headers, and user
metadata. It never deletes the source and refuses versioned buckets because
version history/delete markers need an explicit version-aware migration.

Cross-domain backup requires a distinct endpoint and location ID, a separate
RustFS/S3 account or region, and an explicit quiesce confirmation:

```bash
BACKUP_TARGET_INDEPENDENT=true \
BACKUP_CONFIRM_SOURCE_QUIESCED=YES \
BACKUP_SOURCE_LOCATION_ID=teamgram-dc1 \
BACKUP_TARGET_LOCATION_ID=rustfs-dr-1 \
BACKUP_SOURCE_ACCOUNT_ID=teamgram-prod \
BACKUP_TARGET_ACCOUNT_ID=rustfs-dr \
SOURCE_S3_ENDPOINT=s3.teamgram.example:443 \
SOURCE_S3_ACCESS_KEY=... SOURCE_S3_SECRET_KEY=... SOURCE_S3_USE_SSL=true \
TARGET_S3_ENDPOINT=rustfs-dr.example:9000 \
TARGET_S3_ACCESS_KEY=... TARGET_S3_SECRET_KEY=... TARGET_S3_USE_SSL=true \
  ./tools/object-storage-backup.sh
```

Run a restore drill only against a disposable, explicitly empty target. The
wrapper sets `MIGRATION_REQUIRE_EMPTY_TARGET=true`, so an existing object in
any target bucket aborts the run:

```bash
RESTORE_TARGET_INDEPENDENT=true \
RESTORE_TARGET_DISPOSABLE=YES \
RESTORE_CONFIRM_SOURCE_QUIESCED=YES \
RESTORE_SOURCE_LOCATION_ID=rustfs-dr-1 \
RESTORE_TARGET_LOCATION_ID=restore-lab-2026-10 \
RESTORE_SOURCE_ACCOUNT_ID=rustfs-dr \
RESTORE_TARGET_ACCOUNT_ID=restore-lab \
SOURCE_S3_ENDPOINT=rustfs-dr.example:9000 \
SOURCE_S3_ACCESS_KEY=... SOURCE_S3_SECRET_KEY=... SOURCE_S3_USE_SSL=true \
TARGET_S3_ENDPOINT=restore-lab.example:9000 \
TARGET_S3_ACCESS_KEY=... TARGET_S3_SECRET_KEY=... TARGET_S3_USE_SSL=true \
  ./tools/object-storage-restore-drill.sh
```

Both wrappers write credential-free JSON evidence. A successful local script
run is not proof of cross-host durability; retain the evidence with the backup
manifest and separately record the provider account, failure-domain, key
recovery, object-count, and application read acceptance.
