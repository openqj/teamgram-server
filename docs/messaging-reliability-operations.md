# Messaging Reliability Operations

## Gateway Admission

The gateway enforces `MaxConnections`, handshake concurrency, worker-pool
capacity, WebSocket frame limits, and optional per-IP and per-auth-key limits.
These are process-local limits. `MaxConnectionsPerAuthKey` is not a distributed
account quota; use the shared session service for an account-wide limit across
gateway replicas.

The checked-in configs leave per-IP connection and handshake limits disabled
(`0`). Choose production limits from proxy behavior and measured NAT density;
do not apply a shared proxy address as though it uniquely identified clients.
Account-wide quotas remain a shared session-service responsibility. Alert on
gateway admission and worker-pool rejections.

Admission reads the socket remote address before the PROXY protocol header is
parsed. Behind a TCP proxy that hides client addresses, leave per-IP limits
disabled until the proxy preserves the client address at accept time. Applying
them to the proxy address can reject unrelated users together. The current
Compose deployment sets `nofile=131072`, `somaxconn=65535`, and
`tcp_max_syn_backlog=65535`; production hosts must apply equivalent limits and
verify the effective values after deployment.

During shutdown the gateway enters draining before stopping gnet. New sockets
are rejected, existing work gets the configured stop grace period, and
`gnetway_server_draining` remains set until the process exits.

## PostgreSQL And Delivery

The current runtime uses bounded `pgxpool` instances; PgBouncer is not part of
this deployment. Keep each service pool's `MaxConns` explicit and budget the
sum across all replicas and service processes below PostgreSQL `max_connections`,
leaving room for migrations, exporters, operators, replication, and failover.
The checked-in `teamgramd/etc2/sync.yaml` uses `MinConns: 1` and defaults
`TEAMGRAM_SYNC_POSTGRES_MAX_CONNS` to 16 in the Compose deployments;
two 5,000-recipient isolated A/B reruns kept message integrity and reduced the
aggregate Sync database stages, but did not meet the 2-second live-delivery
budget. Treat this as a measured local candidate, not a production capacity
guarantee, and recalculate the pool budget when adding replicas.
 A 32-connection, 1,000-user A/B did not produce a stable live-latency
improvement, so do not raise the default without a fixed-resource workload.
Do not raise pool sizes from a single saturation alert. Sync exports
`teamgram_sync_postgres_pool_connections`,
`teamgram_sync_postgres_pool_max_connections`, and pgxpool acquire, empty-pool
wait, and canceled-acquire snapshots. The cumulative snapshots reset on process
restart; compare their `delta()` over the same load window with
`mqconsumer_handler_queue_seconds` and `mqconsumer_record_age_seconds`. A high
empty-pool delta together with acquired connections near max supports a pool
capacity investigation. A low empty-pool delta while Kafka queue age remains
high points elsewhere in the pipeline. The message service
exports its pool use and acquire wait; PostgreSQL exporter collectors cover
connection usage and capacity, lock waits, long queries, WAL bytes, checkpoint
counters, archiver errors, and replication lag. The Compose deployment defaults
to `TEAMGRAM_POSTGRES_MAX_CONNECTIONS=256`, which leaves headroom for one
`runall-docker.sh` instance; multiply the pool budget and override this value
when running multiple backend replicas. The connection-capacity alert fires at
80% of usable connections for five minutes.

After a successful Sync fan-out, receipt completion updates are grouped into a
transaction of at most 64 rows with a 2ms collection window. The handler waits
for that transaction before Kafka acknowledgement, so retry and ordering
semantics remain unchanged. In a 1,000-user isolated run this reduced Sync pool
acquire count by 44.2% and cumulative empty-pool wait by 71.7%, while the full
delivery and recovery checks still passed. This is database-pressure evidence,
not an end-to-end live-latency guarantee; compare the pool snapshots with
`mqconsumer_record_age_seconds`, handler queue, gateway queue and client-visible
latency before changing the batch bounds.

The production Compose overlay requires `TEAMGRAM_KAFKA_DLQ_TOPIC`. Provision
that topic before rollout and manage its retention and ACLs as part of the
messaging deployment. Keep the bounded retry defaults explicit with
`TEAMGRAM_KAFKA_MAX_HANDLER_ATTEMPTS` and
`TEAMGRAM_KAFKA_RETRY_DELAY_MS`; the consumer acknowledges a failed record only
after the DLQ producer confirms publication. Verify retry, DLQ and
oldest-uncommitted metrics after a controlled failure. This Kafka DLQ is
separate from PostgreSQL outbox dead-letter rows and uses a different replay
procedure.

The Sync consumer also uses bounded per-key concurrency. Records with the same
Kafka key (the recipient user ID) stay in order, while different keys can be
handled concurrently; offset marks are committed only after all earlier
records in the partition have completed. `TEAMGRAM_KAFKA_HANDLER_CONCURRENCY`
defaults to `64` in the production overlay. Safe, receipt-backed Sync push
records use a bounded offset batch of up to 64 messages or 250ms; override
`TEAMGRAM_KAFKA_SAFE_COMMIT_BATCH_SIZE` only within the checked-in bound when
replaying a measured workload. Failed or non-idempotent records still force an
immediate commit boundary. Check `mqconsumer_record_age_seconds`,
`mqconsumer_handler_queue_seconds`, `mqconsumer_partition_lag`,
`mqconsumer_offset_commit_seconds`, `mqconsumer_oldest_uncommitted_seconds`,
gateway push latency, CPU, and PostgreSQL pool wait under the same workload
before changing either setting. The settings are process-local, so scale Sync
replicas with Kafka partition count and keep total handler concurrency within
the measured gateway, database, and session-service budget.

APIFull channel writes also expose
`teamgram_apifull_channel_persistence_seconds` with `stage=message_tx` and
`stage=outbox_write`. Compare these stages with client ACK and live-delivery
latency before attributing fan-out delay to PostgreSQL; the histogram covers
the backend persistence phase only. Channel send RPCs enqueue live delivery
after the canonical/outbox commit; the bounded background dispatch and the
outbox reconciler own retries after the client ACK returns.

Inspect dead letters without deleting or rewriting them:

```sql
SELECT id, state, attempts, max_attempts, dead_lettered_at, terminal_reason,
       last_error, octet_length(payload::text) AS payload_bytes, created_at
FROM msg_inbox_delivery_outbox
WHERE state = 2
ORDER BY dead_lettered_at, id;

SELECT id, state, attempts, max_attempts, dead_lettered_at, terminal_reason,
       last_error, octet_length(payload::text) AS payload_bytes, created_at
FROM msg_state_delivery_outbox
WHERE state = 2
ORDER BY dead_lettered_at, id;
```

Payload access may expose private message content. Restrict query access and
redact exported incident evidence. After the underlying fault is fixed, a
database administrator may grant `EXECUTE` on
`public.replay_teamgram_delivery(text,bigint,text,text,text)` to a dedicated
operator role. Replay is one transaction, requires operator, ticket, and
reason, writes a row to `msg_delivery_outbox_replay_audit`, and preserves the
payload. Example:

```sql
SELECT public.replay_teamgram_delivery(
  'inbox', 12345, 'oncall-user', 'INC-1234', 'Kafka broker recovered; replay approved'
);
```

The function accepts only `state = 2`; it resets the attempt count and makes
that intent due for normal ordered delivery. Review the audit record and the
outbox row after replay. Never clear an alert by deleting pending or terminal
rows.

## SLO Evidence

The 30-day evaluator uses 43,200 settled UTC minute buckets and keeps synthetic
and organic evidence separate. Schema version 2 records ACK and live delivery,
sampled connection retention, forced reconnect attempts, and recipient-history
loss/duplicate checks. The Layer 229 probe samples both connected sessions once
per second while its canary workload runs, forces one reconnect per account, and
checks each scheduled message against recipient history and live updates. These
are canary samples, not a census of every production user session.

The local observer writes isolated-test rows with `environment=local-isolated`;
those rows cannot trigger production burn alerts. A production Layer 229 canary
appends one immutable row per minute to a durable JSONL file with
`environment=production`, `source=synthetic-layer229`, planned and eligible
counts, all measurement groups, collector status, and raw evidence hash. Missing
minutes or SLI groups remain incomplete and cannot be treated as healthy.
Recovery incidents and drills are separate immutable records so an outage that
crosses a minute boundary does not require rewriting a settled bucket.

The checked-in systemd unit at
`docker/production/systemd/teamgram-messaging-slo.service` runs
`tools/messaging_slo_production_collector.py`; its timer invokes the Layer 229
probe once per minute. Provision two dedicated canary accounts and auth keys,
install the compatible GramJS checkout, provision the `teamgram-slo` service
user and `node_exporter` group, and configure the collector's private
environment file with `TEAMGRAM_SLO_REGION`, `TEAMGRAM_SLO_SHARD`,
`TEAMGRAM_SLO_GRAMJS_DIR`, `TG_DC_HOST`, `TG_DC_PORT`, `TG_API_ID`, `TG_API_HASH`,
and both accounts' `A_*` and `B_*` auth key, user ID and access hash values.
Point the target at production client ingress and keep the environment file
root-owned with mode `0600`. Prepare the data paths:

```sh
install -d -o teamgram-slo -g teamgram-slo -m 0700 /var/lib/teamgram/slo/raw
install -d -o teamgram-slo -g node_exporter -m 0755 /var/lib/node_exporter/textfile_collector
install -d -o root -g root -m 0750 /etc/teamgram
install -o root -g root -m 0600 messaging-slo.env /etc/teamgram/messaging-slo.env
```

Copy the service and timer into `/etc/systemd/system`, verify the absolute
checkout and GramJS paths in the service, and enable the timer:

```sh
systemctl daemon-reload
systemctl enable --now teamgram-messaging-slo.timer
systemctl list-timers teamgram-messaging-slo.timer
```

Set `TEAMGRAM_SLO_TEXTFILE_DIR` in the production Compose overlay to the same
textfile directory. Check the systemd journal along with
`TeamgramMessagingSloCollectorMissing` and `TeamgramMessagingSloEvidenceStale`;
a missed minute remains absent or failed and keeps the 30-day window
incomplete. Do not mark test credentials or a test target as production.

Rebuild the node_exporter textfile counters after each append or on a one-minute
systemd timer. The output is reconstructed from the append-only evidence so a
process restart does not reset the counts:

```sh
python3 tools/messaging_slo_metrics.py \
  /var/lib/teamgram/slo/production.jsonl \
  /var/lib/node_exporter/textfile_collector/teamgram_slo.prom
```

The textfile export includes `teamgram_slo_evidence_latest_timestamp_seconds`;
Prometheus alerts if the durable evidence stops advancing. Keep the evidence
writer and textfile rebuild on a supervised timer and alert on both writer exit
status and Prometheus scrape health. The generated aggregate `.prom` file is
mode `0644` for the read-only node_exporter container; the JSONL evidence stays
mode `0600` and is not mounted into that container.

Run node_exporter with
`--collector.textfile.directory=/var/lib/node_exporter/textfile_collector` and
load `teamgramd/deploy/prometheus/server/slo-recording.rules.yml`. The
production Compose overlay runs Prometheus, node_exporter and Alertmanager on
the private `teamgram_net`; Prometheus sends firing alerts to Alertmanager and
retains its TSDB in `prometheus_data`. Set `TEAMGRAM_SLO_TEXTFILE_DIR` to the
host directory containing `teamgram_slo.prom`. The production rules select only
production synthetic evidence. Set
`TEAMGRAM_ALERT_WEBHOOK_URL` as a secret and render the production Alertmanager
config before deployment. The renderer writes the output with mode `0600`, and
the production Compose overlay requires its path in
`TEAMGRAM_ALERTMANAGER_CONFIG`. Run the renderer as the dedicated Alertmanager
runtime user and set `TEAMGRAM_ALERTMANAGER_UID` and
`TEAMGRAM_ALERTMANAGER_GID` to that user's IDs so the container can read the
private file without broadening its permissions. Set
`TEAMGRAM_ALERTMANAGER_DATA_DIR` to a persistent directory owned by that user;
Alertmanager stores silence and notification state there.
Alertmanager 0.28.1 reads this rendered file with `--config.file`; it does not
support `--config.expand-env`.

```sh
TEAMGRAM_ALERTMANAGER_CONFIG=/etc/teamgram/alertmanager.yml \
TEAMGRAM_ALERTMANAGER_UID="$(id -u)" \
TEAMGRAM_ALERTMANAGER_GID="$(id -g)" \
TEAMGRAM_ALERTMANAGER_DATA_DIR=/var/lib/teamgram/alertmanager \
TEAMGRAM_ALERT_WEBHOOK_URL='https://alerts.example.invalid/hooks/replace-me' \
  python3 docker/production/render_alertmanager_config.py
```

Calculate the completed window from the durable source:

```sh
python3 tools/messaging_slo_window.py \
  /var/lib/teamgram/slo/production.jsonl \
  --environment production --region REGION --source synthetic-layer229 \
  --output /var/lib/teamgram/slo/30day-report.json
```

The evaluator currently uses candidate targets of 99.95% for ACK, live delivery,
connection samples, and reconnects; zero observed message loss or duplicates;
and a candidate 60-second recovery budget with 95% of recovery events within
budget. It also requires at least one recovery incident or drill in the window.
These thresholds need service-owner approval before becoming commitments. A
complete local test fixture is evaluator coverage only, not production SLO
acceptance.

## Manual Drill Record

Create one record per drill and retain it with the release evidence. Do not mark
a drill passed unless the alert reached the configured on-call destination and
the post-recovery message checks completed.

```yaml
drill_id: ""
started_at_utc: ""
finished_at_utc: ""
operator: ""
ticket: ""
git_commit: ""
image_digest: ""
configuration_sha256: ""
fault: "kafka-outage | postgres-lock | gateway-reconnect-storm | instance-replacement | dead-letter-replay"
injection: ""
alert_name: ""
notification_received_at_utc: ""
recovery_time_seconds: null
messages_expected: 0
messages_acked: 0
messages_live_delivered: 0
messages_lost: 0
messages_duplicated: 0
outbox_pending_before: 0
outbox_pending_after_recovery: 0
dead_letters_before: 0
dead_letters_after: 0
pts_contiguous: false
history_matches_live: false
result: "not-run"
follow_up: ""
```

Required scenarios are Kafka stop/recovery, PostgreSQL lock and WAL/checkpoint
pressure, gateway reconnect storm, backend and gateway replacement, and one
diagnosed dead-letter replay. Record the detected and recovered timestamps,
not only the injection command's duration. Append each completed production
incident or drill to the same evidence log after message recovery checks pass:

```sh
python3 tools/messaging_slo_recovery_event.py \
  /var/lib/teamgram/slo/production.jsonl \
  --environment production --region REGION --source synthetic-layer229 \
  --shard gateway-pair-1 --event-id INC-123-drill-1 --drill-id INC-123 \
  --started-at 2026-10-15T10:00:00Z --recovered-at 2026-10-15T10:00:42Z
```

The append is fsynced, mode `0600`, and idempotent for an exact event replay.
Reusing an event ID with different timestamps is rejected. The 30-day report
remains incomplete until the configured minimum recovery-event count is met.
