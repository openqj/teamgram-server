# Object storage

Teamgram DFS uses the standard S3 API. The historical `Minio` configuration
field and `minio-go` package name are compatibility names only; the default
deployment is SeaweedFS and RustFS is a supported independent target.

## Provider contract

Teamgram uses path-style addressing and `us-east-1` by default. Configure the
service with `S3_ENDPOINT` as `host:port` (without a scheme or path), plus
`S3_ACCESS_KEY`, `S3_SECRET_KEY`, `S3_USE_SSL`, and `S3_REGION`. The four DFS
buckets are `documents`, `encryptedfiles`, `photos`, and `videos`.

Run the real data-plane probe before changing a provider:

```bash
S3_ENDPOINT=127.0.0.1:8333 \
S3_ACCESS_KEY=replace-with-key S3_SECRET_KEY=replace-with-secret \
S3_BUCKET=documents go run ./tools/s3-contract-probe
```

The probe checks bucket access, single PUT/GET/HEAD/LIST/DELETE and multipart,
then removes its temporary objects. It does not create a bucket unless
`-create-bucket` or `S3_PROBE_CREATE_BUCKET=true` is supplied.

## Local SeaweedFS

```bash
cp .env.example .env
docker compose -f docker-compose-env.yaml up -d
docker compose up -d
```

The dependency stack runs a persistent single-node `weed mini` instance on
`127.0.0.1:8333`. This is a development/staging topology, not physical HA.
Do not copy `data/minio` into a SeaweedFS or RustFS data directory. Provider
disk formats are not interchangeable.

## Production HA

Use [the Swarm stack](../docker/object-storage/seaweedfs-ha-stack.yml) with
three Raft masters, volume servers on distinct hosts/disks, two Filers backed
by an external HA PostgreSQL cluster, two S3 Gateways, and HAProxy TLS
termination. The overlay network is encrypted and the stack consumes external
Docker secrets/configs. Follow its [runbook](../docker/object-storage/README.md)
for labels, volumes, TLS, and secret provisioning.

## Migration and recovery

Use the API migration tool for MinIO to SeaweedFS/RustFS. It preserves object
keys, bytes, size, content headers, and user metadata, verifies the target by
reading it back, and never deletes the source. Copy mode requires
`MIGRATION_CONFIRM_SOURCE_QUIESCED=YES`; versioned buckets are rejected until
version history and delete markers have an explicit migration plan.

For cross-domain operations use the wrappers:

- `tools/object-storage-backup.sh` requires different endpoints and location
  IDs, an independent target account/region, and source quiesce confirmation.
- `tools/object-storage-restore-drill.sh` requires a disposable target and
  sets `MIGRATION_REQUIRE_EMPTY_TARGET=true` before restoring.

Both emit credential-free JSON evidence. A local command or container health
check is not evidence of physical failure-domain durability; retain the
manifest and perform an application read acceptance after every restore drill.
