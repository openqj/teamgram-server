# PostgreSQL 18 migrations

The PostgreSQL SQL files live in `../sql/postgres`; this directory contains the
runner and portability checks. The historical files under `../sql/` are the
transitional Teamgram/MySQL schema and are not inputs to this runner.

The project has not launched and has no historical data to preserve. A new
environment therefore starts from an empty PostgreSQL 18 database and applies
these migrations in filename order. Do not add compatibility branches for
MySQL or copy MySQL DDL into this directory.

The current fresh-install sequence covers storage bootstrap, auth sessions,
chat and invite state, users and user auxiliary state, dialogs, update
difference state, core message/read-outbox/hashtag tables, media metadata,
APIFull domain tables, KV state, and passkey ceremony/credential state,
plus cross-service compatibility constraints.
Service wiring remains gated until every production method in the owning
service uses the PostgreSQL path.

## Requirements

- PostgreSQL 18 server and `psql` client
- `DATABASE_URL`, for example:
  `postgresql://teamgram:teamgram@127.0.0.1:5432/teamgram?sslmode=disable`

For a clean local PostgreSQL 18 instance, start the Compose file. It starts
the database and runs the idempotent migration job after the health check:

```sh
docker compose -f docker-compose-postgres.yaml up -d
```

This file is separate from `docker-compose-env.yaml` so the database migration
job can be run independently of the full dependency stack. Historical
Teamgram/MySQL SQL and conversion helpers remain isolated under the migration
boundary; this file does not switch the default runtime or create a
dual-database production mode.

For APIFull, set the BFF `PostgresDSN` to the same PostgreSQL URL. The domain
opens one PostgreSQL 18 store per process. A PostgreSQL DSN is required for
production; MySQL is not a supported runtime database.

## Apply

```sh
DATABASE_URL='postgresql://teamgram:teamgram@127.0.0.1:5432/teamgram?sslmode=disable' \
  ./teamgramd/deploy/postgres/apply.sh
```

The `postgres-migrate` Compose service runs the same command inside the
PostgreSQL 18 image, so a host `psql` installation is not required.

`apply.sh` first verifies that the server reports PostgreSQL 18, then creates
`schema_migrations` and applies numbered SQL files from
`deploy/sql/postgres` in lexical
order, stores each file's SHA-256 checksum, and stops if an already-applied
migration was edited. Each migration must be safe to run in one transaction;
non-transactional operations require a separate, documented deployment step.

Migration version prefixes are unique. Gaps such as `024` and `026` are
reserved where earlier releases already consumed the surrounding versions;
new files must use a fresh prefix rather than reusing a number.

The runner rejects PostgreSQL 17, 19, and other server versions before any
schema mutation. A failed version check is a deployment configuration error;
do not bypass it by pointing the production runner at the legacy MySQL SQL
directory.

Run the syntax and portability guard before adding a migration:

```sh
./teamgramd/deploy/postgres/check.sh
```

The guard rejects MySQL-only engine, placeholder, auto-increment, lock, and
boolean syntax. It does not replace execution against a PostgreSQL 18
instance.

Both scripts reject a missing or empty migration directory. The runner checks
this before connecting or mutating a database, so a wrong deployment mount
cannot report a successful schema installation.

## Message delivery journal

Migration `015_message_random_id_per_user.sql` scopes nonzero message
`random_id` uniqueness to `(user_id, sender_user_id, random_id)`, allowing each
recipient to persist its own view of one message. Migration
`016_msg_inbox_delivery_outbox.sql` creates the durable recipient journal used
by Messenger's PostgreSQL single and batch sends. Sender message rows, update
records, and recipient intents commit together.

The worker claims pending intents with `FOR UPDATE SKIP LOCKED`, a UUID token,
and a 60-second lease. Earlier pending rows block later rows for the same
recipient. Kafka publication keeps an intent pending and schedules another
attempt after 30 seconds until the inbox consumer confirms message persistence
and successful update publication. Failed publications retry with exponential
delay capped at five minutes; expired leases recover interrupted workers.
Token checks prevent a stale worker from clearing a replacement claim.
Completion retains the deduplication keys and timestamps and clears the
duplicate payload.

`MESSENGER_POSTGRES_DSN` must point to an isolated PostgreSQL 18 database with
migrations `000` through `016` applied. The journal tests create and clean their
own schema and copy the journal table from the applied migration:

```sh
MESSENGER_POSTGRES_DSN='postgresql://teamgram:teamgram@127.0.0.1:55433/teamgram?sslmode=disable' \
  go test -race ./app/messenger/msg/internal/dao -run '^TestInboxDeliveryPostgres' -count=1
```

On 2026-10-08 the isolated PostgreSQL 18 journal checks passed for rollback,
deduplication, exact large integer payloads, unacknowledged resends, publication
failures, recipient order, concurrent claims, lease recovery, stale token
rejection, acknowledgement races, and worker cancellation. This evidence does
not establish a complete Layer 229 session or production acceptance.

The Kafka adapter in `pkg/mqproducer` bounds each caller's wait to 30 seconds
and each producer's outstanding records to 64. An accepted record retains its
slot after caller cancellation until the broker returns its final result;
background draining handles late acknowledgements. The worker retains durable
intents until consumer acknowledgement, and ambiguous cancellation retains its
lease. Shutdown stops admission, cancels the worker and drains the owned
producers. PostgreSQL message, update, and `pts` rows commit together.

The disposable PostgreSQL 18 and Layer 229 session recovery commands, acceptance
limits and evidence are documented in
[`docs/communication-reliability.md`](../../../docs/communication-reliability.md).
They exercise local dual-account delivery, fresh sessions, backend recreation,
Kafka stop/restart, physical backup, PITR and fenced manual promotion. Local
results do not establish production availability or throughput.

## Design rules

- Use PostgreSQL numbered placeholders (`$1`, `$2`, ...) in Go queries.
- Prefer `GENERATED BY DEFAULT AS IDENTITY`, `ON CONFLICT`, `RETURNING`,
  `jsonb`, `boolean`, and explicit `timestamptz` semantics where the owning
  service requires them.
- Give every idempotency key and update-state uniqueness rule an explicit
  constraint. Keep message ordering and `pts`, `qts`, `seq`, and `date` state
  in the same transaction as the authoritative mutation.
- Keep migrations additive and explicit. Do not emulate MySQL collations or
  `GET_LOCK`; use PostgreSQL constraints, row locks, or advisory locks with a
  documented key scope.

## Business control plane

Migration `077_apifull_business_control_plane.sql` stores explicitly published
Layer 229 Bot, Inline and WebView results. APIFull never creates a result from
an incoming RPC: a row must be published by an operations job, must be in the
`published` state, and must not be expired. The API still validates the exact
TL constructor after decoding the JSON. A caller-specific row takes precedence
over a global row (`caller_user_id = 0`).

Publish one result with the deployment helper (the result file must contain a
canonical JSON object or array):

```sh
DATABASE_URL="$DATABASE_URL" \
BUSINESS_OPERATION='bots.getPopularAppBots' \
BUSINESS_SCOPE_KEY='offset::limit:20' \
BUSINESS_RESULT_FILE=/run/teamgram/business/popular-bots.json \
./teamgramd/deploy/postgres/publish-business-control-plane.sh
```

The scope key is part of the business contract. Current handlers use keys such
as `bot:<id>`, `bot:<id>:id:<app_id>`, `query:<query_id>`,
`peer:<type>:<id>:msg:<message_id>`, and operation-specific SHA-256 keys. Do
not publish placeholder or empty constructors; missing or revoked rows remain
fail-closed.

## Provider control planes

Migration `078_apifull_payment_provider_control_plane.sql` is owned by the
signed payment provider. It records operation payload hashes, receipts and
granted entitlements under an `(operation, user_id, request_key)` idempotency
key. The provider must require PostgreSQL 18 and all three tables at startup;
reusing a request key with a different payload is a conflict.

Migration `079_provider_control_plane.sql` stores the explicitly provisioned
business live sessions, Fragment collectibles, ephemeral callback answers and
CDN objects used by the isolated provider services. Run
`docker/production/seed-provider-fixtures.sh` only for an isolated acceptance
environment, with every identity, response, and CDN public key supplied
explicitly. The helper has no sample identities or response defaults; omit
`LIVE_*` when a stories live session should remain unprovisioned.

For a complete isolated stack, combine the base Compose file with the
acceptance overlay. The overlay's `provider-fixtures` profile wires the backend
to the three signed fixtures on the internal network and adds PG-backed
healthchecks. It is deliberately separate from the production backend
overlay, because fixtures are not payment, business, or CDN authorities:

```sh
export TEAMGRAM_POSTGRES_DSN='postgresql://teamgram:teamgram@postgres:5432/teamgram?sslmode=disable'
export TEAMGRAM_PAYMENT_PROVIDER_KEY='acceptance-payment-api-key'
export TEAMGRAM_PAYMENT_PROVIDER_SIGNING_KEY='acceptance-payment-signing-key-at-least-32-bytes'
export TEAMGRAM_BUSINESS_PROVIDER_KEY='acceptance-business-api-key'
export TEAMGRAM_BUSINESS_PROVIDER_SIGNING_KEY='acceptance-business-signing-key-at-least-32-bytes'
export TEAMGRAM_CDN_PROVIDER_KEY='acceptance-cdn-api-key'
export TEAMGRAM_CDN_PROVIDER_SIGNING_KEY='acceptance-cdn-signing-key-at-least-32-bytes'
```

The values above are deployment placeholders, not credentials to commit. Seed
an explicit acceptance dataset before starting the fixture/backend services;
the seed job is intentionally a manual preflight so an empty dataset cannot be
mistaken for a healthy provider:

```sh
CALLBACK_USER_ID=42 CALLBACK_MESSAGE_ID=1 CALLBACK_DATA='probe-callback' \
CALLBACK_ANSWER_JSON='{"predicate_name":"messages_botCallbackAnswer","cache_time":30,"alert":true,"message":"accepted"}' \
COLLECTIBLE_USERNAME='probe_collectible' COLLECTIBLE_PURCHASE_DATE=1760000000 \
COLLECTIBLE_CURRENCY=USD COLLECTIBLE_AMOUNT=100 \
COLLECTIBLE_URL='https://provider.example/collectible/probe_collectible' \
CDN_PUBLIC_KEYS_JSON='[{"predicate_name":"cdnPublicKey","dc_id":2,"public_key":"probe-public-key"}]' \
docker compose -f docker-compose.yaml -f docker/production/provider-fixtures.acceptance.yaml \
  --profile provider-fixtures run --rm provider-fixtures-seed
```

After the seed job exits successfully, start the fixture/backend services:

```sh
docker compose -f docker-compose.yaml -f docker/production/provider-fixtures.acceptance.yaml \
  --profile provider-fixtures up -d
```

Add `LIVE_USER_ID`, `LIVE_RANDOM_ID`, `LIVE_REQUEST_PAYLOAD`, and
`LIVE_UPDATES_JSON` together when a `stories.startLive` row is needed. Missing
provider rows, expired rows, unavailable services, and an empty CDN key list
remain errors. These fixtures are not application fallbacks or production
payment/CDN authorities.
