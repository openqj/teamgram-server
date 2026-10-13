# Verification-code fixture

The code-provider fixture is an isolated acceptance dependency. It accepts the
HTTP provider `Delivery` payload for both SMS and email, keeps the latest
delivery in memory, and exposes it through an authenticated `GET /latest` so a
test can complete `auth.sendCode` and `auth.signIn` without claiming that a
real carrier or email provider was exercised.

Production files do not start this service. Leaving
`TEAMGRAM_CODE_SMS_PROVIDER`, `TEAMGRAM_CODE_SEND_URL`, and the challenge
secret empty therefore keeps the backend's fail-closed
`SMS_CODE_CREATE_FAILED` behavior.

## Build the fixture image

```sh
docker build \
  -t teamgram-code-provider-fixture:acceptance \
  -f /Users/mac/open/test/teamgram-server/docker/production/code-provider-fixture/Dockerfile \
  /Users/mac/open/test/teamgram-server/docker/production/code-provider-fixture
```

## E2E Compose settings

Set these values when applying
`docker/production/backend-e2e-provider-fixtures.yaml`:

```sh
export TEAMGRAM_CODE_PROVIDER_KEY=code-fixture-key
export TEAMGRAM_CODE_PROVIDER_SECRET=code-fixture-secret
export TEAMGRAM_CODE_CHALLENGE_SECRET=0123456789abcdef0123456789abcdef
```

The overlay points both SMS and email delivery to
`http://code-provider-fixture:23193/send`. It must be the last backend
environment overlay when another file uses Compose `!override` for the
backend environment (for example, `/tmp/teamgram-production-runtime.yaml`):

```sh
docker compose \
  -f /Users/mac/open/tg/services/backend/docker-compose.yaml \
  -f /Users/mac/open/test/teamgram-server/docker/production/backend-deploy-r29-prod.yaml \
  -f /tmp/teamgram-production-runtime.yaml \
  -f /Users/mac/open/test/teamgram-server/docker/production/backend-e2e-provider-fixtures.yaml \
  up -d backend code-provider-fixture
```

The verification response is keyed by the challenge returned from
`auth.sendCode`:

```sh
curl -H "Authorization: Bearer $TEAMGRAM_CODE_PROVIDER_KEY" \
  -H "X-Provider-Secret: $TEAMGRAM_CODE_PROVIDER_SECRET" \
  'http://127.0.0.1:23193/latest?challenge_id=<phone_code_hash>'
```

The fixture is in-memory and scoped to the container lifetime. A restarted
fixture invalidates no backend challenge; the backend will simply deliver a
new code on the next send attempt.
