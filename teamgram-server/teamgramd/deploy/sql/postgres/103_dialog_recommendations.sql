-- Dialog Recommendations Tables
-- 推荐对话功能数据库表

-- 1. 推荐对话关系表
CREATE TABLE IF NOT EXISTS dialog_recommendations (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    peer_type INT NOT NULL,        -- 1:user, 2:chat, 3:channel
    peer_id BIGINT NOT NULL,

    -- 推荐来源和类型
    recommendation_type INT NOT NULL DEFAULT 0,
    -- 0: 算法推荐
    -- 1: 系统置顶/推广
    -- 2: 热门对话
    -- 3: 新用户引导

    -- 推荐权重
    weight INT NOT NULL DEFAULT 0,           -- 排序权重，越高越靠前
    score DOUBLE PRECISION DEFAULT 0.0,      -- 算法评分 (0.0-1.0)

    -- 推广相关字段
    is_promoted BOOLEAN DEFAULT FALSE,       -- 是否为推广对话
    promoted_until BIGINT DEFAULT 0,         -- 推广到期时间戳（秒）
    promoted_priority INT DEFAULT 0,         -- 推广优先级

    -- 时间戳
    recommended_at BIGINT NOT NULL,          -- 推荐时间戳（秒）
    expires_at BIGINT DEFAULT 0,             -- 过期时间戳（秒），0表示不过期
    last_updated BIGINT DEFAULT 0,

    -- 用户行为追踪
    shown_count INT DEFAULT 0,               -- 展示次数
    clicked_count INT DEFAULT 0,             -- 点击次数
    dismissed BOOLEAN DEFAULT FALSE,         -- 用户是否隐藏
    dismissed_at BIGINT DEFAULT 0,

    deleted BOOLEAN DEFAULT FALSE,

    CONSTRAINT unique_user_peer UNIQUE(user_id, peer_type, peer_id)
);

-- 索引优化
CREATE INDEX idx_recommendations_user
    ON dialog_recommendations(user_id)
    WHERE deleted = FALSE;

CREATE INDEX idx_recommendations_promoted
    ON dialog_recommendations(user_id, is_promoted, promoted_priority DESC)
    WHERE deleted = FALSE AND is_promoted = TRUE;

CREATE INDEX idx_recommendations_weight
    ON dialog_recommendations(user_id, weight DESC, score DESC)
    WHERE deleted = FALSE AND dismissed = FALSE;

CREATE INDEX idx_recommendations_expires
    ON dialog_recommendations(expires_at)
    WHERE deleted = FALSE AND expires_at > 0;

-- 2. 推荐配置表
CREATE TABLE IF NOT EXISTS recommendation_configs (
    id SERIAL PRIMARY KEY,
    config_key VARCHAR(100) NOT NULL UNIQUE,
    config_value TEXT NOT NULL,              -- JSON 格式配置
    enabled BOOLEAN DEFAULT TRUE,
    description TEXT,
    created_at BIGINT DEFAULT 0,
    updated_at BIGINT DEFAULT 0
);

-- 初始配置
INSERT INTO recommendation_configs (config_key, config_value, description, created_at, updated_at)
VALUES
    ('max_recommendations', '20', '每用户最大推荐数', EXTRACT(EPOCH FROM NOW())::BIGINT, EXTRACT(EPOCH FROM NOW())::BIGINT),
    ('recommendation_ttl_days', '7', '推荐有效期（天）', EXTRACT(EPOCH FROM NOW())::BIGINT, EXTRACT(EPOCH FROM NOW())::BIGINT),
    ('promoted_max_slots', '3', '同时展示推广对话上限', EXTRACT(EPOCH FROM NOW())::BIGINT, EXTRACT(EPOCH FROM NOW())::BIGINT),
    ('algorithm_weights', '{"frequency":0.3,"recency":0.3,"engagement":0.2,"social":0.2}', '算法权重配置', EXTRACT(EPOCH FROM NOW())::BIGINT, EXTRACT(EPOCH FROM NOW())::BIGINT),
    ('refresh_interval_hours', '6', '推荐刷新间隔（小时）', EXTRACT(EPOCH FROM NOW())::BIGINT, EXTRACT(EPOCH FROM NOW())::BIGINT)
ON CONFLICT (config_key) DO NOTHING;

-- 3. 推荐行为日志表（用于分析和优化）
CREATE TABLE IF NOT EXISTS recommendation_feedback_logs (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    peer_type INT NOT NULL,
    peer_id BIGINT NOT NULL,
    feedback_type INT NOT NULL,
    -- 1: clicked (点击)
    -- 2: dismissed (隐藏)
    -- 3: pinned (置顶)
    -- 4: archived (归档)
    -- 5: muted (静音)
    timestamp BIGINT NOT NULL,
    session_id VARCHAR(100),                 -- 会话标识
    device_info TEXT                         -- 设备信息（JSON）
);

CREATE INDEX idx_feedback_user_time
    ON recommendation_feedback_logs(user_id, timestamp DESC);

COMMENT ON TABLE dialog_recommendations IS '对话推荐关系表';
COMMENT ON TABLE recommendation_configs IS '推荐系统配置表';
COMMENT ON TABLE recommendation_feedback_logs IS '推荐反馈日志表';
