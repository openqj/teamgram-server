CREATE TABLE IF NOT EXISTS apifull_game_score (
    scope TEXT NOT NULL CHECK (scope IN ('peer', 'inline')),
    game_key TEXT NOT NULL,
    user_id BIGINT NOT NULL,
    score INTEGER NOT NULL CHECK (score >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (scope, game_key, user_id)
);

CREATE INDEX IF NOT EXISTS idx_apifull_game_score_board
    ON apifull_game_score (scope, game_key, score DESC, user_id ASC);
