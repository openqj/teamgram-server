# Roadmap

This document outlines short- and medium-term goals and the boundary between community and enterprise editions.

## Project position and evolution

Teamgram is the community open-source foundation currently used by this project. During this phase, the project builds on the Teamgram base implementation and MTProto stack while filling the backend capabilities and Layer 229 methods required by the supported clients and by production deployment.

While the transition is in progress, Teamgram's protocol implementation, TL definitions, and compatibility behavior form the basis of the public MTProto boundary. New and repaired functionality must preserve client compatibility; do not remove required dependencies or break the wire contract in order to decouple early.

Independent implementation and gradual removal of Teamgram dependencies begin only after the required client features and methods pass compatibility and production-readiness acceptance. The decoupling phase must preserve the public MTProto contract and provide a migration path.

## Delivery order and phase exit criteria

Follow the incremental approach in [Architecture](architecture.md). The order below describes acceptance dependencies; feature completion and database work can overlap within explicit boundaries. Historical data transfer is unnecessary before launch, but PostgreSQL business semantics and recovery still require verification.

| Phase | Work | Exit criteria |
|-------|------|---------------|
| 1. Functional and performance baseline | Reuse the sole Layer 229 method ledger to connect client features with permissions, persistence, update recovery, and acceptance scenarios; define online users, message rate, group sizes, media load, geography/RTT, and hardware | Gaps and evidence are traceable; workload, measurement boundaries, and performance budgets are defined without presenting initial budgets as measured results |
| 2. PostgreSQL storage port | Establish the project-local PostgreSQL access layer; convert schema, DAOs, raw SQL, locks, idempotency, and initialization/migration scripts across every production service | Switch the default runtime only after services, transactions, update recovery, compatibility probes, and deployment checks pass on PostgreSQL 18; production access converges on PostgreSQL |
| 3. Complete client workflows | Complete required chat, channel, forum, secret-chat, search, notification, scheduled-message, call, and other supported-client features; associate methods with their actual dependencies | Evidence covers authorization, business outcomes, persistence, multi-device updates, and restart recovery; calls and external providers have real-client/integration acceptance, with remaining gaps recorded accurately |
| 4. Optimize critical paths | Measure serial RPCs, per-member delivery, and channel transaction writes; complete Outbox, batched delivery, and media isolation, then adjust deployment boundaries, indexes, and capacity from evidence | Comparable before/after results under the same workload; private-chat, group, channel, and media P95/P99, throughput, and lag meet selected targets without correctness regression |
| 5. Launch readiness | Verify reconnect catch-up, worker retries, process exit, primary failure, backup restoration, and resource release; configure required real authorization, push, media, and other integrations | Required methods reach the ledger's production acceptance status; RPO/RTO and capacity have evidence and [release acceptance](release-and-changelog.md) passes |
| 6. Gradual independence | Remove Teamgram dependencies module by module after the preceding acceptance gates; assess rewrites separately for modules unable to meet requirements | Preserve the Layer 229 public contract and client behavior; each replacement has compatibility, recovery, and performance evidence plus a rollback or compatible migration path |

Update phase status from actual evidence. Registered handlers, correct constructors, compilation, or database selection alone do not complete a phase.

### PostgreSQL port status (2026-10-08)

The first PostgreSQL 18 slices are implemented and verified. Production
services now require PostgreSQL configuration; any service whose handler
surface is still incomplete must fail startup rather than fall back to MySQL:

| Slice | Evidence | Runtime status |
|-------|----------|----------------|
| Storage foundation | `pgxpool`, transaction/error helpers, focused package tests | Available to new adapters |
| Authsession schema and DAO | Fresh schema, independent `pgx` DAO, `go test ./app/service/authsession/...` | Not wired until all authsession methods share the PostgreSQL path |
| Basic-group chat schema and DAO | Fresh `chats`/`chat_participants` schema, independent DAO, `go test ./app/service/biz/chat/...` | Not wired until invite and related chat aggregates are migrated |
| Update state schema and DAO | Fresh `user_pts_updates`/`auth_seq_updates` schema with unique sequence keys, independent `pgx` DAO, `go test ./app/service/biz/updates/...` | Not wired until update writers and recovery reads share the PostgreSQL transaction |
| Message aggregate schema and DAO | Fresh `messages`, read-outbox and hashtag schema, idempotent random-ID insert, independent `pgx` DAO, `go test ./app/service/biz/message/...` | Core DAO available; full message method surface and runtime wiring remain open |
| User auxiliary and media DAO slices | User privacy/presence/peer/settings DAOs plus fresh media document/photo-size schema and DAOs, focused package tests | Core persistence available; full method coverage and runtime wiring remain open |
| APIFull domain | PostgreSQL connector, CAS/advisory locks, 36-table runtime schema conversion, PostgreSQL 18 probe | Domain path available; production migration and full CRUD compatibility remain open |
| Deployment | Clean PostgreSQL 18 Compose file, checksum migration runner, portability check | PostgreSQL 18 is the production deployment baseline |

The remaining services (user, dialog, message, media, inbox, sync, and update
state) still need complete handler coverage before production acceptance. Their
generated MySQL DAO code is migration-boundary code only and must not be opened
by production configuration.

## Short-term

- **Docs and specs**: Keep specs (architecture, protocol, dependencies, contributing, security, release, roadmap) in sync with the codebase.
- **Database baseline**: PostgreSQL 18 is the sole production database for the independent implementation. The current Teamgram MySQL DAOs, SQL, and migrations are being converted; the project has not launched, so no historical data migration is required. Production services must fail startup until their PostgreSQL path is complete; do not retain a dual-database runtime.
- **CI and quality**: Add CI (e.g. GitHub Actions) for build, test, and lint (e.g. golangci-lint); add `test`, `lint`, `fmt` targets to Makefile where useful.
- **Doc consistency**: Align README and other docs (e.g. docker-compose-env.yaml naming, install guides).

## Medium-term

- **Test coverage**: Add unit and integration tests for critical paths; define coverage expectations over time.
- **Observability and ops**: Document runbooks and usage of the monitoring stack (Prometheus, Grafana, Jaeger, ELK).
- **Install and deploy**: Support more environments (e.g. other Linux distros, Kubernetes examples) and keep them consistent with docker-compose-env.

## Community vs enterprise

The following are available in the **enterprise edition** (contact the [author](https://t.me/benqi)); the community edition does not include or only partially supports them:

- sticker / theme / chat_theme / wallpaper / reactions / secret chat / 2FA / SMS / push (APNS / Web / FCM) / web / scheduled / autodelete / …
- channels / megagroups
- audio / video / group / conferenceCall
- bots
- miniapp

See the main README “Enterprise edition” section. The community edition currently prioritizes protocol completion, client compatibility, and production readiness; the enterprise boundary does not change that evolution goal.
