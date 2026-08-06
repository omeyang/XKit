# 变更日志

本文件记录 XKit 已发布版本的变更。格式遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.0.0/)，版本遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

> 当前进度与包稳定性不在此记录，见 [`docs/02-progress.md`](docs/02-progress.md)。
> 关键设计决策见 [`docs/01-decisions/`](docs/01-decisions/00-index.md)。

## [未发布] - v0.1.0 候选

首个 Tag 候选。包清单、稳定性分级与覆盖率以 [`docs/02-progress.md`](docs/02-progress.md)
为单一事实源，此处不复制具体数字以免失真。

### 新增

- **Context**：xctx（追踪/租户/平台增强）、xtenant（HTTP+gRPC 中间件）、xplatform、xenv
- **Observability**：xlog（slog 后端）、xtrace（W3C Trace Context）、xmetrics（OTel 抽象）、xrotate（lumberjack）、xsampling
- **Resilience**：xbreaker（熔断器）、xretry、xlimit（Token Bucket 分布式限流）
- **Storage**：xcache、xetcd（含 Informer list+watch 缓存）、xmongo、xclickhouse（批次原子性）
- **Distributed**：xdlock、xcron、xelection（etcd 选主）、xsemaphore（Redis Lua + Fallback）
- **MQ**：xkafka（DLQ + OTel）、xpulsar（DLQ + OTel）
- **Config**：xconf（koanf 后端）
- **Business**：xauth（Token + 平台信息 + 双层缓存）
- **Debug**：xdbg（Unix Socket 调试服务）+ `cmd/xdbgctl` 客户端
- **Security**：xtls（TLS 配置 + mTLS）
- **Lifecycle**：xrun（errgroup + 信号）、xhealth（K8s 探针）
- **Util**：xfile、xid（Sonyflake v2）、xjson、xkeylock、xlru、xmac、xnet（netip）、xpool、xproc、xsys、xutil
- **Testkit**：xetcdtest（嵌入式 etcd）、xredismock、xsemaphoremock
- **Internal**：deploy、mqcore、rediscompat（Redis 代理脚本探测）、storageopt

### 工程基础设施

- Go 1.24.6 + golangci-lint v2.8.0 + go-task
- 工具链锁定 go1.24.6（内网流水线构建底座）：`task check-toolchain` 精确门禁，
  CI 设 `GOTOOLCHAIN=local` 复现内网约束（`GOSUMDB=off` + 私有代理取不到工具链模块，
  任何自动下载必失败）；go.mod 不写 `toolchain` 指令；由源码构建的工具
  （golangci-lint / gocyclo / actionlint）版本固定且 go directive ≤ 1.24.6
- 不设 govulncheck 门禁（`task pre-push`、`task ci`、CI 均不含）。工具链锁定
  go1.24.6，而 Go 1.24 已停止安全维护：实测存在多条可达漏洞，其中一部分需把底座
  升到 go1.24.13（`golang:1.24.6-bullseye` 是官方最后一个 bullseye 底座，换底座会
  改变 glibc 依赖），另一部分只在 go1.25.x / x/net v0.51.0+ 修复，在 Go 1.24
  上永无补丁。恒红的门禁拦不住问题，只会淹没真正新增的信号，故移除而非豁免。
  具体条目以实跑为准（数字随依赖与漏洞库变动，此处不记）：
  `go run golang.org/x/vuln/cmd/govulncheck@v1.1.4 ./...`。
- `develop-1.23-release` 分支：Go 1.23 功能等价版本（仅依赖版本上限不同）
- CI 触发覆盖两个长期分支：`on.push` / `on.pull_request` 的 `branches` 均为
  `[main, develop-1.23-release]`，两分支逐字一致。此前只写 `[main]`，
  `develop-1.23-release` 的 push 永远不匹配，该分支自建仓起从未跑过远端 CI
- CI 流水线本地化（`task pre-push` 覆盖 CI 的 Go 侧静态检查；
  actionlint、`-race` 全量测试、覆盖率上报、llms 生成仅 CI 跑）
- 多轮 CA/CB/Codex A/Codex B 对抗审查（覆盖 30+ 包）

### 破坏性变更（相对改造前）

- `pkg/resilience/xbreaker` 不再导出 `CircuitBreaker[T]` 与 `TwoStepCircuitBreaker[T]`
  两个泛型类型别名。泛型类型别名是 Go 1.24 语言特性，`develop-1.23-release` 分支
  无法表达；为使两分支公开 API 严格一致，两边统一移除。
  别名与被别名类型本就是同一类型，运行时行为、方法集、可比较性均无变化。
  **迁移**：显式书写类型名的地方改用 `gobreaker.CircuitBreaker[T]` /
  `gobreaker.TwoStepCircuitBreaker[T]` 并 `import "github.com/sony/gobreaker/v2"`；
  `NewCircuitBreaker` / `NewTwoStepCircuitBreaker` 的调用方式不变，返回值可直接用。

### 关键设计决策

详见 `docs/01-decisions/`：
- 0001 构造函数返回 error 而非 panic
- 0002 接口由使用方定义
- 0003 函数选项模式（Functional Options）
- 0004 错误包装策略（`%w` 跨包，`%v` 抽象边界）
- 0005 slog 作为唯一日志后端
- 0006 中文文档 / 英文标识符
- 0007 util 包不内置可观测性
- 0008 mock 放 `<pkg>mock/` 子包

---

## 开发说明

### 集成测试

消息队列、存储相关测试需要 `integration` build tag：

```bash
go test -tags=integration ./pkg/mq/...
```

容器编排见 [`deploy/integration/README.md`](deploy/integration/README.md)。
