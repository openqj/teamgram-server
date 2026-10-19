ALTER TABLE apifull_stars_sub2api_outbox
    ADD COLUMN IF NOT EXISTS amount_usd NUMERIC(20,8)
    CHECK (amount_usd IS NULL OR amount_usd > 0);
