# XKit 集成测试环境

使用 `podman kube play` 快速启动所有集成测试依赖服务。

## 服务清单

| 服务 | 版本 | 端口 | 用途 |
|------|------|------|------|
| Redis | 7.2 | 6379 | xcache, xdlock |
| MongoDB | 7.0 | 27017 | xmongo |
| ClickHouse | 23.12 | 9000, 8123 | xclickhouse |
| Kafka | 3.7 (KRaft) | 9092 | xmq |
| Pulsar | 3.2.3 | 6650, 18080 | xmq |
| etcd | 3.5.17 | 2379 | xdlock |

## 快速开始

```bash
# 一键启动所有服务
task kube-up

# 运行集成测试（使用预启动的服务，秒级完成）
task kube-test

# 查看服务状态
task kube-status

# 停止所有服务
task kube-down
```

## 与 testcontainers 的关系

项目同时支持两种集成测试方式：

| 方式 | 命令 | 特点 | 适用场景 |
|------|------|------|---------|
| **kube-play** | `task kube-*` | 服务常驻，测试秒级 | 本地开发反复测试 |
| **testcontainers** | `task integration-*` | 自动管理，每次启动 | CI/CD、一次性测试 |

当设置了 `XKIT_*` 环境变量时，testcontainers 会跳过容器启动，直接使用外部服务。

## 手动操作

```bash
# 直接使用 podman kube play
podman kube play deploy/integration/xkit-integration.yaml

# 查看 Pod 状态
podman pod ps

# 查看容器日志
podman logs xkit-kafka-kafka

# 停止并删除
podman kube play --down deploy/integration/xkit-integration.yaml
```

## 环境变量

使用 `env.sh` 设置环境变量：

```bash
source deploy/integration/env.sh
go test -tags=integration -v ./pkg/storage/xcache/...
```

或者直接设置：

```bash
export XKIT_REDIS_ADDR=localhost:6379
export XKIT_MONGO_URI=mongodb://localhost:27017
export XKIT_CLICKHOUSE_ADDR=localhost:9000
export XKIT_KAFKA_BROKERS=localhost:9092
export XKIT_PULSAR_URL=pulsar://localhost:6650
export XKIT_ETCD_ENDPOINTS=localhost:2379
```

## 故障排查

```bash
# 查看服务状态
task kube-status

# 查看特定服务日志
task kube-logs SERVICE=kafka
task kube-logs SERVICE=pulsar

# 重启环境
task kube-restart

# 完全清理后重建
task kube-down
podman pod rm -f $(podman pod ps -q --filter label=app=xkit-integration)
task kube-up
```

## 资源需求

全部服务启动约需要：
- **内存**: ~4GB
- **CPU**: 4 核
- **磁盘**: ~5GB（镜像）
- **启动时间**: ~30-60 秒
