# Contributing

This document covers branching, PR rules, code style, commit messages, and local dev/test. The root [CONTRIBUTING.md](../CONTRIBUTING.md) is a short entry point.

## Branching

- **Default branch**: `master`. Changes should land via Pull Request, not direct push.
- Work in a fork: create a feature branch from the latest default branch, then open a PR.

## Pull requests

- **Title**: Clear and concise; reference issues (e.g. `Fix #123`) when relevant.
- **CI**: PRs should pass build and tests when CI is enabled.
- **Review**: At least one maintainer review when applicable.
- **Scope**: One logical change per PR when possible.

## Code style

- **Go**: Use `gofmt`; prefer `golangci-lint` if the project uses it.
- Follow any project-specific rules (e.g. generated code, dalgen).

## Commit messages

- Prefer **Conventional Commits** (e.g. `feat: add X`, `fix: Y`, `docs: Z`).
- Keep messages concise for CHANGELOG and history.

## Local dev and test

### Build

- **Go**: 1.25+.
- From repo root: `make`; binaries go to `teamgramd/bin/`.

### Dependencies

- PostgreSQL 18 is the sole production database. Legacy generated DAOs remain outside the production runtime; production configs accept only a PostgreSQL DSN. Redis, etcd, Kafka, SeaweedFS S3 (and FFmpeg if needed) are also required. Optional: `docker compose -f docker-compose-env.yaml up -d`.
- **Database**: Create DB `teamgram`, then apply the PostgreSQL 18 migrations with
  `DATABASE_URL='postgresql://teamgram:teamgram@127.0.0.1:5432/teamgram?sslmode=disable' ./teamgramd/deploy/postgres/apply.sh`.
  The SQL files directly under `teamgramd/deploy/sql/` are a historical Teamgram
  schema archive and are not deployment inputs.
- **SeaweedFS S3**: Buckets `documents`, `encryptedfiles`, `photos`, `videos` (created by the `mini` startup configuration when using docker-compose-env).

### Run services

- `cd teamgramd/bin && ./runall2.sh` (or `./runall-docker.sh` in Docker).

### Tests

- From repo root: `go test ./...`. Use `go test ./pkg/...` if some tests need no external deps.

### Code generation

- After changing proto or dal schema, re-run the project’s codegen (e.g. dalgen scripts).
