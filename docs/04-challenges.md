# 04 · 困难与挑战

量化指标驱动。未达标项须在 PR 中说明并给出补救计划。

## 质量硬指标

| 指标 | 目标 | 当前门禁 |
|---|---|---|
| 核心包覆盖率 | ≥ 95% | `task pre-push` 覆盖率检查 |
| 整体覆盖率 | ≥ 90% | 同上 |
| 单函数长度 | ≤ 70 行 | `funlen` lint |
| 圈复杂度 | ≤ 10 | `gocyclo` lint |
| 竞争检测 | 零报告 | `task test-race` |
| 漏洞扫描 | 零高危 | `task vulncheck` |

## 并发与生命周期挑战

- **Goroutine 泄漏**：长生命周期组件（`xpool`、`xlru`、`xkafka` Consumer 等）须有显式 `Close/Shutdown`，测试用 `goleak` 验证。已知上游泄漏（如 lumberjack `millRun`）通过 `goleak` 白名单并在包 memory 中记录限制。
- **Close 线性化**：`Close` 必须等待在途 `Write/Rotate/Acquire` 后置操作完成（参见 `xrotate`、`xsemaphore`）。
- **典型并发风险**（需在包实现时显式处理）：
  - typed-nil interface 参数拦截（`xsampling` composite、`xmetrics` provider）
  - nil `context.Context` 防御（`xsemaphore.ErrNilContext`、`xkeylock`）
  - Close vs Submit/Acquire 竞争（`xpool.submitMu`、`xsemaphore.localMu+closed`）

## 性能挑战

- **热路径零分配**：
  - `xctx.LogAttrs` 空 context 零分配
  - `xnet.RangeSize` IPv4 快速路径 1 alloc（vs 3）
  - `xmac.MarshalText` 1 alloc（vs 2）
  - `xnet.FormatFullIPAddr` IPv6 用栈 `[32]byte` + `hex.Encode`
- **批量/Pipeline 优先**：Redis 多键操作使用 pipeline 或 Lua 脚本（`xsemaphore`、`xdlock`）。

## 分布式与一致性挑战

- **Redis 幂等与原子性**：Lua 脚本 + hash tag 保证多键落在同槽；Release 使用 CAS/Load 避免网络错误后状态丢失。
- **W3C Trace Context**：`xtrace` 严格按 spec 拒绝非法 flags（v00），与 OTel propagator 组合时出站注入归 propagator 负责。
- **At-least-once 去重**：消费侧由业务使用 `xidempotency` 或自行处理；MQ 包不做隐式去重。
- **Fallback 语义**：`xlimit`、`xsemaphore` 在 Redis 不可用时 fail-open 或本地降级，策略与边界在包 doc 中显式声明。

## 跨平台挑战

- **Rlimit 类型差异**：Linux/macOS 上 `unix.Rlimit.Cur/Max` 为 `uint64`；FreeBSD/DragonFly 为 `int64`。`xsys` 通过 build tag 拆分 `rlim_uint64.go`/`rlim_int64.go`。
- **32 位兼容**：`xkeylock.shardPayload` 用可移植 `unsafe.Sizeof` 计算填充。
