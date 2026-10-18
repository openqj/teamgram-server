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

CALLBACK_TTL_SECONDS=${CALLBACK_TTL_SECONDS:-86400}
COLLECTIBLE_CRYPTO_CURRENCY=${COLLECTIBLE_CRYPTO_CURRENCY:-}
COLLECTIBLE_CRYPTO_AMOUNT=${COLLECTIBLE_CRYPTO_AMOUNT:-0}
collectible_crypto_currency=$COLLECTIBLE_CRYPTO_CURRENCY
collectible_crypto_amount=$COLLECTIBLE_CRYPTO_AMOUNT
live_user_id=${LIVE_USER_ID:-}
live_random_id=${LIVE_RANDOM_ID:-}
live_request_payload=${LIVE_REQUEST_PAYLOAD:-}
live_updates_json=${LIVE_UPDATES_JSON:-}
business_connections_json=${BUSINESS_CONNECTIONS_JSON:-}
gift_ownerships_json=${GIFT_OWNERSHIPS_JSON:-}
stars_credits_json=${STARS_CREDITS_JSON:-}
business_bot_operations_json=${BUSINESS_BOT_OPERATIONS_JSON:-}
business_payment_operations_json=${BUSINESS_PAYMENT_OPERATIONS_JSON:-}
business_connections_enabled=false
gift_ownerships_enabled=false
stars_credits_enabled=false
business_bot_operations_enabled=false
business_payment_operations_enabled=false

for value_name in CALLBACK_USER_ID CALLBACK_MESSAGE_ID CALLBACK_TTL_SECONDS COLLECTIBLE_PURCHASE_DATE COLLECTIBLE_AMOUNT COLLECTIBLE_CRYPTO_AMOUNT; do
  value=${!value_name}
  if [[ ! "$value" =~ ^[0-9]+$ ]]; then
    printf '%s must be a non-negative integer\n' "$value_name" >&2
    exit 1
  fi
done
if (( CALLBACK_USER_ID <= 0 || CALLBACK_MESSAGE_ID <= 0 || CALLBACK_TTL_SECONDS <= 0 || COLLECTIBLE_PURCHASE_DATE <= 0 || COLLECTIBLE_AMOUNT < 0 || COLLECTIBLE_CRYPTO_AMOUNT < 0 )); then
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

if [[ -n "$business_connections_json" ]]; then
  business_connections_valid=$(psql "$DATABASE_URL" --set=ON_ERROR_STOP=1 \
    --variable=business_connections_json="$business_connections_json" -Atq <<'SQL'
WITH input AS (
  SELECT :'business_connections_json'::jsonb AS value
)
SELECT CASE
  WHEN jsonb_typeof(value) <> 'array' THEN 'false'
  WHEN EXISTS (
    SELECT 1
    FROM jsonb_array_elements(value) AS item
    WHERE jsonb_typeof(item) IS DISTINCT FROM 'object'
       OR jsonb_typeof(item->'connection_id') IS DISTINCT FROM 'string'
       OR COALESCE(btrim(item->>'connection_id'), '') = ''
       OR jsonb_typeof(item->'owner_user_id') IS DISTINCT FROM 'number'
       OR (item->>'owner_user_id') !~ '^[1-9][0-9]*$'
       OR jsonb_typeof(item->'bot_user_id') IS DISTINCT FROM 'number'
       OR (item->>'bot_user_id') !~ '^[1-9][0-9]*$'
       OR jsonb_typeof(item->'dc_id') IS DISTINCT FROM 'number'
       OR (item->>'dc_id') !~ '^[1-9][0-9]*$'
       OR jsonb_typeof(item->'connection_date') IS DISTINCT FROM 'number'
       OR (item->>'connection_date') !~ '^[1-9][0-9]*$'
       OR jsonb_typeof(item->'qts') IS DISTINCT FROM 'number'
       OR (item->>'qts') !~ '^[1-9][0-9]*$'
       OR jsonb_typeof(item->'disabled') IS DISTINCT FROM 'boolean'
       OR jsonb_typeof(item->'can_reply') IS DISTINCT FROM 'boolean'
       OR jsonb_typeof(item->'rights') IS DISTINCT FROM 'object'
  ) THEN 'false'
  ELSE 'true'
END
FROM input;
SQL
  )
  if [[ "$business_connections_valid" != "true" ]]; then
    printf 'BUSINESS_CONNECTIONS_JSON must contain canonical business connection objects\n' >&2
    exit 1
  fi
  business_connections_unique=$(psql "$DATABASE_URL" --set=ON_ERROR_STOP=1 \
    --variable=business_connections_json="$business_connections_json" -Atq <<'SQL'
WITH input AS (
  SELECT :'business_connections_json'::jsonb AS value
), rows AS (
  SELECT connection_id, owner_user_id, bot_user_id, qts
  FROM input, jsonb_to_recordset(value) AS item(
    connection_id text,
    owner_user_id bigint,
    bot_user_id bigint,
    dc_id integer,
    connection_date integer,
    qts integer,
    disabled boolean,
    can_reply boolean,
    rights jsonb
  )
)
SELECT CASE WHEN EXISTS (
  SELECT connection_id FROM rows GROUP BY connection_id HAVING count(*) > 1
) OR EXISTS (
  SELECT owner_user_id, bot_user_id FROM rows GROUP BY owner_user_id, bot_user_id HAVING count(*) > 1
) OR EXISTS (
  SELECT bot_user_id, qts FROM rows GROUP BY bot_user_id, qts HAVING count(*) > 1
) THEN 'false' ELSE 'true' END;
SQL
  )
  if [[ "$business_connections_unique" != "true" ]]; then
    printf 'BUSINESS_CONNECTIONS_JSON must not contain duplicate connection or owner/bot/qts keys\n' >&2
    exit 1
  fi
  business_connections_enabled=true
fi

if [[ -n "$gift_ownerships_json" ]]; then
  gift_ownerships_valid=$(psql "$DATABASE_URL" --set=ON_ERROR_STOP=1 \
    --variable=gift_ownerships_json="$gift_ownerships_json" -Atq <<'SQL'
WITH input AS (
  SELECT :'gift_ownerships_json'::jsonb AS value
)
SELECT CASE
  WHEN jsonb_typeof(value) <> 'array' THEN 'false'
  WHEN EXISTS (
    SELECT 1
    FROM jsonb_array_elements(value) AS item
    WHERE jsonb_typeof(item) IS DISTINCT FROM 'object'
       OR jsonb_typeof(item->'from_user') IS DISTINCT FROM 'number'
       OR (item->>'from_user') !~ '^[1-9][0-9]*$'
       OR jsonb_typeof(item->'to_user') IS DISTINCT FROM 'number'
       OR (item->>'to_user') !~ '^[1-9][0-9]*$'
       OR jsonb_typeof(item->'slug') IS DISTINCT FROM 'string'
       OR COALESCE(btrim(item->>'slug'), '') = ''
       OR jsonb_typeof(item->'stars') IS DISTINCT FROM 'number'
       OR (item->>'stars') !~ '^[0-9]+$'
       OR jsonb_typeof(item->'saved') IS DISTINCT FROM 'boolean'
  ) THEN 'false'
  ELSE 'true'
END
FROM input;
SQL
  )
  if [[ "$gift_ownerships_valid" != "true" ]]; then
    printf 'GIFT_OWNERSHIPS_JSON must contain canonical gift ownership objects\n' >&2
    exit 1
  fi
  gift_ownerships_unique=$(psql "$DATABASE_URL" --set=ON_ERROR_STOP=1 \
    --variable=gift_ownerships_json="$gift_ownerships_json" -Atq <<'SQL'
WITH input AS (
  SELECT :'gift_ownerships_json'::jsonb AS value
), rows AS (
  SELECT from_user, to_user, slug
  FROM input, jsonb_to_recordset(value) AS item(
    from_user bigint,
    to_user bigint,
    slug text,
    stars bigint,
    saved boolean
  )
)
SELECT CASE WHEN EXISTS (
  SELECT from_user, to_user, slug FROM rows
  GROUP BY from_user, to_user, slug HAVING count(*) > 1
) THEN 'false' ELSE 'true' END;
SQL
  )
  if [[ "$gift_ownerships_unique" != "true" ]]; then
    printf 'GIFT_OWNERSHIPS_JSON must not contain duplicate from_user/to_user/slug keys\n' >&2
    exit 1
  fi
  gift_ownerships_enabled=true
fi

if [[ -n "$stars_credits_json" ]]; then
  stars_credits_valid=$(psql "$DATABASE_URL" --set=ON_ERROR_STOP=1 \
    --variable=stars_credits_json="$stars_credits_json" -Atq <<'SQL'
WITH input AS (
  SELECT :'stars_credits_json'::jsonb AS value
)
SELECT CASE
  WHEN jsonb_typeof(value) <> 'array' THEN 'false'
  WHEN EXISTS (
    SELECT 1
    FROM jsonb_array_elements(value) AS item
    WHERE jsonb_typeof(item) IS DISTINCT FROM 'object'
       OR jsonb_typeof(item->'user_id') IS DISTINCT FROM 'number'
       OR (item->>'user_id') !~ '^[1-9][0-9]*$'
       OR jsonb_typeof(item->'stars') IS DISTINCT FROM 'number'
       OR (item->>'stars') !~ '^[1-9][0-9]*$'
       OR jsonb_typeof(item->'transaction_id') IS DISTINCT FROM 'string'
       OR COALESCE(btrim(item->>'transaction_id'), '') = ''
       OR char_length(item->>'transaction_id') > 191
  ) THEN 'false'
  ELSE 'true'
END
FROM input;
SQL
  )
  if [[ "$stars_credits_valid" != "true" ]]; then
    printf 'STARS_CREDITS_JSON must contain positive user_id/stars and a transaction_id\n' >&2
    exit 1
  fi
  stars_credits_unique=$(psql "$DATABASE_URL" --set=ON_ERROR_STOP=1 \
    --variable=stars_credits_json="$stars_credits_json" -Atq <<'SQL'
WITH input AS (
  SELECT :'stars_credits_json'::jsonb AS value
), rows AS (
  SELECT user_id, transaction_id
  FROM input, jsonb_to_recordset(value) AS item(
    user_id bigint,
    stars bigint,
    transaction_id text
  )
)
SELECT CASE WHEN EXISTS (
  SELECT user_id, transaction_id FROM rows
  GROUP BY user_id, transaction_id HAVING count(*) > 1
) THEN 'false' ELSE 'true' END;
SQL
  )
  if [[ "$stars_credits_unique" != "true" ]]; then
    printf 'STARS_CREDITS_JSON must not contain duplicate user_id/transaction_id keys\n' >&2
    exit 1
  fi
  stars_credits_enabled=true
fi

for operations_name in business_bot_operations business_payment_operations; do
  operations_json=${operations_name}_json
  operations_enabled=${operations_name}_enabled
  if [[ -n "${!operations_json}" ]]; then
    operations_valid=$(psql "$DATABASE_URL" --set=ON_ERROR_STOP=1 \
      --variable=operations_json="${!operations_json}" -Atq <<'SQL'
WITH input AS (
  SELECT :'operations_json'::jsonb AS value
), rows AS (
  SELECT item->>'operation' AS operation,
         item->>'user_id' AS user_id,
         item->>'payload' AS payload,
         item->'result' AS result
  FROM input, jsonb_array_elements(value) AS item
)
SELECT CASE
  WHEN jsonb_typeof((SELECT value FROM input)) <> 'array' THEN 'false'
  WHEN EXISTS (
    SELECT 1 FROM rows
    WHERE COALESCE(btrim(operation), '') = ''
       OR user_id !~ '^[1-9][0-9]*$'
       OR COALESCE(btrim(payload), '') = ''
       OR jsonb_typeof(result) IS DISTINCT FROM 'object'
       OR jsonb_typeof(payload::jsonb) IS DISTINCT FROM 'object'
  ) THEN 'false'
  ELSE 'true'
END;
SQL
    )
    if [[ "$operations_valid" != "true" ]]; then
      printf '%s_JSON must contain operation, positive user_id, object payload JSON text and object result\n' "${operations_name^^}" >&2
      exit 1
    fi
    operations_unique=$(psql "$DATABASE_URL" --set=ON_ERROR_STOP=1 \
      --variable=operations_json="${!operations_json}" -Atq <<'SQL'
WITH input AS (
  SELECT :'operations_json'::jsonb AS value
), rows AS (
  SELECT item->>'operation' AS operation,
         (item->>'user_id')::bigint AS user_id,
         digest(convert_to(item->>'payload', 'UTF8'), 'sha256') AS request_fingerprint
  FROM input, jsonb_array_elements(value) AS item
)
SELECT CASE WHEN EXISTS (
  SELECT operation, user_id, request_fingerprint
  FROM rows
  GROUP BY operation, user_id, request_fingerprint
  HAVING count(*) > 1
) THEN 'false' ELSE 'true' END;
SQL
    )
    if [[ "$operations_unique" != "true" ]]; then
      printf '%s_JSON must not contain duplicate operation/user_id/payload rows\n' "${operations_name^^}" >&2
      exit 1
    fi
    printf -v "$operations_enabled" '%s' true
  fi
done

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
  --variable=callback_ttl_seconds="$CALLBACK_TTL_SECONDS" \
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
  --variable=live_updates_json="$live_updates_json" \
  --variable=business_connections_enabled="$business_connections_enabled" \
  --variable=business_connections_json="$business_connections_json" \
  --variable=gift_ownerships_enabled="$gift_ownerships_enabled" \
  --variable=gift_ownerships_json="$gift_ownerships_json" \
  --variable=stars_credits_enabled="$stars_credits_enabled" \
  --variable=stars_credits_json="$stars_credits_json" \
  --variable=business_bot_operations_enabled="$business_bot_operations_enabled" \
  --variable=business_bot_operations_json="$business_bot_operations_json" \
  --variable=business_payment_operations_enabled="$business_payment_operations_enabled" \
  --variable=business_payment_operations_json="$business_payment_operations_json" <<'SQL'
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
   CURRENT_TIMESTAMP + (:'callback_ttl_seconds')::bigint * INTERVAL '1 second')
ON CONFLICT (user_id, peer_key, message_id, data_digest) DO UPDATE
  SET answer = EXCLUDED.answer,
      expires_at = EXCLUDED.expires_at,
      updated_at = CURRENT_TIMESTAMP;

\if :business_connections_enabled
INSERT INTO apifull_business_connection
  (connection_id, owner_user_id, bot_user_id, dc_id, connection_date, qts,
   disabled, can_reply, rights)
SELECT connection_id, owner_user_id, bot_user_id, dc_id, connection_date, qts,
       disabled, can_reply, rights
FROM jsonb_to_recordset(:'business_connections_json'::jsonb) AS item(
  connection_id text,
  owner_user_id bigint,
  bot_user_id bigint,
  dc_id integer,
  connection_date integer,
  qts integer,
  disabled boolean,
  can_reply boolean,
  rights jsonb
)
ON CONFLICT (connection_id) DO UPDATE
  SET owner_user_id = EXCLUDED.owner_user_id,
      bot_user_id = EXCLUDED.bot_user_id,
      dc_id = EXCLUDED.dc_id,
      connection_date = EXCLUDED.connection_date,
      qts = EXCLUDED.qts,
      disabled = EXCLUDED.disabled,
      can_reply = EXCLUDED.can_reply,
      rights = EXCLUDED.rights,
      updated_at = CURRENT_TIMESTAMP;
\endif

\if :gift_ownerships_enabled
DELETE FROM apifull_gift AS gift
USING jsonb_to_recordset(:'gift_ownerships_json'::jsonb) AS item(
  from_user bigint,
  to_user bigint,
  slug text,
  stars bigint,
  saved boolean
)
WHERE gift.from_user = item.from_user
  AND gift.to_user = item.to_user
  AND gift.slug = item.slug;

INSERT INTO apifull_gift (from_user, to_user, slug, stars, saved)
SELECT from_user, to_user, slug, stars, CASE WHEN saved THEN 1 ELSE 0 END
FROM jsonb_to_recordset(:'gift_ownerships_json'::jsonb) AS item(
  from_user bigint,
  to_user bigint,
  slug text,
  stars bigint,
  saved boolean
);
\endif

\if :stars_credits_enabled
WITH credits AS (
  INSERT INTO apifull_star_tx (user_id, amount, idem)
  SELECT user_id, stars, 'payment:credit:' || transaction_id
  FROM jsonb_to_recordset(:'stars_credits_json'::jsonb) AS item(
    user_id bigint,
    stars bigint,
    transaction_id text
  )
  ON CONFLICT (user_id, idem) DO NOTHING
  RETURNING user_id, amount
), totals AS (
  SELECT user_id, sum(amount) AS amount
  FROM credits
  GROUP BY user_id
)
INSERT INTO apifull_stars (user_id, balance)
SELECT user_id, amount
FROM totals
ON CONFLICT (user_id) DO UPDATE
  SET balance = apifull_stars.balance + EXCLUDED.balance;
\endif

\if :business_bot_operations_enabled
INSERT INTO teamgram_business_bot_operation
  (operation, user_id, request_fingerprint, result, expires_at)
SELECT operation, user_id, digest(convert_to(payload, 'UTF8'), 'sha256'), result,
       COALESCE(expires_at, CURRENT_TIMESTAMP + INTERVAL '10 minutes')
FROM jsonb_to_recordset(:'business_bot_operations_json'::jsonb) AS item(
  operation text,
  user_id bigint,
  payload text,
  result jsonb,
  expires_at timestamptz
)
ON CONFLICT (operation, user_id, request_fingerprint) DO UPDATE
  SET result = EXCLUDED.result,
      expires_at = EXCLUDED.expires_at,
      updated_at = CURRENT_TIMESTAMP;
\endif

\if :business_payment_operations_enabled
INSERT INTO teamgram_business_payment_operation
  (operation, user_id, request_fingerprint, result, expires_at)
SELECT operation, user_id, digest(convert_to(payload, 'UTF8'), 'sha256'), result,
       COALESCE(expires_at, CURRENT_TIMESTAMP + INTERVAL '10 minutes')
FROM jsonb_to_recordset(:'business_payment_operations_json'::jsonb) AS item(
  operation text,
  user_id bigint,
  payload text,
  result jsonb,
  expires_at timestamptz
)
ON CONFLICT (operation, user_id, request_fingerprint) DO UPDATE
  SET result = EXCLUDED.result,
      expires_at = EXCLUDED.expires_at,
      updated_at = CURRENT_TIMESTAMP;
\endif

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
