-- Preserve idempotency for payment query updates queued before migration 091.
WITH candidates AS (
    SELECT
        bot_id,
        update_id,
        CASE
            WHEN jsonb_exists(update_data, 'shipping_query')
                THEN 'shipping_query:' || (update_data->'shipping_query'->>'id')
            WHEN jsonb_exists(update_data, 'pre_checkout_query')
                THEN 'precheckout_query:' || (update_data->'pre_checkout_query'->>'id')
        END AS dedupe_key
    FROM bot_api_updates
    WHERE jsonb_exists(update_data, 'shipping_query') OR jsonb_exists(update_data, 'pre_checkout_query')
), first_seen AS (
    SELECT DISTINCT ON (bot_id, dedupe_key)
        bot_id, dedupe_key, update_id
    FROM candidates
    WHERE dedupe_key IS NOT NULL AND right(dedupe_key, 1) <> ':'
    ORDER BY bot_id, dedupe_key, update_id
)
INSERT INTO bot_api_update_dedup(bot_id, dedupe_key, first_update_id)
SELECT bot_id, dedupe_key, update_id
FROM first_seen
ON CONFLICT (bot_id, dedupe_key) DO NOTHING;
