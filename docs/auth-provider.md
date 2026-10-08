# Authorization Provider

The authorization BFF delegates API credential validation and web-token
verification to one configured provider. A successful provider response is
not enough to create a session: bot imports also resolve the supplied bot
token through the User service, and every import binds the current auth key
through authsession.

## Configuration

| Setting | Environment variable | Requirement |
| --- | --- | --- |
| Endpoint | `TEAMGRAM_AUTH_PROVIDER_ENDPOINT` | HTTPS URL without user info, query, or fragment |
| API key | `TEAMGRAM_AUTH_PROVIDER_KEY` | Sent as a Bearer token |
| Signing key | `TEAMGRAM_AUTH_PROVIDER_SIGNING_KEY` | At least 32 bytes; shared request/response HMAC key |
| Timeout | `TEAMGRAM_AUTH_PROVIDER_TIMEOUT_SECONDS` | Integer from 0 to 300; zero uses 5 seconds |

HTTP is accepted only for loopback development endpoints. Empty configuration
keeps both import methods unavailable.

## Request and response

The BFF sends `POST` JSON with an HMAC-SHA256 signature over the exact request
body. Each request contains a fresh 256-bit `request_id` nonce. The signed
response must echo the exact nonce, preventing a previous signed result from
being replayed for a later token.

```text
X-Teamgram-Auth-Provider-Request-Signature: sha256=<hex>
```

The provider must return a 2xx response no larger than 1 MiB and sign the exact
response body with:

```text
X-Teamgram-Auth-Provider-Signature: sha256=<hex>
```

Bot import request:

```json
{
  "request_id": "<fresh 256-bit nonce>",
  "operation": "verify_bot_authorization",
  "api_id": 123,
  "api_hash": "...",
  "bot_auth_token": "123:..."
}
```

Web-token import uses `operation: "verify_web_token_authorization"` and sends
`web_auth_token` instead. The response must contain:

```json
{
  "request_id": "<the request nonce>",
  "verified": true,
  "operation": "verify_bot_authorization",
  "api_id": 123,
  "user_id": 456
}
```

The operation, API ID, and positive user ID must match the request. For a bot
import, the returned user ID must also match the active Bot registry record
resolved by the User service from the supplied token. Provider failures,
signature failures, malformed identity, deleted users, and authsession bind
failures never produce an authorization response.
