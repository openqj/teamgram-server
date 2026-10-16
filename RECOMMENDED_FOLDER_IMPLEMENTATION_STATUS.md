# 推荐文件夹功能实施进度总结

## 项目概述

为 Teamgram 实现系统级"推荐"聊天文件夹功能，位于"全部"文件夹右侧，展示算法推荐和系统推广的对话。

## 已完成工作 ✅

### Phase 1: 数据库基础架构（100%）

✅ **数据库迁移脚本**
- 文件：[teamgramd/deploy/sql/postgres/103_dialog_recommendations.sql](teamgramd/deploy/sql/postgres/103_dialog_recommendations.sql)
- 创建了3个表：
  - `dialog_recommendations` - 推荐对话关系表
  - `recommendation_configs` - 推荐系统配置表
  - `recommendation_feedback_logs` - 推荐反馈日志表
- 所有索引已优化完成
- 初始配置数据已准备

✅ **数据对象层（DO）**
- [dialog_recommendations_do.go](app/service/biz/dialog/internal/dal/dataobject/dialog_recommendations_do.go)
- [recommendation_configs_do.go](app/service/biz/dialog/internal/dal/dataobject/recommendation_configs_do.go)

✅ **数据访问层（DAO）**
- [dialog_recommendations_dao.go](app/service/biz/dialog/internal/dal/dao/postgres_dao/dialog_recommendations_dao.go)
  - `InsertOrUpdate` - 插入或更新推荐
  - `SelectActiveList` - 获取活跃推荐列表
  - `SelectPromotedList` - 获取推广对话列表
  - `MarkDismissed` - 标记为已隐藏
  - `UpdateStats` - 更新统计
  - `DeleteExpired` - 清理过期推荐
  - `BatchInsert` - 批量插入
- [recommendation_configs_dao.go](app/service/biz/dialog/internal/dal/dao/postgres_dao/recommendation_configs_dao.go)
  - `SelectByKey` - 根据键获取配置
  - `SelectAll` - 获取所有配置
  - `UpdateValue` - 更新配置值

✅ **Store 结构更新**
- 已更新 [db.go](app/service/biz/dialog/internal/dal/dao/postgres_dao/db.go)
- 添加了 `DialogRecommendations` 和 `RecommendationConfigs` 到 Store

### Phase 2: 核心业务逻辑（100%）

✅ **推荐算法引擎**
- 文件：[recommendation_algorithm.go](app/service/biz/dialog/internal/core/recommendation_algorithm.go)
- 实现了评分算法框架：
  - `CalculateScore` - 综合评分计算
  - `calculateFrequencyScore` - 对话频率评分
  - `calculateRecencyScore` - 最近活跃度评分
  - `calculateEngagementScore` - 互动程度评分
  - `calculateSocialScore` - 社交关系评分
  - `GenerateRecommendations` - 生成推荐列表
- 注：当前返回模拟值，需要后续集成消息服务

✅ **核心业务处理器**
- 文件：[dialog_recommendations.go](app/service/biz/dialog/internal/core/dialog_recommendations.go)
- 实现了5个核心方法：
  1. `DialogGetRecommendedDialogs` - 获取推荐对话列表
  2. `DialogRefreshRecommendations` - 刷新用户推荐
  3. `DialogReportRecommendationFeedback` - 报告用户反馈
  4. `DialogAddPromotedDialog` - 添加推广对话（后台管理）
  5. `DialogRemovePromotedDialog` - 移除推广对话（后台管理）

### Phase 3: Protobuf 接口定义（90%）

✅ **Protocol Buffers 定义**
- 文件：[dialog.tl.proto](app/service/biz/dialog/dialog/dialog.tl.proto)
- 已添加：
  - TL Constructor 常量定义
  - `RecommendedDialog` 消息类型
  - `RecommendedDialogs` 消息类型
  - 5个请求消息类型（TL_dialog_*）
  - 5个 RPC 方法声明

⚠️ **待完成**：需要运行 protobuf 编译器生成 Go 代码

## 待完成工作 ⏳

### Phase 4: Protobuf 代码生成（优先级：高）

❌ **生成 Go 代码**
```bash
# 需要运行 protobuf 编译命令
# 项目可能有自己的编译脚本，需要查找并执行
```

**当前问题**：
- 核心处理器代码中引用的 protobuf 类型尚未生成
- 编译错误：`undefined: dialog.TLDialogGetRecommendedDialogs` 等

### Phase 5: BFF 层实现（未开始）

❌ **修改 messages.getDialogFilters**
- 位置：`app/bff/messages/internal/core/messages.getDialogFilters_handler.go`
- 任务：
  - 始终返回 `dialog_filter_id = 0`（全部）
  - 始终返回 `dialog_filter_id = 1`（推荐）
  - 标记为系统文件夹，不可删除、不可编辑

❌ **新增 messages.getRecommendedDialogs**
- 位置：`app/bff/messages/internal/core/messages.getRecommendedDialogs_handler.go`
- 任务：
  - 从 Dialog 服务获取推荐列表
  - 构建对话列表并返回

❌ **新增 messages.reportRecommendationFeedback**
- 位置：`app/bff/messages/internal/core/messages.reportRecommendationFeedback_handler.go`
- 任务：
  - 接收用户反馈（点击、隐藏、置顶）
  - 调用 Dialog 服务更新统计

### Phase 6: 定时任务（未开始）

❌ **推荐刷新定时任务**
- 每6小时刷新一次用户推荐
- 需要创建 job 文件

❌ **过期推荐清理任务**
- 每天清理过期推荐
- 需要创建 job 文件

### Phase 7: 客户端适配（未开始）

❌ **telegram-tt 集成**
- 类型定义
- API 调用
- UI 组件

### Phase 8: 测试（未开始）

❌ **单元测试**
- DAO 层测试
- 算法测试
- 核心业务逻辑测试

❌ **集成测试**
- 端到端流程测试

## 文件清单

### 已创建文件

**数据库**
- `teamgramd/deploy/sql/postgres/103_dialog_recommendations.sql`

**数据对象层**
- `app/service/biz/dialog/internal/dal/dataobject/dialog_recommendations_do.go`
- `app/service/biz/dialog/internal/dal/dataobject/recommendation_configs_do.go`

**DAO 层**
- `app/service/biz/dialog/internal/dal/dao/postgres_dao/dialog_recommendations_dao.go`
- `app/service/biz/dialog/internal/dal/dao/postgres_dao/recommendation_configs_dao.go`

**核心业务层**
- `app/service/biz/dialog/internal/core/recommendation_algorithm.go`
- `app/service/biz/dialog/internal/core/dialog_recommendations.go`

**修改的文件**
- `app/service/biz/dialog/internal/dal/dao/postgres_dao/db.go` - 添加新 DAO 到 Store
- `app/service/biz/dialog/dialog/dialog.tl.proto` - 添加推荐相关的 protobuf 定义

### 待创建文件

**BFF 层**
- `app/bff/messages/internal/core/messages.getRecommendedDialogs_handler.go`
- `app/bff/messages/internal/core/messages.reportRecommendationFeedback_handler.go`

**定时任务**
- `app/service/biz/dialog/internal/jobs/recommendation_refresh_job.go`
- `app/service/biz/dialog/internal/jobs/recommendation_cleanup_job.go`

**测试文件**
- `app/service/biz/dialog/internal/core/dialog_recommendations_test.go`
- `app/service/biz/dialog/internal/core/recommendation_algorithm_test.go`
- `app/service/biz/dialog/internal/dal/dao/postgres_dao/dialog_recommendations_dao_test.go`

## 下一步操作指南

### 立即执行（解决编译问题）

1. **生成 protobuf 代码**
   ```bash
   # 查找项目的 protobuf 编译脚本
   find . -name "*.sh" -o -name "Makefile" | xargs grep -l "protoc"
   
   # 或者手动运行
   cd app/service/biz/dialog
   protoc --go_out=. --go-grpc_out=. dialog/dialog.tl.proto
   ```

2. **验证编译**
   ```bash
   go build ./app/service/biz/dialog/...
   ```

### 后续实施步骤

3. **运行数据库迁移**
   ```bash
   psql -d teamgram -f teamgramd/deploy/sql/postgres/103_dialog_recommendations.sql
   ```

4. **实现 BFF 层**
   - 修改 `messages.getDialogFilters` 添加固定的"推荐"文件夹
   - 创建 `messages.getRecommendedDialogs` handler
   - 创建 `messages.reportRecommendationFeedback` handler

5. **实现定时任务**
   - 推荐刷新任务（6小时间隔）
   - 过期清理任务（每天）

6. **完善推荐算法**
   - 集成消息服务获取真实的对话频率数据
   - 集成用户服务获取社交关系数据
   - 调优权重配置

7. **编写测试**
   - 单元测试覆盖核心逻辑
   - 集成测试验证完整流程

8. **部署和监控**
   - 部署到测试环境
   - 配置监控指标
   - 灰度发布

## 技术决策记录

### 文件夹 ID 分配

```
dialog_filter_id = 0  → "全部" (All Chats)
dialog_filter_id = 1  → "推荐" (Recommended) - 新增
dialog_filter_id ≥ 2  → 用户自定义文件夹
```

### 推荐来源类型

```go
recommendation_type = 0  → 算法推荐
recommendation_type = 1  → 系统置顶/推广
recommendation_type = 2  → 热门对话
recommendation_type = 3  → 新用户引导
```

### 反馈类型

```go
feedback_type = 1  → clicked (点击)
feedback_type = 2  → dismissed (隐藏)
feedback_type = 3  → pinned (置顶)
```

## 依赖项

- PostgreSQL 18+
- Protocol Buffers 编译器
- Go 1.19+
- telegram-tt 客户端（用于前端集成）

## 已知问题

1. **编译错误**：protobuf 类型未生成，需要运行代码生成工具
2. **推荐算法**：当前返回模拟数据，需要集成真实的消息和用户服务
3. **BFF 层**：尚未实现 MTProto API handlers

## 参考文档

- [完整设计文档](RECOMMENDED_FOLDER_DESIGN.md)
- [Telegram Layer 229 文档](https://core.telegram.org/schema)
- [PostgreSQL 文档](https://www.postgresql.org/docs/18/)

## 版本历史

- **2026-10-17**: Phase 1-3 完成，基础架构和核心逻辑实现
- **待定**: Phase 4-8 实施

---

**当前状态**: ⚠️ 待 protobuf 代码生成后可继续  
**完成度**: 约 60%（基础架构和核心逻辑已完成）  
**预计剩余工作量**: 2-3天（包括 BFF 层、定时任务、测试）
