# Provider and Delivery Contract

Provider boundaries use the same authenticated transport rules. This contract
describes Teamgram's integration boundary, not who owns the authoritative
service behind it; that service may be first-party or an enabled third-party
upstream. See [Provider ownership and readiness](provider-ownership-and-readiness-zh.md)
for the per-capability ownership and evidence status. The operation-specific
result remains opaque and must still be decoded and validated against its
Layer 229 constructor by the caller.

## Request

```json
{
  "request_id": "per-attempt-request-id",
  "operation": "namespace.method",
  "user_id": 42,
  "payload": {}
}
```

The exact request body is signed with HMAC-SHA256 using the configured signing
key. The hexadecimal digest is sent as `sha256=<digest>` in the provider
specific request-signature header. `Authorization: Bearer <provider-key>` is
optional where the provider has a separate bearer key. `Idempotency-Key` is a
stable SHA-256 digest of `user_id`, `operation`, and the exact payload bytes.
Retries with the same key and payload must return the original result; a
different payload under that key is a conflict.

## Response

The response body is signed with the provider-specific response-signature
header and has this envelope:

```json
{
  "request_id": "same-as-request",
  "operation": "same-as-request",
  "verified": true,
  "user_id": 42,
  "result": {}
}
```

Callers must verify the signature before decoding JSON, then require all three
identity fields to match. An `error`, missing result, invalid signature,
redirect, non-2xx status, or malformed result is a provider failure and must
map to the method's canonical MTProto error. Provider responses never create
implicit local success.

The shared implementation is `pkg/providercontract`; business and payment
clients use it for signatures, idempotency keys, and request headers.

## Durable delivery intents

Outbox rows use the same state machine:

```text
pending -> inflight -> complete
                    \-> pending (retry)
complete -> complete (idempotent acknowledgement)
```

Claiming a row increments `attempts`, stores a lease and a claim token, and
commits before delivery. Completion or retry must include the current claim
token. Expired leases are reclaimable after a process restart. Each business
mutation and its delivery intent are committed in one PostgreSQL transaction;
the reconciler is therefore allowed to delay delivery but cannot lose it.

The shared state transition checks live in `pkg/providercontract` and are
used by the Bot payment outbox scanner. Layer-specific outboxes may add event
types, ownership checks, and payload validation, but must retain these state
and lease rules.
