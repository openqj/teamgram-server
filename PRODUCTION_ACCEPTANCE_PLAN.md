# Teamgram-Server 生产验收总控方案

**验收日期**: 2026-10-17  
**主控**: Claude (Opus 5.5)  
**执行代理**: 本地 codex CLI  
**目标**: 完整生产就绪验收，避免高额费用

---

## 一、验收范围定义

### 1.1 核心边界
- ✅ **包含**: PostgreSQL 18 数据持久化、Layer 229 MTProto 兼容性、Bot API 合约
- ✅ **包含**: 消息投递完整性、会话恢复、多设备同步
- ⚠️ **部分**: 性能 SLO（已知未达标，记录当前基线）
- ❌ **排除**: MySQL 5.7 遗留路径、未实现的 85 个 Bot API 方法

### 1.2 验收标准
1. **功能正确性**: 关键路径无数据丢失/重复/乱序
2. **协议兼容性**: telegram-tt 本地客户端可正常使用核心功能
3. **持久化完整性**: 进程重启后状态可恢复
4. **可观测性**: Prometheus 指标暴露关键路径健康度
5. **文档齐全**: 部署步骤、环境变量、已知限制清晰记录

---

## 二、验收任务分解

### Phase 1: 环境与依赖验证 (codex)
**目标**: 确认开发/生产环境可启动并健康运行

- [ ] 1.1 验证 Docker Compose 配置完整性
  ```bash
  # codex 执行
  docker compose -f docker-compose.yaml config
  ```
- [ ] 1.2 PostgreSQL 18 迁移完整性检查
  ```bash
  # 检查所有迁移文件编号连续性
  ls -1 teamgramd/deploy/sql/postgres/*.sql | wc -l
  ```
- [ ] 1.3 启动最小依赖栈（PostgreSQL + Redis + MinIO）
  ```bash
  docker compose up -d postgres redis minio
  docker compose ps
  ```
- [ ] 1.4 健康检查通过验证
  ```bash
  # 等待 30 秒后检查
  docker compose ps --format json | jq '.[] | {name: .Name, health: .Health}'
  ```

**产出**: `validation-report-phase1.json`

---

### Phase 2: 核心服务启动验证 (codex)
**目标**: 后端服务正常启动并暴露指标

- [ ] 2.1 BFF/Session/Sync 服务启动
  ```bash
  docker compose up -d
  ```
- [ ] 2.2 Prometheus 指标端点可达性
  ```bash
  curl -s http://localhost:9090/metrics | grep -c "^teamgram_"
  ```
- [ ] 2.3 日志无启动致命错误
  ```bash
  docker compose logs --tail=100 | grep -i "fatal\|panic\|error" > startup-errors.log
  ```

**产出**: `validation-report-phase2.json`

---

### Phase 3: Layer 229 协议验收 (telegram-tt)
**目标**: 真实客户端可完成核心流程

#### 3.1 认证流程验收
- [ ] 用户注册 (auth.sendCode → auth.signUp)
- [ ] 用户登录 (auth.sendCode → auth.signIn)
- [ ] 会话持久化（客户端重启后自动恢复）

#### 3.2 消息流验收
- [ ] 发送文本消息到私聊
- [ ] 发送文本消息到群组
- [ ] 接收实时消息推送
- [ ] 历史消息拉取

#### 3.3 多设备同步验收
- [ ] 同一用户两个会话同时在线
- [ ] 一个会话发送消息，另一个实时收到
- [ ] 已读状态同步

**执行方式**: 
```bash
# telegram-tt 已配置本地运行（见 memory/telegram-tt-local-run.md）
cd /path/to/telegram-tt
npm run dev:api  # 端口 1234
# 手动操作客户端，记录关键步骤截图
```

**产出**: `validation-report-phase3-layer229.md` + 截图证据

---

### Phase 4: Bot API 合约验收 (bot-api-demo)
**目标**: 已实现的 100 个 Bot API 方法符合 Telegram 合约

- [ ] 4.1 BotFather 核心流程
  ```bash
  cd tools/bot-api-demo
  BOT_TOKEN='...' ./bot-api-e2e.sh -once
  ```
- [ ] 4.2 消息发送/接收/编辑
- [ ] 4.3 Webhook 订阅与推送
- [ ] 4.4 Inline Query 响应

**产出**: `validation-report-phase4-botapi.json`

---

### Phase 5: 持久化完整性验收 (Python 探针)
**目标**: 进程重启不丢失状态

使用现有工具：
- `tools/probe-backend-restart.ts` - 后端重启恢复验证
- `tools/messaging_reliability_probe.py` - 消息可靠性探针

**关键场景**:
1. 发送消息 → 停止后端 → 启动后端 → 验证消息已持久化
2. 建立会话 → 重启 session 服务 → 客户端自动重连
3. Outbox 未投递消息 → 重启 sync 服务 → 投递完成

**产出**: `validation-report-phase5-persistence.json`

---

### Phase 6: 性能基线记录 (非阻塞)
**目标**: 记录当前性能表现，不作为验收门槛

使用现有工具：
- `tools/messaging_group_channel_fanout_probe.py`
- `tools/messaging_capacity_probe.py`

**记录指标**:
- 1000 用户 channel fan-out P50/P95/P99
- ACK 延迟
- PostgreSQL 连接池使用率
- Kafka consumer lag

**产出**: `performance-baseline-2026-10-17.json`

---

### Phase 7: 文档完整性审查 (人工)
**检查清单**:
- [ ] README.md 包含快速启动步骤
- [ ] LAYER229_FUNCTIONAL_AUDIT.md 标注已实现/未实现边界
- [ ] BOT_API_COMPATIBILITY.md 列出 501 响应的未实现方法
- [ ] .env.example 包含所有必需环境变量
- [ ] 已知限制清晰记录（如：parse_mode 未实现）

**产出**: `documentation-checklist.md`

---

## 三、验收阻塞与降级策略

### 阻塞条件（必须修复）
1. ❌ 核心服务无法启动
2. ❌ 数据库迁移失败
3. ❌ 消息发送后持久化丢失
4. ❌ 进程重启后会话无法恢复

### 降级放行条件（记录但不阻塞）
1. ⚠️ 性能未达 2 秒 SLO（已知问题）
2. ⚠️ 85 个 Bot API 方法返回 501（按设计）
3. ⚠️ Webhook 并发度未达 `max_connections`（已知限制）

---

## 四、执行时间线

| 阶段 | 预计时长 | 执行者 | 依赖 |
|------|---------|--------|------|
| Phase 1 | 15 min | codex | Docker 环境 |
| Phase 2 | 20 min | codex | Phase 1 |
| Phase 3 | 60 min | 人工 + telegram-tt | Phase 2 |
| Phase 4 | 30 min | codex | Phase 2 |
| Phase 5 | 45 min | codex | Phase 2 |
| Phase 6 | 90 min | codex (非阻塞) | Phase 2 |
| Phase 7 | 30 min | 人工审查 | - |

**总预计**: 约 4.5 小时（Phase 6 可并行）

---

## 五、产出物清单

1. ✅ 各阶段验收报告 JSON/Markdown
2. ✅ 性能基线数据（非阻塞）
3. ✅ 文档完整性检查清单
4. ✅ **生产就绪决策报告** (`PRODUCTION_READINESS_DECISION.md`)

---

## 六、下一步行动

**立即开始**: Phase 1 环境验证

```bash
# 由本地 codex CLI 执行
cd /Users/mac/open/test/teamgram-server
./validation-scripts/phase1-environment.sh
```

---

**验收负责人**: Claude Opus 5.5  
**执行日期**: 2026-10-17  
**预计完成**: 今日内
