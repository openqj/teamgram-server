# 生产验收执行计划（离线验证模式）

**日期**: 2026-10-17  
**模式**: 代码审查 + 文档验证 + 现有探针证据分析  
**目标**: 生成生产就绪决策报告

---

## Phase 2: 代码完整性审查

### 2.1 核心服务启动代码检查
- [ ] Main entry point 验证
- [ ] 配置加载机制
- [ ] PostgreSQL 连接池初始化
- [ ] 服务健康检查端点

### 2.2 关键路径代码验证
- [ ] 认证流程完整性
- [ ] 消息持久化路径
- [ ] Sync 更新投递逻辑

---

## Phase 3: Layer 229 协议合规验证

基于现有证据：
- LAYER229_METHOD_LEDGER.csv 详细记录了每个方法的 audit_status
- BOT_API_COMPATIBILITY.md 明确标注实现覆盖率
- docker/production/ 中 170+ 个探针文件

**验收标准**:
- 已实现方法的 RPC 路由正确
- 错误响应遵循 Telegram 格式
- 构造器 ID 与 Layer 229 匹配

---

## Phase 4: Bot API 合约验证

**检查项**:
- getMe 返回正确用户信息
- 消息发送参数验证
- Webhook 配置持久化
- 已实现方法返回 ok:true，未实现返回 501

---

## Phase 5: 持久化完整性

检查文件：
- teamgramd/deploy/sql/postgres/ - 99 个迁移文件
- app/*/internal/persist/ - PostgreSQL DAO 实现
- 事务边界和重试逻辑

---

## Phase 6: 性能基线（现有数据）

使用现有报告：
- PERFORMANCE_OPTIMIZATION_SUMMARY.md
- /tmp/teamgram-group-channel-fanout-*.json（已有 5 份隔离测试）

---

## Phase 7: 文档完整性审查

检查项：
- README.md 快速启动步骤
- AGENTS.md 兼容性要求
- 已知限制清晰记录
- 环境变量文档

