# Partial API evidence harness

`tools/partial_probe_generator.py` turns the partial rows in
`LAYER229_METHOD_LEDGER.csv` and the focused matrix into a reproducible review
plan. Generation does not connect to a service, restart a container, or change
the ledger.

```sh
cd teamgram-server
python3 tools/partial_probe_generator.py \
  --output-dir /tmp/teamgram-partial-probes \
  --json
bash -n /tmp/teamgram-partial-probes/run-session-probes.sh
bash -n /tmp/teamgram-partial-probes/run-sql-evidence.sh
```

The generated `manifest.json` records the source file hashes and the evidence
each partial method still needs. `session-probes.json` maps methods to matrix
rows. A row is `METHOD_PROBE_AVAILABLE` only when the matrix entry explicitly
declares the exact TL method using `method`, `methods`, `tl_method`,
`tl_methods`, or `covers`. Existing category rows are marked
`CATEGORY_CANDIDATE_ONLY`; they are useful for planning but do not prove any
individual method. Methods without a category candidate are marked
`MANUAL_SESSION_PROBE_REQUIRED`. Restart rows are similarly separated:
`CATEGORY_RESTART_PLAN_ONLY` means the category has a manual restart plan but
no executable restart probe; it is not restart evidence.

Run the source consistency check before generating artifacts:

```sh
python3 tools/partial_probe_generator.py --check --json
```

`run-session-probes.sh` requires `RUN_PARTIAL_PROBES=1`. It executes only
method-exact rows by default. Running broad category candidates requires the
additional explicit acknowledgement `RUN_PARTIAL_CATEGORY_PROBES=1`.

## Restart barrier

`tools/restart_probe.py` is a guarded backend restart barrier. It is a dry run
unless `--execute` is supplied. `--execute` requires both a before command and
an after command, so a backend cannot be restarted accidentally without a
business assertion. `--allow-barrier-only` is an explicit exception and
reports `BARRIER_ONLY` with exit status 2. The result becomes `PASS` only when
both assertions succeed; an optional cleanup command is always attempted
after execution. Layered Compose files are supported by repeating
`--compose-file`; use `--project-name backend --project-directory
/Users/mac/open/tg/services/backend` (or the corresponding
`ACCEPTANCE_COMPOSE_*` variables) to target the existing Compose project.

The generated `run-restart-probe.sh` adds another explicit guard. Set
`RUN_RESTART_PROBE=1`, provide `RESTART_BEFORE_COMMAND` and
`RESTART_AFTER_COMMAND`, then pass the Compose file and service. It forwards
`--execute` only after those assertions are present.

```sh
python3 tools/restart_probe.py \
  --compose-file "$ACCEPTANCE_COMPOSE_FILE" \
  --service "$ACCEPTANCE_SERVICE" \
  --before-command 'python3 tools/your_tagged_setup.py' \
  --after-command 'python3 tools/your_tagged_reconnect_assertion.py' \
  --cleanup-command 'python3 tools/your_tagged_cleanup.py' \
  --sql-file /tmp/teamgram-partial-probes/sql-evidence.sql \
  --output /tmp/teamgram-restart-evidence.json \
  --execute
```

The generated wrapper is equivalent to the direct command:

```sh
RUN_RESTART_PROBE=1 \
RESTART_BEFORE_COMMAND='python3 tools/your_tagged_setup.py' \
RESTART_AFTER_COMMAND='python3 tools/your_tagged_reconnect_assertion.py' \
/tmp/teamgram-partial-probes/run-restart-probe.sh \
  "$ACCEPTANCE_COMPOSE_FILE" "$ACCEPTANCE_SERVICE"
```

Set `RESTART_DRY_RUN=1` with the wrapper to write a plan without invoking
Compose. The wrapper still requires `RUN_RESTART_PROBE=1` and both business
assertion commands.

The evidence includes Compose config, pre/post service state, restart-barrier
polls, command exit codes, output hashes, and (when configured) pre/post
read-only SQL captures. DSNs are read from `ACCEPTANCE_DATABASE_URL` or
standard `PG*` variables and are redacted from the JSON artifact.

## SQL evidence

`tools/sql_evidence.py` executes a supplied SQL file with `psql` in
`ON_ERROR_STOP` mode and writes the SQL hash, result, and captured output. It
first rejects psql meta-commands and data/schema mutation statements and
requires an explicit read-only transaction. It refuses to run when no
PostgreSQL connection environment is configured. For a database available only
inside Compose, stream the query through a container without exposing a DSN:

```sh
python3 tools/sql_evidence.py \
  --sql-file /tmp/teamgram-partial-probes/sql-evidence.sql \
  --output /tmp/teamgram-partial-sql.json \
  --docker-container teamgram-postgres \
  --docker-user teamgram --docker-database teamgram
```

The generated SQL uses `SET TRANSACTION READ ONLY`; its table row estimates
are metadata evidence only and do not replace a method-specific keyed
assertion. `read_only_guard.status` and the tracked row-count check must both
be `PASS` before treating the SQL artifact as valid evidence.

```sh
RUN_SQL_EVIDENCE=1 /tmp/teamgram-partial-probes/run-sql-evidence.sh
```

Generated artifacts are evidence inputs. They do not automatically promote a
`PARTIAL_*` ledger row, and a matrix `PASS` still requires reviewer validation
against the corresponding session, provider, restart, fan-out, and cleanup
gates.
