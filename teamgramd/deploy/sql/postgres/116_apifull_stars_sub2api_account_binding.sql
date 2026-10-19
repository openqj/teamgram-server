-- Keep the stable account-to-Teamgram mapping without fractional Stars state.
CREATE TABLE IF NOT EXISTS apifull_sub2api_stars_account (
    account_id VARCHAR(64) PRIMARY KEY,
    user_id BIGINT NOT NULL CHECK (user_id > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO apifull_sub2api_stars_account (account_id, user_id)
SELECT account_id, user_id
FROM apifull_sub2api_stars_residual
ON CONFLICT (account_id) DO NOTHING;

DROP TABLE IF EXISTS apifull_sub2api_stars_residual;
