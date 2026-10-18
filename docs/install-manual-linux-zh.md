# Teamgram Server：Linux 源码安装

本文是 PostgreSQL 18-only 的 Linux 手动安装路径，适用于开发环境或本地
单节点部署。完整依赖栈建议使用 Docker。

## 环境要求

- 带 `systemd` 和 `curl` 的 Linux
- Go 1.25 或更高版本
- PostgreSQL 18 与 `psql` 客户端
- Redis 7、etcd 3.5、Kafka 3.x（KRaft 模式）
- SeaweedFS S3（或其他 S3 兼容对象存储）和 FFmpeg

持久化状态仅支持 PostgreSQL 18 的 `5432` 端口。

## 安装依赖

使用发行版包管理器安装所需工具，并通过 `systemctl` 启用 PostgreSQL、Redis、
etcd、Kafka、SeaweedFS 和 FFmpeg。确认 PostgreSQL 主版本为 18 且监听 `5432`：

```sh
psql --version
pg_isready -h 127.0.0.1 -p 5432
```

按 PostgreSQL 的管理流程创建 `teamgram` 数据库和应用角色。应用 DSN 不应使用
超级用户：

```sh
export DATABASE_URL='postgresql://teamgram:change-me@127.0.0.1:5432/teamgram?sslmode=disable'
```

## 应用迁移

在仓库根目录执行：

```sh
./teamgramd/deploy/postgres/check.sh
DATABASE_URL="$DATABASE_URL" ./teamgramd/deploy/postgres/apply.sh
```

runner 按文件名顺序应用 `teamgramd/deploy/sql/postgres/*.sql`，在
`schema_migrations` 保存校验和，并拒绝被修改过的迁移。该目录之外的 SQL
仅为历史归档，不是安装输入。

## 配置并运行

在 `teamgramd/etc/`（或 Docker 使用的 `teamgramd/etc2/`）为每个持久化服务
配置 PostgreSQL DSN，同时配置 Redis、etcd、Kafka、S3 endpoint 和 FFmpeg 地址。
PostgreSQL DSN 缺失或不可达时必须停止启动。

```sh
make
./teamgramd/bin/runall2.sh
```

`gnetway` 通过 TCP `10443`/`5222` 和 WebSocket `11443` 暴露 MTProto。内部
端口和发现 key 见[服务拓扑](service-topology-zh.md)。

## 检查

```sh
git diff --check
./teamgramd/deploy/postgres/check.sh
docker compose -f docker-compose.yaml config
docker compose -f docker-compose-env.yaml config
```

迭代时使用受影响包的定向编译，最后只执行一次 `go build ./...` 作为总集成门禁。
英文说明见 [install-manual-linux.md](install-manual-linux.md)。
