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

This file is separate from `docker-compose-env.yaml` because application
services still contain transitional Teamgram MySQL paths during the port. It
does not switch the default runtime or create a dual-database production mode.

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

The existing Kafka adapter does not cancel its underlying synchronous producer
call when its context expires. The worker bounds its wait to 30 seconds and
allows only one in-flight publication per DAO; shutdown cancels the worker and
closes the owning producer. PostgreSQL message, update, and `pts` counter rows
commit together for the PostgreSQL runtime. Downstream Sync consumption,
ordering between concurrent sender transactions, and complete
restart/fault-injection session checks remain acceptance work.

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
