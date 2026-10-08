# Protocol and compatibility

This document describes the MTProto sub-protocols, API Layer, and client compatibility for Teamgram Server.

## MTProto 2.0

Teamgram implements [MTProto 2.0](https://core.telegram.org/mtproto) with these transports:

- **Abridged**
- **Intermediate**
- **Padded intermediate**
- **Full**

Clients connect to gnetway over TCP or WebSocket and use one of the above for handshake and RPC.

## Protocol boundary and API Layer

- Public backend feature methods mean the **MTProto User API**. Internal gRPC, DAL, and storage contracts may use project-specific conventions, but public adapters must preserve Telegram semantics. Bot API and TDLib are out of scope unless a method is explicitly marked otherwise.
- The current supported **API Layer** is **229**.
- The TL wire contract follows the official Telegram TL definitions and the repository schema [`telegram-api/schema/telegram_api_layer229.tl`](../../telegram-api/schema/telegram_api_layer229.tl). Generated `proto/mtproto` types, handwritten Layer 229 codec additions, and `telegram-api` method definitions must remain consistent with it.
- Standard methods must preserve constructor IDs, method names, parameter order, flags, optional fields, vector ordering, and result constructors. An API Layer upgrade must update the schema, generated code, client compatibility notes, and tests together; layers must not be mixed silently.
- Method behavior must match Telegram client expectations for permissions, entity IDs and `access_hash`, pagination, timestamps, idempotency, canonical RPC errors, and `pts`, `qts`, `seq`, and `date` update state.

## Client workflow completion and acceptance evidence

- [LAYER229_METHOD_LEDGER.csv](../LAYER229_METHOD_LEDGER.csv) is the sole source of method status, `audit_status`, `production_acceptance_status`, and evidence. [Functional acceptance notes](../LAYER229_FUNCTIONAL_AUDIT.md) describe scenarios without creating a second status ledger.
- Use existing evidence fields or their linked acceptance notes to map client/version and feature → Layer 229 methods → permissions and entity identity → authoritative persistence → update synchronization and difference recovery → scenarios/results. Track every required method and external dependency when a feature spans several methods.
- Complete required chat, channel/megagroup, forum, secret-chat, search, notification, scheduled-message, media, and call workflows separately. Scope follows the actual needs of supported clients; Bot API and TDLib scope remains as defined above.
- Cover flags/optional fields, valid results and errors, denied permissions, method-specific idempotency, multi-device updates, and restart recovery. Correct TL types or valid empty results demonstrate only their respective branches, not complete feature readiness. Do not report success for unimplemented business behavior or missing required providers.
- Verify reconnects, reordered/duplicate updates, edits/deletes, and membership permission changes according to [Architecture](architecture.md). Calls also need real bidirectional media evidence, and files need standard file-path and metadata/content consistency evidence. Keep unverified items at their actual unaccepted status.

## Compatible clients

| Client | Notes |
|--------|--------|
| [Android (teamgram-android)](../clients/teamgram-android.md) | Patch server address/port in ConnectionsManager.cpp |
| [iOS (teamgram-ios)](../clients/teamgram-ios.md) | Configure to point to your server |
| [Desktop (teamgram-tdesktop)](../clients/teamgram-tdesktop.md) | Configure to point to your server |

**Important:** Default sign-in verification code is **12345** (development only; change for production).

## Deployment notes

- Clients must reach gnetway ports (default 10443, 11443, 5222). Adjust TLS or reverse proxy (e.g. Nginx for WebSocket) as needed.
