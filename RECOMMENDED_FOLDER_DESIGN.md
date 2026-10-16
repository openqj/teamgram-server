# "推荐"聊天文件夹技术设计方案

## 一、功能概述

在现有的聊天文件夹（Dialog Filters）功能基础上，增加一个系统级的"推荐"固定文件夹，位于"全部"文件夹右侧。该文件夹展示：
- 算法推荐的对话（基于用户行为）
- 系统后台设置的置顶推广对话

### 核心特性
- **固定展示**：与"全部"文件夹一样，不可删除、不可编辑
- **智能推荐**：基于用户行为的算法推荐
- **可推广**：支持后台配置推广对话
- **用户可控**：支持隐藏不感兴趣的推荐

## 二、文件夹 ID 分配策略

```
dialog_filter_id = 0  → "全部" (All Chats) - 系统固定
dialog_filter_id = 1  → "推荐" (Recommended) - 系统固定，新增
dialog_filter_id ≥ 2  → 用户自定义文件夹
```

## 三、数据库设计

### 3.1 推荐对话关系表

```sql
-- teamgram-server/app/service/biz/dialog/internal/dal/dataobject/dialog_recommendations.sql
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
```

### 3.2 推荐配置表

```sql
-- 全局推荐策略配置
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
INSERT INTO recommendation_configs (config_key, config_value, description, created_at, updated_at) VALUES
('max_recommendations', '20', '每用户最大推荐数', EXTRACT(EPOCH FROM NOW()), EXTRACT(EPOCH FROM NOW())),
('recommendation_ttl_days', '7', '推荐有效期（天）', EXTRACT(EPOCH FROM NOW()), EXTRACT(EPOCH FROM NOW())),
('promoted_max_slots', '3', '同时展示推广对话上限', EXTRACT(EPOCH FROM NOW()), EXTRACT(EPOCH FROM NOW())),
('algorithm_weights', '{"frequency":0.3,"recency":0.3,"engagement":0.2,"social":0.2}', '算法权重配置', EXTRACT(EPOCH FROM NOW()), EXTRACT(EPOCH FROM NOW())),
('refresh_interval_hours', '6', '推荐刷新间隔（小时）', EXTRACT(EPOCH FROM NOW()), EXTRACT(EPOCH FROM NOW()));
```

### 3.3 推荐行为日志表（可选）

```sql
-- 用于分析和优化推荐算法
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
```

## 四、服务层实现

### 4.1 目录结构

```
app/service/biz/dialog/
├── dialog.proto                              # 服务接口定义
├── internal/
│   ├── core/
│   │   ├── dialog_filter_pg.go              # 现有文件夹逻辑
│   │   ├── dialog_recommendations.go         # 新增：推荐核心逻辑
│   │   └── recommendation_algorithm.go       # 新增：推荐算法
│   ├── dal/
│   │   ├── dataobject/
│   │   │   ├── dialog_recommendations_do.go  # 新增
│   │   │   └── recommendation_configs_do.go  # 新增
│   │   └── dao/postgres_dao/
│   │       ├── dialog_recommendations_dao.go # 新增
│   │       └── recommendation_configs_dao.go # 新增
```

### 4.2 DAO 层接口

```go
// app/service/biz/dialog/internal/dal/dao/postgres_dao/dialog_recommendations_dao.go

type DialogRecommendationsDAO struct {
    db DB
}

// 插入或更新推荐
func (d *DialogRecommendationsDAO) InsertOrUpdate(ctx context.Context, 
    do *dataobject.DialogRecommendationsDO) (int64, error)

// 获取用户的推荐列表
func (d *DialogRecommendationsDAO) SelectActiveList(ctx context.Context, 
    userID int64, limit int) ([]dataobject.DialogRecommendationsDO, error)

// 获取推广对话列表
func (d *DialogRecommendationsDAO) SelectPromotedList(ctx context.Context, 
    userID int64) ([]dataobject.DialogRecommendationsDO, error)

// 标记为已隐藏
func (d *DialogRecommendationsDAO) MarkDismissed(ctx context.Context, 
    userID int64, peerType int, peerID int64) error

// 更新展示/点击统计
func (d *DialogRecommendationsDAO) UpdateStats(ctx context.Context, 
    userID int64, peerType int, peerID int64, shown, clicked int) error

// 清理过期推荐
func (d *DialogRecommendationsDAO) DeleteExpired(ctx context.Context, 
    beforeTimestamp int64) (int64, error)

// 批量插入推荐
func (d *DialogRecommendationsDAO) BatchInsert(ctx context.Context, 
    recommendations []dataobject.DialogRecommendationsDO) error
```

### 4.3 服务接口定义

```protobuf
// app/service/biz/dialog/dialog.proto

service RPCDialog {
    // ... 现有方法
    
    // 获取推荐对话列表
    rpc dialog_getRecommendedDialogs(TLDialogGetRecommendedDialogs) returns (RecommendedDialogs);
    
    // 刷新用户的推荐（手动触发）
    rpc dialog_refreshRecommendations(TLDialogRefreshRecommendations) returns (Bool);
    
    // 用户反馈（点击、隐藏等）
    rpc dialog_reportRecommendationFeedback(TLDialogReportRecommendationFeedback) returns (Bool);
    
    // === 后台管理接口 ===
    
    // 添加推广对话
    rpc dialog_addPromotedDialog(TLDialogAddPromotedDialog) returns (Bool);
    
    // 移除推广对话
    rpc dialog_removePromotedDialog(TLDialogRemovePromotedDialog) returns (Bool);
    
    // 获取推广对话列表（管理后台用）
    rpc dialog_getPromotedDialogs(TLDialogGetPromotedDialogs) returns (PromotedDialogList);
}

// 请求和响应消息
message TLDialogGetRecommendedDialogs {
    int64 user_id = 1;
    int32 offset = 2;
    int32 limit = 3;
}

message RecommendedDialogs {
    repeated RecommendedDialog dialogs = 1;
}

message RecommendedDialog {
    int32 peer_type = 1;
    int64 peer_id = 2;
    int32 recommendation_type = 3;
    bool is_promoted = 4;
    double score = 5;
}

message TLDialogReportRecommendationFeedback {
    int64 user_id = 1;
    int32 peer_type = 2;
    int64 peer_id = 3;
    int32 feedback_type = 4;  // 1:clicked, 2:dismissed, 3:pinned
}

message TLDialogAddPromotedDialog {
    int32 peer_type = 1;
    int64 peer_id = 2;
    int32 priority = 3;
    int64 expires_at = 4;
    repeated int64 target_user_ids = 5;  // 空表示全部用户
}
```

### 4.4 推荐算法实现

```go
// app/service/biz/dialog/internal/core/recommendation_algorithm.go

package core

import (
    "context"
    "math"
    "time"
)

// RecommendationAlgorithm 推荐算法
type RecommendationAlgorithm struct {
    core *DialogCore
    
    // 权重配置
    frequencyWeight  float64  // 对话频率权重
    recencyWeight    float64  // 最近活跃权重
    engagementWeight float64  // 互动程度权重
    socialWeight     float64  // 社交关系权重
}

// NewRecommendationAlgorithm 创建推荐算法实例
func NewRecommendationAlgorithm(core *DialogCore) *RecommendationAlgorithm {
    return &RecommendationAlgorithm{
        core:             core,
        frequencyWeight:  0.3,
        recencyWeight:    0.3,
        engagementWeight: 0.2,
        socialWeight:     0.2,
    }
}

// CalculateScore 计算推荐分数
func (r *RecommendationAlgorithm) CalculateScore(ctx context.Context, 
    userID int64, peerType int, peerID int64) (float64, error) {
    
    score := 0.0
    
    // 1. 对话频率分数（最近30天的消息数）
    frequencyScore, err := r.calculateFrequencyScore(ctx, userID, peerType, peerID, 30)
    if err != nil {
        return 0, err
    }
    score += frequencyScore * r.frequencyWeight
    
    // 2. 最近活跃度分数
    recencyScore, err := r.calculateRecencyScore(ctx, userID, peerType, peerID)
    if err != nil {
        return 0, err
    }
    score += recencyScore * r.recencyWeight
    
    // 3. 互动程度分数
    engagementScore, err := r.calculateEngagementScore(ctx, userID, peerType, peerID)
    if err != nil {
        return 0, err
    }
    score += engagementScore * r.engagementWeight
    
    // 4. 社交关系分数
    socialScore, err := r.calculateSocialScore(ctx, userID, peerType, peerID)
    if err != nil {
        return 0, err
    }
    score += socialScore * r.socialWeight
    
    // 归一化到 0-1
    return math.Min(1.0, math.Max(0.0, score)), nil
}

// calculateFrequencyScore 计算对话频率分数
func (r *RecommendationAlgorithm) calculateFrequencyScore(ctx context.Context,
    userID int64, peerType int, peerID int64, days int) (float64, error) {
    
    // 获取最近N天的消息数
    // TODO: 调用消息服务统计
    // messageCount := getMessageCountInDays(userID, peerType, peerID, days)
    
    // 归一化：假设日均10条消息为满分
    // return math.Min(1.0, float64(messageCount) / float64(days*10)), nil
    
    return 0.5, nil  // 临时返回
}

// calculateRecencyScore 计算最近活跃度分数
func (r *RecommendationAlgorithm) calculateRecencyScore(ctx context.Context,
    userID int64, peerType int, peerID int64) (float64, error) {
    
    // 获取最后一条消息的时间
    // lastMessageTime := getLastMessageTime(userID, peerType, peerID)
    // hoursSinceLastMessage := time.Since(lastMessageTime).Hours()
    
    // 时间衰减：24小时内=1.0, 7天=0.5, 30天=0.1
    // score := 1.0 / (1.0 + hoursSinceLastMessage/24.0)
    
    return 0.5, nil  // 临时返回
}

// calculateEngagementScore 计算互动程度分数
func (r *RecommendationAlgorithm) calculateEngagementScore(ctx context.Context,
    userID int64, peerType int, peerID int64) (float64, error) {
    
    // 考虑因素：
    // - 双向消息比例
    // - 平均回复速度
    // - 消息长度
    // - 是否有媒体内容
    
    return 0.5, nil  // 临时返回
}

// calculateSocialScore 计算社交关系分数
func (r *RecommendationAlgorithm) calculateSocialScore(ctx context.Context,
    userID int64, peerType int, peerID int64) (float64, error) {
    
    // 考虑因素：
    // - 共同好友数（私聊）
    // - 群组活跃度（群聊）
    // - 是否互相关注（频道）
    
    return 0.5, nil  // 临时返回
}

// GenerateRecommendations 为用户生成推荐列表
func (r *RecommendationAlgorithm) GenerateRecommendations(ctx context.Context, 
    userID int64, limit int) ([]RecommendedDialog, error) {
    
    // 1. 获取用户的所有活跃对话
    // dialogs := getUserActiveDialogs(userID)
    
    // 2. 排除已在推荐列表中的
    // existingRecommendations := getExistingRecommendations(userID)
    
    // 3. 为每个对话计算分数
    // scored := []struct{
    //     dialog Dialog
    //     score float64
    // }{}
    // for _, dialog := range dialogs {
    //     score, _ := r.CalculateScore(ctx, userID, dialog.PeerType, dialog.PeerID)
    //     scored = append(scored, struct{dialog Dialog; score float64}{dialog, score})
    // }
    
    // 4. 排序并取 top N
    // sort.Slice(scored, func(i, j int) bool {
    //     return scored[i].score > scored[j].score
    // })
    
    // 5. 返回推荐
    return nil, nil  // 临时返回
}
```

### 4.5 核心业务逻辑

```go
// app/service/biz/dialog/internal/core/dialog_recommendations.go

package core

import (
    "context"
    "time"
    
    "github.com/teamgram/proto/mtproto"
    "github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dal/dataobject"
)

// GetRecommendedDialogs 获取推荐对话列表
func (c *DialogCore) DialogGetRecommendedDialogs(ctx context.Context, 
    in *dialog.TLDialogGetRecommendedDialogs) (*dialog.RecommendedDialogs, error) {
    
    store, err := c.pgStore()
    if err != nil {
        return nil, err
    }
    
    userID := in.UserId
    limit := in.Limit
    if limit <= 0 || limit > 50 {
        limit = 20
    }
    
    // 1. 获取推广对话（优先级最高）
    promotedDialogs, err := store.DialogRecommendations.SelectPromotedList(ctx, userID)
    if err != nil {
        return nil, err
    }
    
    // 2. 获取算法推荐对话
    recommendedDialogs, err := store.DialogRecommendations.SelectActiveList(ctx, userID, limit)
    if err != nil {
        return nil, err
    }
    
    // 3. 合并结果（推广在前）
    result := &dialog.RecommendedDialogs{
        Dialogs: make([]*dialog.RecommendedDialog, 0, len(promotedDialogs)+len(recommendedDialogs)),
    }
    
    for _, d := range promotedDialogs {
        result.Dialogs = append(result.Dialogs, &dialog.RecommendedDialog{
            PeerType:           d.PeerType,
            PeerId:             d.PeerId,
            RecommendationType: d.RecommendationType,
            IsPromoted:         d.IsPromoted,
            Score:              d.Score,
        })
    }
    
    for _, d := range recommendedDialogs {
        if !d.IsPromoted {  // 避免重复
            result.Dialogs = append(result.Dialogs, &dialog.RecommendedDialog{
                PeerType:           d.PeerType,
                PeerId:             d.PeerId,
                RecommendationType: d.RecommendationType,
                IsPromoted:         d.IsPromoted,
                Score:              d.Score,
            })
        }
    }
    
    // 4. 更新展示统计
    go func() {
        for _, d := range result.Dialogs {
            _ = store.DialogRecommendations.UpdateStats(context.Background(), 
                userID, d.PeerType, d.PeerId, 1, 0)
        }
    }()
    
    return result, nil
}

// RefreshRecommendations 刷新用户的推荐列表
func (c *DialogCore) DialogRefreshRecommendations(ctx context.Context,
    in *dialog.TLDialogRefreshRecommendations) (*mtproto.Bool, error) {
    
    store, err := c.pgStore()
    if err != nil {
        return mtproto.BoolFalse, err
    }
    
    userID := in.UserId
    
    // 1. 创建算法实例
    algorithm := NewRecommendationAlgorithm(c)
    
    // 2. 生成推荐
    recommendations, err := algorithm.GenerateRecommendations(ctx, userID, 20)
    if err != nil {
        return mtproto.BoolFalse, err
    }
    
    // 3. 批量插入/更新
    now := time.Now().Unix()
    ttlDays := 7  // 从配置读取
    expiresAt := now + int64(ttlDays*24*3600)
    
    dataObjects := make([]dataobject.DialogRecommendationsDO, 0, len(recommendations))
    for i, rec := range recommendations {
        dataObjects = append(dataObjects, dataobject.DialogRecommendationsDO{
            UserId:             userID,
            PeerType:           rec.PeerType,
            PeerId:             rec.PeerId,
            RecommendationType: 0,  // 算法推荐
            Weight:             len(recommendations) - i,  // 递减权重
            Score:              rec.Score,
            RecommendedAt:      now,
            ExpiresAt:          expiresAt,
            LastUpdated:        now,
        })
    }
    
    err = store.DialogRecommendations.BatchInsert(ctx, dataObjects)
    if err != nil {
        return mtproto.BoolFalse, err
    }
    
    return mtproto.BoolTrue, nil
}

// ReportRecommendationFeedback 报告用户反馈
func (c *DialogCore) DialogReportRecommendationFeedback(ctx context.Context,
    in *dialog.TLDialogReportRecommendationFeedback) (*mtproto.Bool, error) {
    
    store, err := c.pgStore()
    if err != nil {
        return mtproto.BoolFalse, err
    }
    
    userID := in.UserId
    peerType := in.PeerType
    peerID := in.PeerId
    feedbackType := in.FeedbackType
    
    switch feedbackType {
    case 1:  // clicked
        err = store.DialogRecommendations.UpdateStats(ctx, userID, peerType, peerID, 0, 1)
    case 2:  // dismissed
        err = store.DialogRecommendations.MarkDismissed(ctx, userID, peerType, peerID)
    case 3:  // pinned
        // 用户将推荐对话置顶，表示推荐成功
        err = store.DialogRecommendations.UpdateStats(ctx, userID, peerType, peerID, 0, 1)
    }
    
    if err != nil {
        return mtproto.BoolFalse, err
    }
    
    return mtproto.BoolTrue, nil
}

// AddPromotedDialog 添加推广对话（后台管理接口）
func (c *DialogCore) DialogAddPromotedDialog(ctx context.Context,
    in *dialog.TLDialogAddPromotedDialog) (*mtproto.Bool, error) {
    
    store, err := c.pgStore()
    if err != nil {
        return mtproto.BoolFalse, err
    }
    
    now := time.Now().Unix()
    expiresAt := in.ExpiresAt
    if expiresAt == 0 {
        expiresAt = now + 30*24*3600  // 默认30天
    }
    
    targetUserIDs := in.TargetUserIds
    if len(targetUserIDs) == 0 {
        // TODO: 获取所有活跃用户 ID
        // targetUserIDs = getAllActiveUserIDs()
    }
    
    // 批量插入推广
    dataObjects := make([]dataobject.DialogRecommendationsDO, 0, len(targetUserIDs))
    for _, userID := range targetUserIDs {
        dataObjects = append(dataObjects, dataobject.DialogRecommendationsDO{
            UserId:             userID,
            PeerType:           in.PeerType,
            PeerId:             in.PeerId,
            RecommendationType: 1,  // 系统推广
            Weight:             1000 + in.Priority,  // 推广对话高权重
            IsPromoted:         true,
            PromotedUntil:      expiresAt,
            PromotedPriority:   in.Priority,
            RecommendedAt:      now,
            ExpiresAt:          expiresAt,
            LastUpdated:        now,
        })
    }
    
    err = store.DialogRecommendations.BatchInsert(ctx, dataObjects)
    if err != nil {
        return mtproto.BoolFalse, err
    }
    
    return mtproto.BoolTrue, nil
}
```

## 五、BFF 层实现（MTProto API）

### 5.1 修改 messages.getDialogFilters

```go
// app/bff/messages/internal/core/messages.getDialogFilters_handler.go

func (c *MessagesCore) MessagesGetDialogFilters(ctx context.Context, 
    in *mtproto.TLMessagesGetDialogFilters) (*mtproto.Messages_DialogFilters, error) {
    
    authUserID := c.MD.UserId
    
    result := &mtproto.Messages_DialogFilters{
        Filters: []*mtproto.DialogFilter{},
        TagsEnabled: false,
    }
    
    // 1. 添加"全部"文件夹（固定）
    result.Filters = append(result.Filters, mtproto.MakeTLDialogFilterDefault(&mtproto.DialogFilter{
        Constructor: mtproto.CRC32_dialogFilterDefault,
        Data2: &mtproto.DialogFilter_Data{
            Id:    0,
            Title: "All Chats",
        },
    }).To_DialogFilter())
    
    // 2. 添加"推荐"文件夹（固定，新增）
    result.Filters = append(result.Filters, mtproto.MakeTLDialogFilterDefault(&mtproto.DialogFilter{
        Constructor: mtproto.CRC32_dialogFilterDefault,  // 使用相同的 constructor
        Data2: &mtproto.DialogFilter_Data{
            Id:    1,
            Title: "Recommended",
            // 注意：客户端需要通过 id=1 识别这是推荐文件夹
        },
    }).To_DialogFilter())
    
    // 3. 加载用户自定义文件夹（id >= 2）
    filterList, err := c.svcCtx.Dao.DialogClient.DialogGetDialogFilters(ctx, &dialog.TLDialogGetDialogFilters{
        UserId: authUserID,
    })
    if err != nil {
        c.Logger.Errorf("load dialog filters error: %v", err)
        return result, nil  // 返回系统文件夹
    }
    
    for _, filter := range filterList.Datas {
        if filter.Id >= 2 {  // 只添加自定义文件夹
            result.Filters = append(result.Filters, filter.DialogFilter)
        }
    }
    
    return result, nil
}
```

### 5.2 新增 messages.getRecommendedDialogs

```go
// app/bff/messages/internal/core/messages.getRecommendedDialogs_handler.go

package core

import (
    "context"
    
    "github.com/teamgram/proto/mtproto"
)

// MessagesGetRecommendedDialogs
// messages.getRecommendedDialogs#a1b2c3d4 offset_date:int offset_id:int offset_peer:InputPeer limit:int hash:long = messages.Dialogs;
func (c *MessagesCore) MessagesGetRecommendedDialogs(ctx context.Context, 
    in *mtproto.TLMessagesGetRecommendedDialogs) (*mtproto.Messages_Dialogs, error) {
    
    authUserID := c.MD.UserId
    limit := in.Limit
    if limit <= 0 || limit > 50 {
        limit = 20
    }
    
    // 1. 从 Dialog 服务获取推荐列表
    recommended, err := c.svcCtx.Dao.DialogClient.DialogGetRecommendedDialogs(ctx, 
        &dialog.TLDialogGetRecommendedDialogs{
            UserId: authUserID,
            Offset: 0,
            Limit:  limit,
        })
    if err != nil {
        c.Logger.Errorf("get recommended dialogs error: %v", err)
        return mtproto.MakeTLMessagesDialogs(&mtproto.Messages_Dialogs{
            Dialogs:  []*mtproto.Dialog{},
            Messages: []*mtproto.Message{},
            Chats:    []*mtproto.Chat{},
            Users:    []*mtproto.User{},
        }).To_Messages_Dialogs(), nil
    }
    
    // 2. 构建对话列表
    var (
        dialogList  []*mtproto.Dialog
        messageList []*mtproto.Message
        chatList    []*mtproto.Chat
        userList    []*mtproto.User
        userIDSet   = make(map[int64]bool)
        chatIDSet   = make(map[int64]bool)
    )
    
    for _, rec := range recommended.Dialogs {
        // 获取对话详情
        dialogExt, err := c.svcCtx.Dao.DialogClient.DialogGetDialogById(ctx,
            &dialog.TLDialogGetDialogById{
                UserId:   authUserID,
                PeerType: rec.PeerType,
                PeerId:   rec.PeerId,
            })
        if err != nil {
            continue
        }
        
        // 添加 Dialog
        dialogList = append(dialogList, dialogExt.Dialog)
        
        // 添加 Top Message
        if dialogExt.TopMessage != nil {
            messageList = append(messageList, dialogExt.TopMessage)
        }
        
        // 收集需要的 User/Chat
        switch rec.PeerType {
        case 1:  // user
            userIDSet[rec.PeerId] = true
        case 2, 3:  // chat, channel
            chatIDSet[rec.PeerId] = true
        }
    }
    
    // 3. 批量获取 User/Chat 信息
    // TODO: 调用 User/Chat 服务批量获取
    
    // 4. 返回结果
    return mtproto.MakeTLMessagesDialogs(&mtproto.Messages_Dialogs{
        Dialogs:  dialogList,
        Messages: messageList,
        Chats:    chatList,
        Users:    userList,
    }).To_Messages_Dialogs(), nil
}
```

### 5.3 新增反馈接口

```go
// app/bff/messages/internal/core/messages.reportRecommendationFeedback_handler.go

package core

import (
    "context"
    
    "github.com/teamgram/proto/mtproto"
)

// MessagesReportRecommendationFeedback
// messages.reportRecommendationFeedback#c8d9e0f1 peer:InputPeer feedback_type:RecommendationFeedbackType = Bool;
func (c *MessagesCore) MessagesReportRecommendationFeedback(ctx context.Context,
    in *mtproto.TLMessagesReportRecommendationFeedback) (*mtproto.Bool, error) {
    
    authUserID := c.MD.UserId
    peer := mtproto.FromInputPeer(c.MD.UserId, in.Peer)
    feedbackType := in.FeedbackType
    
    // 调用 Dialog 服务
    _, err := c.svcCtx.Dao.DialogClient.DialogReportRecommendationFeedback(ctx,
        &dialog.TLDialogReportRecommendationFeedback{
            UserId:       authUserID,
            PeerType:     peer.PeerType,
            PeerId:       peer.PeerId,
            FeedbackType: feedbackType,
        })
    
    if err != nil {
        c.Logger.Errorf("report recommendation feedback error: %v", err)
        return mtproto.BoolFalse, nil
    }
    
    return mtproto.BoolTrue, nil
}
```

## 六、定时任务

### 6.1 推荐刷新定时任务

```go
// app/service/biz/dialog/internal/jobs/recommendation_refresh_job.go

package jobs

import (
    "context"
    "time"
    
    "github.com/zeromicro/go-zero/core/logx"
)

type RecommendationRefreshJob struct {
    core *core.DialogCore
}

func NewRecommendationRefreshJob(core *core.DialogCore) *RecommendationRefreshJob {
    return &RecommendationRefreshJob{core: core}
}

// Run 每6小时刷新一次推荐
func (j *RecommendationRefreshJob) Run() {
    ticker := time.NewTicker(6 * time.Hour)
    defer ticker.Stop()
    
    for {
        select {
        case <-ticker.C:
            j.refreshAllUsers()
        }
    }
}

func (j *RecommendationRefreshJob) refreshAllUsers() {
    ctx := context.Background()
    
    // 1. 获取需要刷新的用户列表
    // TODO: 分批获取活跃用户
    // userIDs := getActiveUserIDs()
    
    // 2. 批量刷新
    // for _, userID := range userIDs {
    //     _, err := j.core.DialogRefreshRecommendations(ctx, &dialog.TLDialogRefreshRecommendations{
    //         UserId: userID,
    //     })
    //     if err != nil {
    //         logx.Errorf("refresh recommendations for user %d error: %v", userID, err)
    //     }
    // }
    
    logx.Info("recommendation refresh job completed")
}
```

### 6.2 过期推荐清理任务

```go
// app/service/biz/dialog/internal/jobs/recommendation_cleanup_job.go

package jobs

import (
    "context"
    "time"
)

type RecommendationCleanupJob struct {
    store *postgres_dao.Store
}

func NewRecommendationCleanupJob(store *postgres_dao.Store) *RecommendationCleanupJob {
    return &RecommendationCleanupJob{store: store}
}

// Run 每天清理过期推荐
func (j *RecommendationCleanupJob) Run() {
    ticker := time.NewTicker(24 * time.Hour)
    defer ticker.Stop()
    
    for {
        select {
        case <-ticker.C:
            j.cleanup()
        }
    }
}

func (j *RecommendationCleanupJob) cleanup() {
    ctx := context.Background()
    now := time.Now().Unix()
    
    count, err := j.store.DialogRecommendations.DeleteExpired(ctx, now)
    if err != nil {
        // log error
        return
    }
    
    // log: cleaned {count} expired recommendations
}
```

## 七、客户端适配（telegram-tt）

### 7.1 类型定义

```typescript
// src/types/DialogFilter.ts

export enum DialogFilterId {
  All = 0,
  Recommended = 1,  // 新增
  // 用户自定义从 2 开始
}

export interface DialogFilter {
  id: number;
  title: string;
  isSystem?: boolean;      // 是否为系统文件夹
  isEditable?: boolean;    // 是否可编辑
  isDeletable?: boolean;   // 是否可删除
  // ... 其他字段
}

export interface RecommendedDialog {
  peerId: string;
  peerType: 'user' | 'chat' | 'channel';
  recommendationType: number;
  isPromoted?: boolean;
  score?: number;
}
```

### 7.2 API 调用

```typescript
// src/api/gramjs/methods/dialogs.ts

export async function fetchRecommendedDialogs(
  offset: number = 0,
  limit: number = 20,
): Promise<ApiDialogList> {
  const result = await invokeRequest(new GramJs.messages.GetRecommendedDialogs({
    offsetDate: 0,
    offsetId: 0,
    offsetPeer: new GramJs.InputPeerEmpty(),
    limit,
    hash: BigInt(0),
  }));

  if (!(result instanceof GramJs.messages.Dialogs)) {
    return { dialogs: [], users: [], chats: [] };
  }

  return {
    dialogs: result.dialogs.map(buildApiDialog).filter(Boolean),
    users: result.users.map(buildApiUser).filter(Boolean),
    chats: result.chats.map(buildApiChat).filter(Boolean),
  };
}

export async function reportRecommendationFeedback(
  peer: GramJs.TypeInputPeer,
  feedbackType: number,
): Promise<boolean> {
  const result = await invokeRequest(new GramJs.messages.ReportRecommendationFeedback({
    peer,
    feedbackType,
  }));

  return result instanceof GramJs.Bool && result.value;
}
```

### 7.3 UI 组件

```typescript
// src/components/left/main/ChatFolders.tsx

function ChatFolders() {
  const folders = useFolders();  // 包含 All, Recommended, 及用户自定义
  const activeFolderId = useActiveFolderId();
  
  return (
    <div className="ChatFolders">
      {folders.map((folder) => (
        <ChatFolderTab
          key={folder.id}
          folder={folder}
          isActive={activeFolderId === folder.id}
          isSystem={folder.id === 0 || folder.id === 1}  // 系统文件夹
          isEditable={folder.id >= 2}  // 只有自定义可编辑
        />
      ))}
    </div>
  );
}

// 推荐文件夹内容
function RecommendedDialogList() {
  const [dialogs, setDialogs] = useState<ApiDialog[]>([]);
  const [loading, setLoading] = useState(false);
  
  useEffect(() => {
    loadRecommendedDialogs();
  }, []);
  
  const loadRecommendedDialogs = async () => {
    setLoading(true);
    const result = await fetchRecommendedDialogs();
    setDialogs(result.dialogs);
    setLoading(false);
  };
  
  const handleDismiss = async (dialog: ApiDialog) => {
    await reportRecommendationFeedback(
      buildInputPeer(dialog.id, dialog.accessHash),
      2  // dismissed
    );
    setDialogs(dialogs.filter(d => d.id !== dialog.id));
  };
  
  return (
    <div className="RecommendedDialogList">
      {dialogs.map((dialog) => (
        <DialogItem
          key={dialog.id}
          dialog={dialog}
          isRecommended
          onDismiss={() => handleDismiss(dialog)}
        />
      ))}
    </div>
  );
}
```

## 八、后台管理界面

### 8.1 推广管理 API

```go
// app/bff/admin/internal/core/admin.addPromotedDialog_handler.go

type AddPromotedDialogRequest struct {
    PeerType      int      `json:"peer_type"`       // 1:user, 2:chat, 3:channel
    PeerID        int64    `json:"peer_id"`
    Priority      int      `json:"priority"`        // 优先级
    ExpiresAt     int64    `json:"expires_at"`      // 过期时间戳
    TargetUserIDs []int64  `json:"target_user_ids"` // 目标用户，空=全部
}

type PromotedDialogStats struct {
    PeerType      int     `json:"peer_type"`
    PeerID        int64   `json:"peer_id"`
    PeerTitle     string  `json:"peer_title"`
    Priority      int     `json:"priority"`
    ExpiresAt     int64   `json:"expires_at"`
    ShownCount    int64   `json:"shown_count"`
    ClickedCount  int64   `json:"clicked_count"`
    CTR           float64 `json:"ctr"`           // 点击率
    ActiveUsers   int64   `json:"active_users"`
}

func (h *AdminHandler) AddPromotedDialog(w http.ResponseWriter, r *http.Request) {
    var req AddPromotedDialogRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }
    
    // 调用 Dialog 服务
    _, err := h.dialogClient.DialogAddPromotedDialog(r.Context(),
        &dialog.TLDialogAddPromotedDialog{
            PeerType:       req.PeerType,
            PeerId:         req.PeerID,
            Priority:       int32(req.Priority),
            ExpiresAt:      req.ExpiresAt,
            TargetUserIds:  req.TargetUserIDs,
        })
    
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    
    json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (h *AdminHandler) GetPromotedDialogStats(w http.ResponseWriter, r *http.Request) {
    // 获取推广统计
    // TODO: 实现统计逻辑
    
    stats := []PromotedDialogStats{
        // ...
    }
    
    json.NewEncoder(w).Encode(stats)
}
```

## 九、性能优化

### 9.1 缓存策略

```go
// 使用 Redis 缓存推荐列表
const (
    recommendationCacheKey = "recommendation:%d"  // user_id
    recommendationCacheTTL = 6 * time.Hour
)

func (c *DialogCore) getRecommendationsFromCache(userID int64) ([]RecommendedDialog, bool) {
    key := fmt.Sprintf(recommendationCacheKey, userID)
    
    data, err := c.redis.Get(context.Background(), key).Result()
    if err != nil {
        return nil, false
    }
    
    var recommendations []RecommendedDialog
    if err := json.Unmarshal([]byte(data), &recommendations); err != nil {
        return nil, false
    }
    
    return recommendations, true
}

func (c *DialogCore) setRecommendationsToCache(userID int64, recommendations []RecommendedDialog) {
    key := fmt.Sprintf(recommendationCacheKey, userID)
    
    data, err := json.Marshal(recommendations)
    if err != nil {
        return
    }
    
    c.redis.Set(context.Background(), key, data, recommendationCacheTTL)
}
```

### 9.2 批量处理

```go
// 批量生成推荐，避免逐个计算
func (r *RecommendationAlgorithm) BatchGenerateRecommendations(ctx context.Context,
    userIDs []int64, limit int) (map[int64][]RecommendedDialog, error) {
    
    result := make(map[int64][]RecommendedDialog)
    
    // 使用 goroutine pool
    pool := workerpool.New(10)
    mu := sync.Mutex{}
    
    for _, userID := range userIDs {
        uid := userID
        pool.Submit(func() {
            recommendations, err := r.GenerateRecommendations(ctx, uid, limit)
            if err == nil {
                mu.Lock()
                result[uid] = recommendations
                mu.Unlock()
            }
        })
    }
    
    pool.StopWait()
    return result, nil
}
```

## 十、数据迁移脚本

```sql
-- migrations/20241017_add_recommendation_tables.up.sql

-- 1. 创建推荐表
CREATE TABLE IF NOT EXISTS dialog_recommendations (
    -- ... (见第三章)
);

-- 2. 创建配置表
CREATE TABLE IF NOT EXISTS recommendation_configs (
    -- ... (见第三章)
);

-- 3. 创建反馈日志表
CREATE TABLE IF NOT EXISTS recommendation_feedback_logs (
    -- ... (见第三章)
);

-- 4. 插入初始配置
INSERT INTO recommendation_configs (config_key, config_value, description, created_at, updated_at)
VALUES
    ('max_recommendations', '20', '每用户最大推荐数', EXTRACT(EPOCH FROM NOW()), EXTRACT(EPOCH FROM NOW())),
    ('recommendation_ttl_days', '7', '推荐有效期（天）', EXTRACT(EPOCH FROM NOW()), EXTRACT(EPOCH FROM NOW())),
    ('promoted_max_slots', '3', '同时展示推广对话上限', EXTRACT(EPOCH FROM NOW()), EXTRACT(EPOCH FROM NOW())),
    ('algorithm_weights', '{"frequency":0.3,"recency":0.3,"engagement":0.2,"social":0.2}', '算法权重配置', EXTRACT(EPOCH FROM NOW()), EXTRACT(EPOCH FROM NOW())),
    ('refresh_interval_hours', '6', '推荐刷新间隔（小时）', EXTRACT(EPOCH FROM NOW()), EXTRACT(EPOCH FROM NOW()));
```

## 十一、测试计划

### 11.1 单元测试

```go
// app/service/biz/dialog/internal/core/dialog_recommendations_test.go

func TestGenerateRecommendations(t *testing.T) {
    // 测试推荐算法
}

func TestCalculateScore(t *testing.T) {
    // 测试评分计算
}

func TestPromotedDialogPriority(t *testing.T) {
    // 测试推广对话优先级
}
```

### 11.2 集成测试

```go
func TestGetRecommendedDialogsFlow(t *testing.T) {
    // 1. 添加推广对话
    // 2. 生成算法推荐
    // 3. 获取推荐列表
    // 4. 验证排序和去重
}
```

### 11.3 性能测试

```bash
# 使用 k6 进行压力测试
k6 run --vus 100 --duration 30s recommendation_load_test.js
```

## 十二、上线步骤

### Phase 1: 基础架构（Week 1-2）
1. 创建数据库表和索引
2. 实现 DAO 层
3. 添加固定文件夹 ID=1
4. 单元测试

### Phase 2: 推荐算法（Week 3-4）
1. 实现基础推荐算法
2. 实现定时刷新任务
3. 缓存优化
4. 集成测试

### Phase 3: 推广功能（Week 5）
1. 实现推广对话管理
2. 后台管理接口
3. 统计分析

### Phase 4: 客户端集成（Week 6-7）
1. telegram-tt 显示"推荐"标签
2. 用户交互和反馈
3. UI/UX 优化

### Phase 5: 灰度发布（Week 8）
1. 小范围用户测试
2. 收集反馈
3. 优化迭代
4. 全量发布

## 十三、监控指标

### 13.1 业务指标
- 推荐对话点击率（CTR）
- 用户隐藏率（Dismiss Rate）
- 推荐对话置顶率
- 日活跃用户推荐覆盖率

### 13.2 技术指标
- 推荐生成耗时（P50, P99）
- 推荐列表加载耗时
- 缓存命中率
- 数据库查询耗时

### 13.3 告警规则
- 推荐生成失败率 > 5%
- 推荐列表加载耗时 > 2s
- 缓存命中率 < 80%
- 数据库连接池耗尽

## 十四、兼容性说明

### 14.1 Telegram Layer 229 兼容
- 文件夹 ID 分配不影响现有 API
- `messages.getDialogFilters` 返回值向后兼容
- 新增 API 使用新的 TL constructor

### 14.2 客户端兼容
- 旧版客户端忽略 ID=1 的文件夹
- 新版客户端识别并显示"推荐"标签
- API 版本协商机制

## 十五、安全与隐私

### 15.1 数据隐私
- 推荐算法只使用用户自己的行为数据
- 不跨用户共享推荐列表
- 支持用户关闭推荐功能

### 15.2 推广审核
- 推广对话需人工审核
- 防止垃圾推广
- 用户可举报不当推广

### 15.3 权限控制
- 后台管理接口需要管理员权限
- API 限流防止滥用
- 操作日志记录

---

## 附录

### A. TL Schema 定义

```tl
// 推荐对话列表
messages.getRecommendedDialogs#a1b2c3d4 offset_date:int offset_id:int offset_peer:InputPeer limit:int hash:long = messages.Dialogs;

// 推荐反馈
recommendationFeedbackClicked#1 = RecommendationFeedbackType;
recommendationFeedbackDismissed#2 = RecommendationFeedbackType;
recommendationFeedbackPinned#3 = RecommendationFeedbackType;

messages.reportRecommendationFeedback#c8d9e0f1 peer:InputPeer feedback_type:RecommendationFeedbackType = Bool;
```

### B. 配置示例

```yaml
# config.yaml
recommendation:
  enabled: true
  max_per_user: 20
  ttl_days: 7
  refresh_interval_hours: 6
  promoted_max_slots: 3
  algorithm:
    frequency_weight: 0.3
    recency_weight: 0.3
    engagement_weight: 0.2
    social_weight: 0.2
```

### C. 数据库索引优化

```sql
-- 针对高频查询的索引
CREATE INDEX CONCURRENTLY idx_recommendations_user_weight 
    ON dialog_recommendations(user_id, weight DESC, score DESC) 
    WHERE deleted = FALSE AND dismissed = FALSE;

-- 针对过期清理的索引
CREATE INDEX CONCURRENTLY idx_recommendations_expires 
    ON dialog_recommendations(expires_at) 
    WHERE deleted = FALSE AND expires_at > 0 AND expires_at < EXTRACT(EPOCH FROM NOW() + INTERVAL '7 days');
```

---

**文档版本**: 1.0  
**创建日期**: 2026-10-17  
**最后更新**: 2026-10-17  
**负责人**: [待填写]  
**审核人**: [待填写]
