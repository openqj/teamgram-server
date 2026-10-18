# 推荐文件夹功能实施进度总结

## 2026-10-18 继续实施结果

- 已使用正式生成的 `dialog.tl.pb.go` 和 `dialog.tl_grpc.pb.go`，删除临时 recommendation stub。
- 推荐查询支持输入校验、offset、总量限制、推广优先和完整去重；展示统计只更新当前页。
- 反馈在同一个 PostgreSQL 事务中更新状态并写入 `recommendation_feedback_logs`。
- 推荐刷新从用户真实 `dialogs` 行生成候选，并保留历史展示/点击计数和用户隐藏状态。
- 推广接口在目标用户为空时使用近 7 天活跃用户；添加、移除和刷新均使用事务批量写入。
- 迁移文件为 `teamgramd/deploy/sql/postgres/104_biz_dialog_recommendations.sql`。
- 实验性 BFF 已补全 Dialog、top message、用户和群组实体 hydration，并在 `recommendation_experimental` build tag 下通过编译。

生产兼容边界：`folder_id=1` 仍是 Layer 229 归档语义，推荐暂不注入正式 `messages.getDialogFilters`。公开推荐 BFF 文件保留 `recommendation_experimental` build tag，当前不属于生产 MTProto surface。

## 项目概述

为 Teamgram 实现系统级"推荐"聊天文件夹功能，位于"全部"文件夹右侧，展示算法推荐和系统推广的对话。

## 已完成工作 ✅

### Phase 1: 数据库基础架构（100%）

✅ **数据库迁移脚本**
- 文件：[teamgramd/deploy/sql/postgres/104_biz_dialog_recommendations.sql](teamgramd/deploy/sql/postgres/104_biz_dialog_recommendations.sql)
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
- 当前候选来自用户 PostgreSQL `dialogs` 行，评分使用最近活动、top message、未读和 peer 类型信号；消息级频率和社交图仍待独立数据源。

✅ **核心业务处理器**
- 文件：[dialog_recommendations.go](app/service/biz/dialog/internal/core/dialog_recommendations.go)
- 实现了5个核心方法：
  1. `DialogGetRecommendedDialogs` - 获取推荐对话列表
  2. `DialogRefreshRecommendations` - 刷新用户推荐
  3. `DialogReportRecommendationFeedback` - 报告用户反馈
  4. `DialogAddPromotedDialog` - 添加推广对话（后台管理）
  5. `DialogRemovePromotedDialog` - 移除推广对话（后台管理）

### Phase 3: Protobuf 接口定义（已完成）

✅ **Protocol Buffers 定义**
- 文件：[dialog.tl.proto](app/service/biz/dialog/dialog/dialog.tl.proto)
- 已添加：
  - TL Constructor 常量定义
  - `RecommendedDialog` 消息类型
  - `RecommendedDialogs` 消息类型
  - 5个请求消息类型（TL_dialog_*）
  - 5个 RPC 方法声明

✅ **已完成**：已生成并编译内部 gRPC protobuf 代码；这些实验性类型未加入通用 MTProto codec 注册。

## 待完成工作 ⏳

### Phase 4: BFF 层与兼容性边界

⚠️ **正式 messages.getDialogFilters 不修改**
- Layer 229 已将 `folder_id=1` 用于归档，生产路径不能复用该 ID。
- 推荐文件夹暂通过内部 Dialog RPC 提供，公开 BFF 文件保留实验 build tag。

⚠️ **实验性 messages.getRecommendedDialogs**
- 文件已创建：`app/bff/messages/internal/core/messages.getRecommendedDialogs_handler.go`
- 已从 Dialog 服务获取推荐列表，并通过现有 DialogExt 流程补齐 User/Chat/Message 返回对象。

⚠️ **实验性 messages.reportRecommendationFeedback**
- 文件已创建：`app/bff/messages/internal/core/messages.reportRecommendationFeedback_handler.go`
- 已接收反馈并调用 Dialog 服务；仍需独立实验 schema 承载，不能加入正式 Layer 229 surface。

### Phase 5: 定时任务

✅ **推荐刷新定时任务**
- 每6小时刷新一次用户推荐
- 文件已创建；需由部署调度器注册

✅ **过期推荐清理任务**
- 每天清理过期推荐
- 文件已创建；需由部署调度器注册

### Phase 7: 客户端适配（未开始）

❌ **telegram-tt 集成**
- 类型定义
- API 调用
- UI 组件

### Phase 8: 测试

⚠️ **单元测试**
- 算法和 DAO 测试文件已添加；依赖 PostgreSQL 或 mock 的用例当前跳过。
- `go test ./app/service/biz/dialog/...` 已通过。

⏳ **集成测试**
- 端到端流程测试仍待 PostgreSQL 测试环境。

## 文件清单

### 已创建文件

**数据库**
- `teamgramd/deploy/sql/postgres/104_biz_dialog_recommendations.sql`

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

### 已创建文件（部分仍待接入）

**BFF 层**
- `app/bff/messages/internal/core/messages.getRecommendedDialogs_handler.go`
- `app/bff/messages/internal/core/messages.reportRecommendationFeedback_handler.go`
- `app/bff/messages/internal/core/recommendation_experimental_types.go` - 仅用于实验 build 的本地请求适配器

**定时任务**
- `app/service/biz/dialog/internal/jobs/recommendation_refresh_job.go`
- `app/service/biz/dialog/internal/jobs/recommendation_cleanup_job.go`

**测试文件**
- `app/service/biz/dialog/internal/core/dialog_recommendations_test.go`
- `app/service/biz/dialog/internal/core/recommendation_algorithm_test.go`
- `app/service/biz/dialog/internal/dal/dao/postgres_dao/dialog_recommendations_dao_test.go`

## 下一步操作指南

### 立即执行

1. **验证内部 protobuf 和 Dialog 服务编译**
   ```bash
   go test ./app/service/biz/dialog/...
   ```

### 后续实施步骤

2. **运行数据库迁移**
   ```bash
   psql -d teamgram -f teamgramd/deploy/sql/postgres/104_biz_dialog_recommendations.sql
   ```

3. **完善实验性 BFF 层**
   - 保持 `recommendation_experimental` 下的独立请求类型和路由接入
   - 保持正式 `messages.getDialogFilters` 的 Layer 229 归档语义

4. **注册定时任务**
   - 接入推荐刷新任务（6小时间隔）
   - 接入过期清理任务（每天）

5. **完善推荐算法**
   - 集成消息服务获取真实的对话频率数据
   - 集成用户服务获取社交关系数据
   - 调优权重配置

6. **编写测试**
   - 单元测试覆盖核心逻辑
   - 集成测试验证完整流程

7. **部署和监控**
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

1. **推荐算法信号有限**：当前使用 `dialogs` 行的活动时间、top message、未读和 peer 类型信号；消息级频率和社交图仍待独立数据源
2. **BFF 层**：公开入口保留实验 build tag，未进入正式 Layer 229 schema

## 参考文档

- [完整设计文档](RECOMMENDED_FOLDER_DESIGN.md)
- [Telegram Layer 229 文档](https://core.telegram.org/schema)
- [PostgreSQL 文档](https://www.postgresql.org/docs/18/)

## 版本历史

- **2026-10-17**: Phase 1-3 完成，基础架构和核心逻辑实现
- **2026-10-18**: 实验性 BFF hydration 完成，默认和实验 build 均完成编译验证

---

**当前状态**: ✅ 内部 PostgreSQL/Dialog 能力已实现并通过定向编译验证
**完成度**: 核心能力和实验性 BFF hydration 已完成，生产 BFF/客户端仍受 Layer 229 兼容边界限制
**预计剩余工作量**: 取决于独立实验 schema、调度注册、消息统计和社交数据源
