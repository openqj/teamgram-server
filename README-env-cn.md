# 高性能组件环境使用说明（docker-compose-env.yaml）

基于 `docker-compose-env.yaml` 的环境栈，包含 PostgreSQL 18、监控、日志、消息队列和存储等组件。

> PostgreSQL 18 是本项目唯一的生产关系数据库。项目尚未上线，不提供旧 MySQL 数据迁移路径。

## 组件列表

| 组件 | 说明 |
|------|------|
| kafka | 消息队列（KRaft 模式，无需 Zookeeper） |
| etcd | 配置与发现 |
| redis | 缓存 |
| postgres | 关系数据库（PostgreSQL 18） |
| minio / minio-mc | 对象存储及初始化 bucket |
| jaeger | 分布式追踪 |
| prometheus | 指标采集 |
| node-exporter | 主机指标导出 |
| grafana | 可视化与仪表盘 |
| elasticsearch | 搜索与日志存储 |
| kibana | 日志/数据可视化 |
| filebeat | 日志采集 |
| go-stash | 日志处理管道（Kafka → Elasticsearch） |

## 前置要求

- Docker 与 Docker Compose（Compose V2：`docker compose`）
- 如需运行 Elasticsearch，建议系统 `vm.max_map_count` ≥ 262144（Linux：`sysctl -w vm.max_map_count=262144`）

## 环境变量

- 模板文件：`.env.example`
- 实际使用：复制为 `.env` 并按需修改（生产环境务必修改密码等敏感项）

```bash
cp .env.example .env
# 编辑 .env 修改 POSTGRES_*、MINIO_*、GRAFANA_* 等
```

| 变量 | 默认值 | 说明 |
|------|--------|------|
| POSTGRES_DB | teamgram | 默认数据库名 |
| POSTGRES_USER | teamgram | 应用用数据库用户 |
| POSTGRES_PASSWORD | teamgram | 应用用数据库密码 |
| MINIO_ROOT_USER | minio | MinIO 用户名 |
| MINIO_ROOT_PASSWORD | miniostorage | MinIO 密码 |
| GRAFANA_ADMIN_USER | admin | Grafana 管理员用户名 |
| GRAFANA_ADMIN_PASSWORD | admin | Grafana 管理员密码 |
| GRAFANA_ROOT_URL | http://localhost:3000 | Grafana 访问根 URL |

## 启动与停止

```bash
# 启动所有服务（后台）
docker compose -f docker-compose-env.yaml up -d

# 查看状态
docker compose -f docker-compose-env.yaml ps

# 停止
docker compose -f docker-compose-env.yaml down

# 停止（数据在 ./data/ 下，不会被删除）
docker compose -f docker-compose-env.yaml down
```

Compose 会自动读取当前目录下的 `.env`；若未配置 `.env`，将使用上述默认值。

## 服务端口与访问地址

所有服务仅绑定 `127.0.0.1`，仅本机可访问。

| 服务 | 端口 | 访问地址 |
|------|------|----------|
| Jaeger UI | 16686 | http://127.0.0.1:16686 |
| Prometheus | 9090 | http://127.0.0.1:9090 |
| Grafana | 3000 | http://127.0.0.1:3000 |
| Kibana | 5601 | http://127.0.0.1:5601 |
| MinIO API | 9000 | http://127.0.0.1:9000 |
| MinIO Console | 9001 | http://127.0.0.1:9001 |
| PostgreSQL | 5432 | 127.0.0.1:5432 |
| Redis | 6379 | 127.0.0.1:6379 |
| etcd | 2379 | http://127.0.0.1:2379 |
| Kafka | 9092 | 127.0.0.1:9092 |
| Elasticsearch | 9200 | http://127.0.0.1:9200 |
| Node Exporter | 9100 | http://127.0.0.1:9100 |

## 配置说明

以下服务依赖 `teamgramd/deploy/` 目录下的配置文件，需存在且格式正确方可正常启动：

- **Prometheus**：`teamgramd/deploy/prometheus/prometheus.yml`（抓取 Prometheus 自身与 node-exporter）
- **Filebeat**：`teamgramd/deploy/filebeat/filebeat.yml`
- **Go-Stash**：`teamgramd/deploy/go-stash/config.yaml`（需按业务配置 Kafka topic 与 Elasticsearch index）

PostgreSQL schema 由 `postgres-migrate` 服务自动执行 `teamgramd/deploy/postgres/apply.sh`，迁移脚本幂等且会校验 checksum。

## 网络

- 网络名：`teamgram_net`
- 驱动：bridge
- 子网：`172.20.0.0/16`

## 数据持久化

数据通过**本地目录**持久化到项目下的 `data/` 目录，与各服务对应关系如下：

| 服务 | 主机路径 |
|------|----------|
| kafka | `./data/kafka` |
| etcd | `./data/etcd` |
| redis | `./data/redis` |
| postgres | `./data/postgres` |
| minio | `./data/minio` |
| prometheus | `./data/prometheus` |
| grafana | `./data/grafana` |
| elasticsearch | `./data/elasticsearch` |

首次启动时 Docker Compose 会自动创建上述目录。执行 `down` 不会删除这些目录及其中数据；备份或迁移时直接复制 `data/` 即可。

## 注意事项

- **Go-Stash**：若镜像 `kevwan/go-stash` 不可用，需改为自建或其它可用镜像，并保证 `teamgramd/deploy/go-stash/config.yaml` 与运行环境一致。
- **Grafana**：首次登录使用 `.env` 中的 `GRAFANA_ADMIN_USER` / `GRAFANA_ADMIN_PASSWORD`，建议在 Grafana 中添加 Prometheus、Elasticsearch 等数据源以查看监控与日志。
