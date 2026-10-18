# 对象存储

Teamgram DFS 只使用标准 S3 API。配置里的历史字段名 `Minio` 和 Go 包名
`minio-go` 只是兼容名称，默认部署已经改为 SeaweedFS；RustFS 可作为独立的
S3 目标并行部署。

## S3 契约

Teamgram 默认使用 path-style 和 `us-east-1`。服务端配置：
`S3_ENDPOINT=host:port`（不能带 scheme 或路径）、`S3_ACCESS_KEY`、
`S3_SECRET_KEY`、`S3_USE_SSL`、`S3_REGION`。DFS 使用四个 bucket：
`documents`、`encryptedfiles`、`photos`、`videos`。

切换 provider 前执行真实数据面探针：

```bash
S3_ENDPOINT=127.0.0.1:8333 \
S3_ACCESS_KEY=replace-with-key S3_SECRET_KEY=replace-with-secret \
S3_BUCKET=documents go run ./tools/s3-contract-probe
```

探针覆盖 bucket、单对象 PUT/GET/HEAD/LIST/DELETE 和 multipart，并在结束时
删除临时对象。除非显式传 `-create-bucket` 或设置
`S3_PROBE_CREATE_BUCKET=true`，不会创建 bucket。

## 本地 SeaweedFS

```bash
cp .env.example .env
docker compose -f docker-compose-env.yaml up -d
docker compose up -d
```

依赖栈启动持久化的单节点 `weed mini`，宿主机 S3 API 为
`127.0.0.1:8333`。这只适合开发/预发布，不代表物理高可用。禁止把
`data/minio` 直接复制到 SeaweedFS 或 RustFS 数据目录，不同 provider 的磁盘
格式不兼容。

## 生产 HA

使用 [Swarm 栈](../docker/object-storage/seaweedfs-ha-stack.yml)：3 个 Raft
Master、分布在独立主机/磁盘的 Volume Server、由外部 HA PostgreSQL 支撑的 2 个
Filer、2 个 S3 Gateway，以及在 HAProxy 处终止 TLS。overlay 网络开启加密，
生产凭据、Filer 配置、S3 配置和 TLS bundle 均通过外部 Docker secret/config
注入。节点标签、卷、证书和 secret 初始化见
[部署手册](../docker/object-storage/README.md)。

## 迁移、跨域备份与恢复演练

MinIO 到 SeaweedFS/RustFS 必须走 S3 API 迁移工具。工具保留 object key、内容、
大小、Content-Type、缓存头和用户 metadata，目标端重新读取校验，不删除源端。
copy 模式必须先停止写入并设置 `MIGRATION_CONFIRM_SOURCE_QUIESCED=YES`；启用
versioning 的 bucket 会被拒绝，版本历史和 delete marker 需要单独方案。

- `tools/object-storage-backup.sh` 要求源/目标 endpoint 和 location ID 不同，
  显式确认目标属于独立账户或故障域，并确认源端静默。
- `tools/object-storage-restore-drill.sh` 要求目标显式标记为可丢弃且为空，
  自动设置 `MIGRATION_REQUIRE_EMPTY_TARGET=true`。

两条脚本只生成不含凭据的 JSON 证据。命令成功或容器健康并不能证明物理故障域
耐久性；每次恢复后仍需保留 manifest 并完成应用层读取验收。
