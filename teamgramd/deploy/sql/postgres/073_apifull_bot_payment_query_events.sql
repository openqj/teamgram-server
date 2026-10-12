-- Allow durable delivery intents for the initial Layer 229 bot queries.
-- Answered events remain available for payment state wakeups; query events
-- drive updateBotShippingQuery/updateBotPrecheckoutQuery delivery after a
-- process restart.

ALTER TABLE apifull_bot_payment_outbox
    DROP CONSTRAINT IF EXISTS apifull_bot_payment_outbox_event_type_check;

ALTER TABLE apifull_bot_payment_outbox
    ADD CONSTRAINT apifull_bot_payment_outbox_event_type_check
    CHECK (event_type IN (
        'shipping_query', 'precheckout_query',
        'shipping_answered', 'precheckout_answered'
    ));
