# 02 · 约束条件

## 语言与工具链

| 项 | 约束 |
|---|---|
| Go 版本 | **1.25.9**（固定，流水线一致；`task verify` 校验） |
| golangci-lint | **v2.8.0**（`Taskfile.yml: GOLANGCI_LINT_VERSION` 与 `.github/workflows/ci.yml` 同步） |
| 任务运行器 | `go-task`（Makefile 已弃用） |
| OS 支持 | Linux / macOS / FreeBSD / DragonFly；Windows 部分包通过 build tag 退化 |
| 架构 | amd64 / arm64；32 位平台由 `xkeylock` 等包通过 build tag 兼容 |

## 依赖策略

- **最小化外部依赖**：新增依赖需 ADR 说明替代方案与被拒理由。
- **版本锁定**：所有依赖在 `go.mod` 中显式锁版本，`go mod verify` 通过。
- **关键三方库**：
  - `log/slog`（标准库日志，唯一日志后端）
  - `go.opentelemetry.io/otel`（可观测性）
  - `github.com/go-redis/redis/v9`（Redis 客户端）
  - `github.com/hashicorp/golang-lru/v2`（LRU）
  - `github.com/sony/sonyflake/v2`（ID 生成）
  - `github.com/confluentinc/confluent-kafka-go`（Kafka）
  - `github.com/apache/pulsar-client-go`（Pulsar）
  - `go.etcd.io/etcd/client/v3`（etcd）
  - `go.mongodb.org/mongo-driver/v2`（MongoDB）
  - `github.com/ClickHouse/clickhouse-go/v2`（ClickHouse）

## 环境约束

- **集成测试**：`pkg/mq/...`、`pkg/storage/...` 等依赖外部中间件的测试使用 `integration` build tag；容器编排见 `deploy/integration/`。
- **CI 与本地一致**：`task pre-push` 与 `.github/workflows/ci.yml` 执行同一组检查（fmt-check / lint-ci / vulncheck / test-short）。

## 代码约束

- `.golangci.yml`：
  - `errcheck.check-blank: true` — `_ = expr` 仍会被标记
  - `errcheck.check-type-assertions: true` — 类型断言必须 comma-ok
  - `funlen.max` = 70 行
  - `gocyclo` ≤ 10
- **Mock 隔离**：mock 放 `<pkg>mock/` 子包，避免拖低主包覆盖率。
- **错误抽象边界**：跨包错误用 `%w` 保持链；模糊抽象边界（如 ID 生成器内部错误）用 `%v`，须以"设计决策:"注释说明。

## 文档约束

- 单文件 ≤ 800 行；超出拆分。
- 禁止记账式表述（"以前/后来/原本/改为"）；由 `task docs-ledger-check` 强制。
- 历史演进入 `CHANGELOG.md`；审查过程入 `docs/_archive/`。
