#!/usr/bin/env bash
set -Eeuo pipefail

: "${DATABASE_URL:?DATABASE_URL must be set}"

: "${CALLBACK_USER_ID:?CALLBACK_USER_ID must be set}"
: "${CALLBACK_MESSAGE_ID:?CALLBACK_MESSAGE_ID must be set}"
: "${CALLBACK_DATA:?CALLBACK_DATA must be set}"
: "${CALLBACK_ANSWER_JSON:?CALLBACK_ANSWER_JSON must be set to a canonical messages.botCallbackAnswer object}"
: "${COLLECTIBLE_USERNAME:?COLLECTIBLE_USERNAME must be set}"
: "${COLLECTIBLE_PURCHASE_DATE:?COLLECTIBLE_PURCHASE_DATE must be set}"
: "${COLLECTIBLE_CURRENCY:?COLLECTIBLE_CURRENCY must be set}"
: "${COLLECTIBLE_AMOUNT:?COLLECTIBLE_AMOUNT must be set}"
: "${COLLECTIBLE_URL:?COLLECTIBLE_URL must be set}"
: "${CDN_PUBLIC_KEYS_JSON:?CDN_PUBLIC_KEYS_JSON must be set to a non-empty JSON array}"

COLLECTIBLE_CRYPTO_CURRENCY=${COLLECTIBLE_CRYPTO_CURRENCY:-}
COLLECTIBLE_CRYPTO_AMOUNT=${COLLECTIBLE_CRYPTO_AMOUNT:-0}
collectible_crypto_currency=$COLLECTIBLE_CRYPTO_CURRENCY
collectible_crypto_amount=$COLLECTIBLE_CRYPTO_AMOUNT
live_user_id=${LIVE_USER_ID:-}
live_random_id=${LIVE_RANDOM_ID:-}
live_request_payload=${LIVE_REQUEST_PAYLOAD:-}
live_updates_json=${LIVE_UPDATES_JSON:-}

for value_name in CALLBACK_USER_ID CALLBACK_MESSAGE_ID COLLECTIBLE_PURCHASE_DATE COLLECTIBLE_AMOUNT COLLECTIBLE_CRYPTO_AMOUNT; do
  value=${!value_name}
  if [[ ! "$value" =~ ^[0-9]+$ ]]; then
    printf '%s must be a non-negative integer\n' "$value_name" >&2
    exit 1
  fi
done
if (( CALLBACK_USER_ID <= 0 || CALLBACK_MESSAGE_ID <= 0 || COLLECTIBLE_PURCHASE_DATE <= 0 || COLLECTIBLE_AMOUNT < 0 || COLLECTIBLE_CRYPTO_AMOUNT < 0 )); then
  printf 'callback IDs, purchase date, amount and crypto amount are invalid\n' >&2
  exit 1
fi
if [[ -n "$live_user_id$live_random_id$live_request_payload$live_updates_json" ]]; then
  if [[ -z "$live_user_id" || -z "$live_random_id" || -z "$live_request_payload" || -z "$live_updates_json" ]]; then
    printf 'LIVE_USER_ID, LIVE_RANDOM_ID, LIVE_REQUEST_PAYLOAD and LIVE_UPDATES_JSON must be provided together\n' >&2
    exit 1
  fi
  [[ "$live_user_id" =~ ^[0-9]+$ && "$live_random_id" =~ ^[0-9]+$ ]] || {
    printf 'LIVE_USER_ID and LIVE_RANDOM_ID must be non-negative integers\n' >&2
    exit 1
  }
  if (( live_user_id <= 0 || live_random_id <= 0 )); then
    printf 'LIVE_USER_ID and LIVE_RANDOM_ID must be positive\n' >&2
    exit 1
  fi
  live_enabled=true
else
  live_enabled=false
fi

case "$CALLBACK_USER_ID" in
  ''|*[!0-9]*) printf 'CALLBACK_USER_ID must be a positive integer\n' >&2; exit 1 ;;
esac
case "$CALLBACK_MESSAGE_ID" in
  ''|*[!0-9]*) printf 'CALLBACK_MESSAGE_ID must be a positive integer\n' >&2; exit 1 ;;
esac
if (( CALLBACK_USER_ID <= 0 || CALLBACK_MESSAGE_ID <= 0 )); then
  printf 'CALLBACK_USER_ID and CALLBACK_MESSAGE_ID must be positive\n' >&2
  exit 1
fi

# psql variables are not expanded inside a dollar-quoted PL/pgSQL body. Validate
# the supplied JSON before the transaction so malformed key rows cannot be
# inserted and later make the CDN provider fail readiness.
cdn_keys_valid=$(psql "$DATABASE_URL" --set=ON_ERROR_STOP=1 \
  --variable=cdn_public_keys_json="$CDN_PUBLIC_KEYS_JSON" -Atq <<'SQL'
SELECT CASE WHEN jsonb_typeof(:'cdn_public_keys_json'::jsonb) <> 'array'
  OR jsonb_array_length(:'cdn_public_keys_json'::jsonb) = 0
  OR EXISTS (
    SELECT 1 FROM jsonb_array_elements(:'cdn_public_keys_json'::jsonb) AS item
    WHERE item->>'predicate_name' <> 'cdnPublicKey'
       OR item->>'dc_id' !~ '^[0-9]+$'
       OR (item->>'dc_id')::integer <= 0
       OR btrim(item->>'public_key') = ''
  ) THEN 'false' ELSE 'true' END;
SQL
)
if [[ "$cdn_keys_valid" != "true" ]]; then
  printf 'CDN_PUBLIC_KEYS_JSON must contain canonical non-empty cdnPublicKey objects\n' >&2
  exit 1
fi

# The callback provider hashes the raw callback data, so changing CALLBACK_DATA
# creates a separate idempotent row. All values are explicit seed inputs; this
# helper intentionally has no sample identities, URLs, or response payloads.
psql "$DATABASE_URL" --set=ON_ERROR_STOP=1 \
  --variable=callback_user_id="$CALLBACK_USER_ID" \
  --variable=callback_message_id="$CALLBACK_MESSAGE_ID" \
  --variable=callback_data="$CALLBACK_DATA" \
  --variable=callback_answer_json="$CALLBACK_ANSWER_JSON" \
  --variable=collectible_username="$COLLECTIBLE_USERNAME" \
  --variable=collectible_purchase_date="$COLLECTIBLE_PURCHASE_DATE" \
  --variable=collectible_currency="$COLLECTIBLE_CURRENCY" \
  --variable=collectible_amount="$COLLECTIBLE_AMOUNT" \
  --variable=collectible_crypto_currency="$collectible_crypto_currency" \
  --variable=collectible_crypto_amount="$collectible_crypto_amount" \
  --variable=collectible_url="$COLLECTIBLE_URL" \
  --variable=cdn_public_keys_json="$CDN_PUBLIC_KEYS_JSON" \
  --variable=live_enabled="$live_enabled" \
  --variable=live_user_id="$live_user_id" \
  --variable=live_random_id="$live_random_id" \
  --variable=live_request_payload="$live_request_payload" \
  --variable=live_updates_json="$live_updates_json" <<'SQL'
BEGIN;

INSERT INTO teamgram_business_collectible
  (lookup_key, purchase_date, currency, amount, crypto_currency, crypto_amount, url, active, expires_at)
VALUES
  ('username:' || lower(trim(:'collectible_username')), (:'collectible_purchase_date')::integer,
   :'collectible_currency', (:'collectible_amount')::bigint, :'collectible_crypto_currency',
   (:'collectible_crypto_amount')::bigint, :'collectible_url', TRUE, NULL)
ON CONFLICT (lookup_key) DO UPDATE
  SET purchase_date = EXCLUDED.purchase_date,
      currency = EXCLUDED.currency,
      amount = EXCLUDED.amount,
      crypto_currency = EXCLUDED.crypto_currency,
      crypto_amount = EXCLUDED.crypto_amount,
      url = EXCLUDED.url,
      active = EXCLUDED.active,
      expires_at = EXCLUDED.expires_at,
      updated_at = CURRENT_TIMESTAMP;

INSERT INTO teamgram_business_callback_answer
  (user_id, peer_key, message_id, data_digest, answer, expires_at)
VALUES
  (:'callback_user_id'::bigint,
   'inputPeerSelf:{"predicate_name":"inputPeerSelf","constructor":2107670217}',
   :'callback_message_id'::bigint,
   digest(convert_to(:'callback_data', 'UTF8'), 'sha256'),
   (:'callback_answer_json')::jsonb,
   CURRENT_TIMESTAMP + INTERVAL '1 hour')
ON CONFLICT (user_id, peer_key, message_id, data_digest) DO UPDATE
  SET answer = EXCLUDED.answer,
      expires_at = EXCLUDED.expires_at,
      updated_at = CURRENT_TIMESTAMP;

\if :live_enabled
INSERT INTO teamgram_business_live_session
  (user_id, random_id, request_fingerprint, updates)
VALUES
  (:'live_user_id'::bigint, :'live_random_id'::bigint,
   digest(convert_to(:'live_request_payload', 'UTF8'), 'sha256'),
   (:'live_updates_json')::jsonb)
ON CONFLICT (user_id, random_id) DO UPDATE
  SET request_fingerprint = EXCLUDED.request_fingerprint,
      updates = EXCLUDED.updates;
\endif

-- This is the canonical location JSON used by the isolated CDN probe.  The
-- provider computes source_key as SHA-256(raw location bytes), then serves
-- AES-CTR encrypted bytes and SHA-256 part hashes from this row.
WITH source AS (
  SELECT encode(digest(convert_to('{"access_hash":99,"id":42,"predicate_name":"inputDocumentFileLocation"}', 'UTF8'), 'sha256'), 'hex') AS source_key
)
INSERT INTO teamgram_cdn_object
  (file_token, source_key, plaintext, encryption_key, encryption_iv, dc_id)
SELECT digest(convert_to('teamgram-cdn:' || source_key, 'UTF8'), 'sha256'),
       source_key,
       convert_to('seeded Teamgram CDN object\n', 'UTF8'),
       digest(convert_to('teamgram-cdn-key:' || source_key, 'UTF8'), 'sha256'),
       substring(digest(convert_to('teamgram-cdn-iv:' || source_key, 'UTF8'), 'sha256') FROM 1 FOR 16),
       2
FROM source
ON CONFLICT (source_key) DO UPDATE
  SET plaintext = EXCLUDED.plaintext,
      encryption_key = EXCLUDED.encryption_key,
      encryption_iv = EXCLUDED.encryption_iv,
      dc_id = EXCLUDED.dc_id;

INSERT INTO teamgram_cdn_config (id, public_keys)
VALUES (1, (:'cdn_public_keys_json')::jsonb)
ON CONFLICT (id) DO UPDATE SET public_keys = EXCLUDED.public_keys, updated_at = CURRENT_TIMESTAMP;

COMMIT;
SQL
