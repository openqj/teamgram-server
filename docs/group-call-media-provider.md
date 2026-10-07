# Group Call Media Provider

The APIFull BFF delegates live-stream channel discovery and RTMP credential
issuance to one configured media-control endpoint. The provider owns the
streaming session and relay credentials; APIFull only authorizes callers and
validates the provider response.

## Configuration

Configure these values on the BFF process. Production endpoints must use
HTTPS. HTTP is accepted only for loopback development endpoints.

| Setting | Environment variable | Requirement |
| --- | --- | --- |
| Endpoint | `TEAMGRAM_GROUP_CALL_MEDIA_ENDPOINT` | HTTPS URL, no user info, query, or fragment |
| API key | `TEAMGRAM_GROUP_CALL_MEDIA_API_KEY` | Sent as a Bearer token |
| Signing key | `TEAMGRAM_GROUP_CALL_MEDIA_SIGNING_KEY` | At least 32 bytes; shared for request and response HMAC |
| RTMP host | `TEAMGRAM_GROUP_CALL_MEDIA_RTMP_HOST` | Hostname or host:port allowed in returned RTMP URLs |
| Timeout | `TEAMGRAM_GROUP_CALL_MEDIA_TIMEOUT_SECONDS` | Integer from 0 to 300; zero uses 5 seconds |

Do not put live credentials in checked-in YAML. Missing configuration keeps
the methods unavailable.

## HTTP Contract

APIFull sends `POST` with `Content-Type: application/json` and
`Authorization: Bearer <api-key>`. The request signature is the lowercase
hexadecimal HMAC-SHA256 of the exact request body bytes:

```text
X-Teamgram-Group-Call-Request-Signature: sha256=<hex>
```

The provider must return a 2xx status, a JSON body no larger than 1 MiB, and
`X-Teamgram-Group-Call-Signature` computed over the exact response body with
the same signing key. Redirects are rejected. APIFull checks the HMAC before
decoding the response.

### Stream channels

Request body:

```json
{
  "operation": "get_stream_channels",
  "user_id": 123,
  "call_id": 456,
  "channel_id": 789,
  "revoke": false
}
```

Response body:

```json
{
  "verified": true,
  "user_id": 123,
  "call_id": 456,
  "channel_id": 789,
  "channels": [
    {"channel": 0, "scale": 0, "last_timestamp_ms": 1700000000000}
  ]
}
```

The identity fields must exactly match the request. `channels` must be present
and contain at most 32 entries. Channel, scale, and timestamp values must be
nonnegative, and each `(channel, scale)` pair must be unique.

### RTMP credentials

Request body uses `operation: "get_rtmp_url"` and the same identity fields.
`revoke` mirrors the MTProto request. For issuance, APIFull sends an
`Idempotency-Key` stable for the call and user; the provider must return the
same active credential for retries with that key. A revoke request omits the
idempotency header.

The response must include the matching identity fields and `verified: true`,
plus `url` and `key`. The URL scheme must be `rtmp` or `rtmps`, its host must
match the configured RTMP host, and it must have a path without query or
fragment. The key must be nonempty, at most 1024 bytes, and contain no control
characters. Return URL and key fields for revoke responses as well; the BFF
uses the same typed MTProto result for both operations.

## Failure Behavior

Provider transport, status, signature, JSON, identity, and field-validation
failures return an error without fabricating media state. The BFF only calls
these operations for RTMP-enabled group calls and enforces channel membership
and `ManageCall` authorization before RTMP credential operations.

This contract describes component wiring. A configured relay, provider
credentials, real RTMP session, TURN/WebRTC negotiation, and production
acceptance are separate deployment requirements.
