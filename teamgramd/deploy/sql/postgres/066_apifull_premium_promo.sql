-- PostgreSQL 18 premium pricing catalogue. The row is deployment-owned and
-- must be provisioned before help.getPremiumPromo can use the local path.
CREATE TABLE IF NOT EXISTS apifull_premium_promo (
    id SMALLINT PRIMARY KEY CHECK (id = 1),
    promo JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT apifull_premium_promo_object CHECK (jsonb_typeof(promo) = 'object')
);
