#!/usr/bin/env bash
set -Eeuo pipefail

: "${USER_ID:?USER_ID must identify the authenticated browser account}"
: "${TEAMGRAM_GRAMJS_DIR:?TEAMGRAM_GRAMJS_DIR must point to the checked-in GramJS workspace}"

PG_CONTAINER=${PG_CONTAINER:-teamgram-postgres}
TSX=${TSX:-$TEAMGRAM_GRAMJS_DIR/node_modules/.bin/tsx}
if [[ ! -x "$TSX" ]]; then
  printf 'tsx executable not found: %s\n' "$TSX" >&2
  exit 2
fi

tag=${SPONSORED_COLLECTIBLE_TAG:-"acceptance-${USER_ID}-${PPID}-${RANDOM}"}
query=${SPONSORED_QUERY:-"${tag}"}
collectible_id=${COLLECTIBLE_ID:-$((900000000 + RANDOM))}
document_id=${DOCUMENT_ID:-$((910000000 + RANDOM))}
pattern_document_id=${PATTERN_DOCUMENT_ID:-$((920000000 + RANDOM))}

for value_name in USER_ID collectible_id document_id pattern_document_id; do
  value=${!value_name}
  if [[ ! "$value" =~ ^[0-9]+$ ]] || (( value <= 0 )); then
    printf '%s must be a positive integer\n' "$value_name" >&2
    exit 2
  fi
done
if [[ -z "$query" || ${#query} -gt 200 ]]; then
  printf 'SPONSORED_QUERY must be 1-200 characters\n' >&2
  exit 2
fi

psql_exec() {
  docker exec -i "$PG_CONTAINER" psql -U teamgram -d teamgram -v ON_ERROR_STOP=1 "$@"
}

cleanup() {
  psql_exec --variable=query="$query" --variable=owner_id="$USER_ID" \
    --variable=collectible_id="$collectible_id" --variable=document_id="$document_id" \
    --variable=pattern_document_id="$pattern_document_id" <<'SQL' >/dev/null
BEGIN;
DELETE FROM apifull_sponsored_inventory WHERE inventory_type = 'peers' AND query = :'query';
DELETE FROM apifull_collectible_emoji_status
 WHERE owner_user_id = :'owner_id'::bigint AND collectible_id = :'collectible_id'::bigint;
DELETE FROM apifull_emoji_document
 WHERE id IN (:'document_id'::bigint, :'pattern_document_id'::bigint);
COMMIT;
SQL
}
trap cleanup EXIT

collision=$(psql_exec --variable=query="$query" --variable=owner_id="$USER_ID" \
  --variable=collectible_id="$collectible_id" --variable=document_id="$document_id" \
  --variable=pattern_document_id="$pattern_document_id" -At <<'SQL'
SELECT (SELECT count(*) FROM apifull_sponsored_inventory WHERE inventory_type = 'peers' AND query = :'query')
     + (SELECT count(*) FROM apifull_collectible_emoji_status WHERE owner_user_id = :'owner_id'::bigint AND collectible_id = :'collectible_id'::bigint)
     + (SELECT count(*) FROM apifull_emoji_document WHERE id IN (:'document_id'::bigint, :'pattern_document_id'::bigint));
SQL
)
if [[ "$collision" != "0" ]]; then
  printf 'fixture identity collision; choose a new query/ID set (rows=%s)\n' "$collision" >&2
  exit 2
fi

psql_exec --variable=query="$query" --variable=owner_id="$USER_ID" \
  --variable=collectible_id="$collectible_id" --variable=document_id="$document_id" \
  --variable=pattern_document_id="$pattern_document_id" <<'SQL'
BEGIN;
INSERT INTO apifull_sponsored_inventory (inventory_type, query, peer_key, payload, active)
VALUES ('peers', :'query', '', '{"predicate_name":"contacts_sponsoredPeers","peers":[{"predicate_name":"sponsoredPeer","random_id":"c3BvbnNvcmVkLXNlZWQ=","peer":{"predicate_name":"peerUser","user_id":7}}],"chats":[],"users":[]}'::jsonb, TRUE);

INSERT INTO apifull_emoji_document (id, access_hash, mime_type, dc_id)
VALUES
  (:'document_id'::bigint, 910000001, 'application/x-tgsticker', 2),
  (:'pattern_document_id'::bigint, 920000001, 'application/x-tgsticker', 2);

INSERT INTO apifull_collectible_emoji_status
  (owner_user_id, collectible_id, document_id, title, slug, pattern_document_id,
   center_color, edge_color, pattern_color, text_color)
VALUES (:'owner_id'::bigint, :'collectible_id'::bigint, :'document_id'::bigint,
        'Acceptance Aurora', 'acceptance-aurora', :'pattern_document_id'::bigint,
        1118481, 2236962, 3355443, 4473924);
COMMIT;
SQL

TEAMGRAM_GRAMJS_DIR="$TEAMGRAM_GRAMJS_DIR" \
USER_ID="$USER_ID" SPONSORED_QUERY="$query" COLLECTIBLE_ID="$collectible_id" \
  "$TSX" "$(dirname "$0")/probe-sponsored-collectible-fixtures.ts"

# Verify cleanup immediately after the RPC. The EXIT trap still handles failed
# seed/RPC paths; a successful path removes it after the explicit cleanup.
cleanup
trap - EXIT

remaining=$(psql_exec --variable=query="$query" --variable=owner_id="$USER_ID" \
  --variable=collectible_id="$collectible_id" --variable=document_id="$document_id" \
  --variable=pattern_document_id="$pattern_document_id" -At <<'SQL'
SELECT (SELECT count(*) FROM apifull_sponsored_inventory WHERE inventory_type = 'peers' AND query = :'query')
     + (SELECT count(*) FROM apifull_collectible_emoji_status WHERE owner_user_id = :'owner_id'::bigint AND collectible_id = :'collectible_id'::bigint)
     + (SELECT count(*) FROM apifull_emoji_document WHERE id IN (:'document_id'::bigint, :'pattern_document_id'::bigint));
SQL
)
if [[ "$remaining" != "0" ]]; then
  printf 'fixture cleanup verification failed: remaining=%s\n' "$remaining" >&2
  exit 1
fi
printf '%s\n' "{\"event\":\"sponsored_collectible_fixture_cleanup\",\"query\":\"$query\",\"remaining\":0}"
