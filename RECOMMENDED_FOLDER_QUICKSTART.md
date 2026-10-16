# 推荐文件夹功能 - 快速启动指南

## 当前状态

✅ **已完成的基础架构**（约 60%）：
- 数据库表结构和 DAO 层
- 核心业务逻辑和推荐算法
- Protobuf 接口定义

⚠️ **待完成**：
- Protobuf 代码生成
- BFF 层实现
- 集成测试

## 立即可执行的步骤

### 1. 运行数据库迁移

```bash
# 连接到 PostgreSQL 数据库
psql -U postgres -d teamgram

# 执行迁移脚本
\i teamgramd/deploy/sql/postgres/103_dialog_recommendations.sql

# 验证表已创建
\dt dialog_recommendations
\dt recommendation_configs
\dt recommendation_feedback_logs

# 查看初始配置
SELECT * FROM recommendation_configs;
```

### 2. 生成 Protobuf 代码

项目使用自定义的 protobuf 工具链。需要找到正确的生成命令：

```bash
# 方法 1: 查找项目中的生成脚本
find . -name "*.sh" | xargs grep -l "mtprotoc\|protoc"

# 方法 2: 查看其他服务如何生成
ls -la app/service/biz/message/message/*.pb.go

# 方法 3: 使用 go generate（如果配置了）
cd app/service/biz/dialog
go generate ./...

# 方法 4: 查看 Makefile
find . -name Makefile | xargs grep -i proto
```

**临时解决方案**：注释掉依赖 protobuf 生成代码的部分，先测试其他组件。

### 3. 测试 DAO 层

创建单元测试验证数据库操作：

```bash
cd app/service/biz/dialog/internal/dal/dao/postgres_dao

# 运行 DAO 测试
go test -v -run TestDialogRecommendations

# 测试配置 DAO
go test -v -run TestRecommendationConfigs
```

### 4. 添加测试推荐数据

```sql
-- 为测试用户添加推荐
INSERT INTO dialog_recommendations 
  (user_id, peer_type, peer_id, recommendation_type, weight, score, 
   recommended_at, expires_at, last_updated)
VALUES 
  (12345, 1, 67890, 0, 100, 0.8, 
   EXTRACT(EPOCH FROM NOW())::BIGINT, 
   EXTRACT(EPOCH FROM NOW() + INTERVAL '7 days')::BIGINT,
   EXTRACT(EPOCH FROM NOW())::BIGINT);

-- 添加推广对话
INSERT INTO dialog_recommendations 
  (user_id, peer_type, peer_id, recommendation_type, weight, score,
   is_promoted, promoted_priority, promoted_until,
   recommended_at, expires_at, last_updated)
VALUES 
  (12345, 3, 11111, 1, 1010, 1.0,
   TRUE, 10, EXTRACT(EPOCH FROM NOW() + INTERVAL '30 days')::BIGINT,
   EXTRACT(EPOCH FROM NOW())::BIGINT,
   EXTRACT(EPOCH FROM NOW() + INTERVAL '30 days')::BIGINT,
   EXTRACT(EPOCH FROM NOW())::BIGINT);

-- 验证数据
SELECT * FROM dialog_recommendations WHERE user_id = 12345;
```

## 后续实施步骤

### Step 1: 完成 Protobuf 生成（优先级：最高）

一旦找到正确的生成命令：

```bash
# 重新生成 dialog service 的 protobuf 代码
cd app/service/biz/dialog
[proto-gen-command]

# 验证生成的文件
ls -la dialog/*.pb.go
grep -r "RecommendedDialog" dialog/*.pb.go
```

### Step 2: 实现 BFF 层 Handler

创建以下文件：

1. **messages.getRecommendedDialogs_handler.go**
```go
package core

import (
    "context"
    "github.com/teamgram/proto/mtproto"
)

// MessagesGetRecommendedDialogs
// messages.getRecommendedDialogs#a1b2c3d4 offset_date:int offset_id:int 
//   offset_peer:InputPeer limit:int hash:long = messages.Dialogs;
func (c *MessagesCore) MessagesGetRecommendedDialogs(ctx context.Context, 
    in *mtproto.TLMessagesGetRecommendedDialogs) (*mtproto.Messages_Dialogs, error) {
    
    // TODO: 实现获取推荐对话逻辑
    return nil, nil
}
```

2. **修改 messages.getDialogFilters_handler.go**
```go
// 在返回结果中添加固定的"推荐"文件夹（id=1）
result.Filters = append(result.Filters, &mtproto.DialogFilter{
    Constructor: mtproto.CRC32_dialogFilterDefault,
    Data2: &mtproto.DialogFilter_Data{
        Id:    1,
        Title: "Recommended",
        // 其他必要字段
    },
})
```

### Step 3: 创建定时任务

```bash
# 创建推荐刷新任务
vi app/service/biz/dialog/internal/jobs/recommendation_refresh_job.go

# 创建过期清理任务
vi app/service/biz/dialog/internal/jobs/recommendation_cleanup_job.go
```

### Step 4: 集成测试

```bash
# 启动服务
cd teamgramd
go run cmd/biz/dialog/main.go

# 测试推荐 API
# 使用 grpcurl 或编写测试客户端
```

## 调试清单

### 数据库连接检查
```sql
-- 检查连接
\conninfo

-- 检查表结构
\d dialog_recommendations

-- 检查索引
\di dialog_recommendations*
```

### Go 编译检查
```bash
# 检查 dialog service 编译
go build ./app/service/biz/dialog/cmd/...

# 检查依赖
go mod tidy
go mod verify
```

### 运行时检查
```bash
# 查看日志
tail -f logs/dialog.log

# 检查数据库查询
# 在 PostgreSQL 中开启查询日志
ALTER SYSTEM SET log_statement = 'all';
SELECT pg_reload_conf();
```

## 配置参数

### 推荐系统配置（可在数据库中调整）

```sql
-- 调整推荐刷新间隔（小时）
UPDATE recommendation_configs 
SET config_value = '12' 
WHERE config_key = 'refresh_interval_hours';

-- 调整最大推荐数
UPDATE recommendation_configs 
SET config_value = '30' 
WHERE config_key = 'max_recommendations';

-- 调整算法权重
UPDATE recommendation_configs 
SET config_value = '{"frequency":0.4,"recency":0.3,"engagement":0.2,"social":0.1}' 
WHERE config_key = 'algorithm_weights';
```

## 性能优化建议

### 数据库索引
```sql
-- 如果查询慢，可以添加额外的复合索引
CREATE INDEX idx_recommendations_user_weight_score 
ON dialog_recommendations(user_id, weight DESC, score DESC) 
WHERE deleted = FALSE AND dismissed = FALSE;

-- 为时间范围查询优化
CREATE INDEX idx_recommendations_expires_range 
ON dialog_recommendations(expires_at) 
WHERE deleted = FALSE AND expires_at > 0;
```

### 缓存策略
- 使用 Redis 缓存用户推荐列表（TTL 6小时）
- 缓存推广对话列表（TTL 1小时）

## 故障排查

### 问题 1: Protobuf 编译失败
```bash
# 检查 protoc 版本
protoc --version

# 检查 Go protobuf 插件
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
```

### 问题 2: 数据库连接失败
```bash
# 检查 PostgreSQL 是否运行
pg_isready

# 检查配置文件
cat app/service/biz/dialog/etc/dialog.yaml | grep -i postgres
```

### 问题 3: 推荐列表为空
```sql
-- 检查是否有推荐数据
SELECT COUNT(*) FROM dialog_recommendations WHERE deleted = FALSE;

-- 检查用户的推荐
SELECT * FROM dialog_recommendations 
WHERE user_id = [YOUR_USER_ID] AND deleted = FALSE;

-- 检查是否过期
SELECT * FROM dialog_recommendations 
WHERE expires_at > 0 AND expires_at < EXTRACT(EPOCH FROM NOW())::BIGINT;
```

## 监控指标

### 关键指标
- 推荐列表请求 QPS
- 推荐生成耗时（P50, P99）
- 用户点击率（CTR）
- 用户隐藏率
- 数据库查询耗时

### 监控查询
```sql
-- 推荐统计
SELECT 
    recommendation_type,
    COUNT(*) as total,
    SUM(shown_count) as total_shown,
    SUM(clicked_count) as total_clicked,
    ROUND(SUM(clicked_count)::NUMERIC / NULLIF(SUM(shown_count), 0) * 100, 2) as ctr_percent
FROM dialog_recommendations
WHERE deleted = FALSE
GROUP BY recommendation_type;

-- 推广效果统计
SELECT 
    peer_type,
    peer_id,
    COUNT(DISTINCT user_id) as target_users,
    SUM(shown_count) as total_shown,
    SUM(clicked_count) as total_clicked
FROM dialog_recommendations
WHERE is_promoted = TRUE AND deleted = FALSE
GROUP BY peer_type, peer_id
ORDER BY total_clicked DESC;
```

## 联系与支持

- 设计文档: [RECOMMENDED_FOLDER_DESIGN.md](RECOMMENDED_FOLDER_DESIGN.md)
- 实施状态: [RECOMMENDED_FOLDER_IMPLEMENTATION_STATUS.md](RECOMMENDED_FOLDER_IMPLEMENTATION_STATUS.md)
- 问题追踪: 在项目 Issues 中创建

## 附录

### 有用的 SQL 查询

```sql
-- 清空测试数据
DELETE FROM dialog_recommendations WHERE user_id IN (12345, 67890);
DELETE FROM recommendation_feedback_logs WHERE user_id IN (12345, 67890);

-- 重置配置为默认值
UPDATE recommendation_configs SET config_value = '20' 
WHERE config_key = 'max_recommendations';

-- 导出推荐数据
\copy (SELECT * FROM dialog_recommendations) TO 'recommendations.csv' CSV HEADER;

-- 备份表
CREATE TABLE dialog_recommendations_backup AS 
SELECT * FROM dialog_recommendations;
```

### Go 测试命令

```bash
# 运行所有测试
go test ./app/service/biz/dialog/... -v

# 运行特定测试
go test -v -run TestDialogGetRecommendedDialogs

# 生成测试覆盖率报告
go test -coverprofile=coverage.out ./app/service/biz/dialog/...
go tool cover -html=coverage.out
```

---

**最后更新**: 2026-10-17  
**当前阶段**: 基础架构完成，待 Protobuf 生成  
**下一里程碑**: BFF 层实现和集成测试
