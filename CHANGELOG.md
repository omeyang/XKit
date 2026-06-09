# 变更日志

本文件记录 XKit 已发布版本的变更。格式遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.0.0/)，版本遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

> 当前进度与包稳定性不在此记录，见 [`docs/02-progress.md`](docs/02-progress.md)。
> 关键设计决策见 [`docs/01-decisions/`](docs/01-decisions/00-index.md)。

## [未发布] - v0.1.0 候选

首个 Tag 候选。包含 38 个包（Stable 14 / Beta 21 / Alpha 1 / Internal 3），
整体覆盖率 94.0%，核心包均 ≥ 90%。

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

- Go 1.25.10 + golangci-lint v2.8.0 + go-task
- `develop-1.23-release` 分支：Go 1.23 功能等价版本（仅依赖版本上限不同）
- CI 流水线本地化（`task pre-push` 与 `.github/workflows/ci.yml` 同源）
- 多轮 CA/CB/Codex A/Codex B 对抗审查（覆盖 30+ 包）

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
