# CLAUDE.md

为 Claude Code (claude.ai/code) 提供本仓库的指导信息。

## 项目概述

XKit 是一个 Go 工具库，为业务开发提供可复用的基础组件。项目使用 Go 1.25+，遵循 Spec-Driven Development（规范驱动开发）工作流。

**目录结构：**
- `pkg/` — 公开 API（对外导出的包）
- `internal/` — 内部实现（不导出）
- `.specify/` — 功能规格、计划和任务文档

## 常用命令

所有命令使用 [go-task](https://taskfile.dev)。运行 `task --list` 查看全部选项。

```bash
# 开发工作流
task test              # 运行所有单元测试
task test-race         # 带竞态检测的测试
task lint              # 运行 golangci-lint
task pre-commit        # 提交前快速检查（fmt + lint + short tests）
task ci                # 完整 CI 检查（lint + coverage + race + vulncheck）

# 运行单个测试
go test -v -run TestFunctionName ./pkg/path/to/package/...

# 按模块运行测试
task test-context      # Context 模块（xctx, xenv, xplatform, xtenant）
task test-observability # Observability 模块（xlog, xmetrics, xtrace 等）
task test-storage      # Storage 模块（xcache, xclickhouse, xmongo）
task test-resilience   # Resilience 模块（xbreaker, xretry, xlimit）
task test-distributed  # Distributed 模块（xdlock, xcron）
task test-mq           # MQ 模块（xkafka, xpulsar）

# 性能与覆盖率
task bench             # 运行所有基准测试
task test-cover        # 生成覆盖率报告（.artifacts/coverage.html）

# 集成测试（需要 Docker/Podman）
task kube-up           # 启动所有服务（Redis, Mongo, ClickHouse, Kafka, Pulsar, etcd）
task kube-test         # 对运行中的服务执行集成测试
task kube-down         # 停止所有服务
```

## 架构

**按领域组织的包：**

| 领域 | 包 | 用途 |
|------|-----|------|
| Context | `xctx`, `xtenant`, `xplatform`, `xenv` | 请求上下文、租户/平台信息、环境变量 |
| Observability | `xlog`, `xtrace`, `xmetrics`, `xsampling`, `xrotate` | 日志、追踪、指标、采样、轮转 |
| Resilience | `xbreaker`, `xretry`, `xlimit` | 熔断器、重试策略、限流器 |
| Storage | `xcache`, `xmongo`, `xclickhouse`, `xetcd` | 缓存抽象、数据库客户端 |
| Distributed | `xdlock`, `xcron` | 分布式锁、定时任务 |
| MQ | `xkafka`, `xpulsar` | 消息队列客户端 |
| Config | `xconf` | 配置管理 |
| Util | `xfile`, `xpool` | 文件工具、Worker Pool |

**关键模式：**
- 每个包都有 `doc.go` 提供包文档
- 公开 API 包含 `example_test.go` 可运行示例
- 集成测试使用 `integration` 构建标签
- Mock 使用 `go.uber.org/mock` 生成
- 所有服务都有工厂方法（New, NewFromXxx）
- Client() 方法暴露底层实现，不限制高级特性

## Spec-Driven Development

新功能遵循此工作流：

1. `/speckit.specify` — 在 `.specify/specs/{NNN-short-name}/spec.md` 创建规格
2. `/speckit.plan` — 创建技术计划（`plan.md`）
3. `/speckit.tasks` — 创建任务拆解（`tasks.md`）
4. `/speckit.implement` — 执行实现

功能目录使用数字前缀（如 `001-feature-name/`）。

## 代码规范

- **语言**：文档和注释用中文，代码标识符用英文
- **测试覆盖率**：核心业务逻辑 ≥95%，整体 ≥90%
- **Lint**：必须通过 `golangci-lint run ./...`，零警告
- **依赖**：优先使用标准库；新外部依赖需在 plan.md 中说明理由
- **API 位置**：公开 API 放 `pkg/`，内部代码放 `internal/`
- **测试风格**：推荐表驱动测试（table-driven tests）

## 参考文件

- 项目原则：`.specify/memory/constitution.md`
- 文档标准：`.specify/prior-art/01-standards/01-documentation/`
- 测试标准：`.specify/prior-art/01-standards/03-testing/02-testing-standards.md`
