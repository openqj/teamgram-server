# Release and changelog

This document defines versioning, CHANGELOG format, and the release checklist.

## Versioning

- **Semantic versioning** in spirit; example format: `v0.96.0-teamgram-server`.
  - **Major**: Incompatible API or architecture changes.
  - **Minor**: Backward-compatible new features.
  - **Patch**: Backward-compatible fixes and small improvements.
- The suffix (e.g. `-teamgram-server`) may be used to distinguish from dependencies (e.g. proto).

## CHANGELOG

- **Location**: Root **CHANGELOG.md** (when present).
- **Format**: Prefer [Keep a Changelog](https://keepachangelog.com/) (sections: Added, Changed, Deprecated, Removed, Fixed, Security).
- **Responsibility**: Update CHANGELOG for each release with the changes for that version.

## Production acceptance

Before production launch or a critical-path change, record the following evidence for the release. Functional status comes from the sole [Layer 229 method ledger](../LAYER229_METHOD_LEDGER.csv), performance targets and measurement boundaries from [Architecture](architecture.md), and phase dependencies from [Roadmap](roadmap.md).

1. **Functionality and protocol**: Required supported-client methods reach the ledger's production acceptance status for TL contracts, permissions, idempotency, business outcomes, persistence, and multi-device updates. A single-session success, correct constructor, or compilation does not establish complete feature readiness.
2. **Database switch**: Before making PostgreSQL the default runtime, schema, DAOs, raw SQL, initialization/migrations, all production services, and Layer 229 probes pass on PostgreSQL 18. Define transaction boundaries for persistence, updates, and Outbox; avoid a permanent dual-database production path.
3. **Failures and recovery**: Verify reconnect difference recovery, delivery retries, duplicate events, process exits, worker interruptions, permission changes, and primary failure. Record recovery of acknowledged messages/updates, backup restoration exercises, and corresponding RPO/RTO under explicit failure models. An asynchronous replica does not establish zero data loss after primary failure.
4. **Performance and capacity**: Fix online users, message/broadcast rates, group/channel sizes, concurrent media, history volume, warm/cold caches, geography/RTT, hardware, and replication configuration. Record scenario-specific P95/P99, throughput, CPU/memory, database lock waits, queue lag, and recovery catch-up time. Validate the initial 50/300/100 ms budgets under the selected load; do not publish them as achieved performance or official Telegram metrics.
5. **Media and external dependencies**: Configure and accept required real authorization, push notifications, object storage, and other providers. In-scope calls require client-compatible signaling, TURN/relays, SFU behavior, bidirectional audio/video, and reconnect evidence. Verify resource release for background media work and after call termination.
6. **Deployment and rollback**: Verify recovery after session-node exit, network boundaries for internal RPC/infrastructure, monitoring/alerts, and release recovery. Module-level independence changes need compatibility and performance evidence plus a rollback or compatible migration plan.

These are acceptance requirements. Recording them does not automatically promote method status or establish that the current release has completed the PostgreSQL port or met the performance budgets.

## Release checklist

Before cutting a new release:

1. **Version**: Update `VERSION` in Makefile or version injection if used.
2. **CHANGELOG**: Add an entry for the new version and summarize changes.
3. **Tag**: Create a Git tag (e.g. `v0.96.0`) and push.
4. **Artifacts**: Build and publish binaries or Docker images per project practice.
5. **README**: Update any version or “current release” references if needed.
