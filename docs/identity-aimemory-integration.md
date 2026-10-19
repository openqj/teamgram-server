# Teamgram to Identity and AIMemory

Teamgram remains the MTProto communication service. Product accounts are
owned by `services/identity`, and AIMemory accepts only short-lived Identity
JWTs. Teamgram does not sign product tokens and does not send an MTProto auth
key to either service.

## Enable the bridge

Set these environment variables for the `interface.httpserver` process:

```text
TEAMGRAM_IDENTITY_BASE_URL=http://identity:8090
TEAMGRAM_IDENTITY_TEAMGRAM_SERVICE_TOKEN=<same value as IDENTITY_TEAMGRAM_SERVICE_TOKEN>
TEAMGRAM_IDENTITY_INSTANCE_ID=default
TEAMGRAM_IDENTITY_REPLAY_REDIS_HOST=redis:6379
TEAMGRAM_IDENTITY_REPLAY_REDIS_PASS=
TEAMGRAM_IDENTITY_REPLAY_REDIS_TLS=false
TEAMGRAM_IDENTITY_REPLAY_TTL_SECONDS=60
TEAMGRAM_IDENTITY_REQUEST_TIMEOUT_SECONDS=5
TEAMGRAM_CORS_ORIGINS=https://memory.example.com
```

`identity:8090` is valid only when the Teamgram and Identity containers share
the same Docker network. With the standalone Compose files on Docker Desktop,
use a routable address such as `http://host.docker.internal:28082`, or attach
both stacks to one external network before using the service name.

When WebA calls the token exchange from a different origin, set
`TEAMGRAM_CORS_ORIGINS` to the exact WebA origin(s). This enables the HTTP
gateway's preflight response for the proof request; leaving it empty keeps the
gateway same-origin only.

The `AuthsessionClient` configuration must resolve the existing
`service.authsession` RPC service. The bridge is disabled when all Identity
values are empty and fails startup when only part of the required configuration
is supplied. When enabled, the shared Redis replay store is required; it
prevents a load-balancer retry from exchanging the same encrypted ping twice.

## Provision a binding

An operator provisions the mapping once through Identity's internal API. The
account ID is the canonical `acct:<UUID>` value returned by Identity login.

```http
POST /v1/internal/teamgram/bind
X-Identity-Internal-Token: <internal service token>
Content-Type: application/json

{"teamgram_user_id":12345,"account_id":"acct:550e8400-e29b-41d4-a716-446655440000"}
```

Token exchange never creates or changes this mapping.

## Exchange a session proof

The client posts the raw encrypted MTProto transport payload containing a
normal `ping` request to the Teamgram gateway:

```http
POST /v1/internal/aimemory/token
Content-Type: application/octet-stream

<encrypted MTProto payload>
```

Teamgram validates the auth key and decrypted TL constructor, resolves the
server-side auth session user through `service.authsession`, and calls
Identity's `/v1/internal/teamgram/token`. A successful response is:

```json
{
  "access_token": "<short-lived Identity JWT>",
  "token_type": "Bearer",
  "expires_at": 1760000900
}
```

The caller uses that token with AIMemory's normal `Authorization: Bearer`
header. Invalid proof returns `401`, an unbound Teamgram account returns `404`,
and an unavailable Identity or authsession dependency returns `503`.
