---
package: pkg/util/xid
stability: Stable
coverage: 99.1%
tags: [util, id-generation, sonyflake, distributed]
related:
  - ../../05-concepts/02-error-handling-policy.md
---

# pkg/util/xid

Sonyflake v2 分布式 ID 生成。64-bit 时序 ID，含时间 / 机器 / 序列三段，可解析回来。

## 用途

- 分布式系统的全局唯一 ID（订单号、消息 ID、追踪 ID 等）
- 时序优势：时间在前，索引友好
- 不依赖外部服务：仅需 machineID（默认从 hostname/IP 哈希）

## 不适用场景

- 需要严格连续 ID（用数据库自增）
- 需要保密性（ID 含时间戳，可被推断生成时机）
- 单机非分布式：用 `uuid.New().String()` 更简单

## 快速上手

```go
import "github.com/omeyang/xkit/pkg/util/xid"

// 1. 启动时初始化（必须）
if err := xid.Init(); err != nil {
    log.Fatal(err)
}

// 2. 生成 ID
id, err := xid.New()                          // int64
s,  err := xid.NewString()                    // string

// 3. ctx 可中断的重试版（处理时钟回拨）
id, err := xid.NewWithRetry(ctx)

// 4. 解析与分解
id2, _ := xid.Parse("abc123")
parts, _ := xid.Decompose(id2)
fmt.Println(parts.Time, parts.MachineID, parts.Sequence)
```

## 关键类型与函数

| 名称 | 说明 |
|---|---|
| `Init(opts...) error` | 初始化全局生成器（必须先于 `New*`，可重入返回 `ErrAlreadyInitialized`）|
| `New() (int64, error)` | 生成 |
| `NewWithRetry(ctx) (int64, error)` | 重试版（仅对 `generateID` 错误有效，Sonyflake 内部时钟回拨 sleep 持锁无法被 ctx 中断）|
| `MustNewWithRetry()` | panic 版（极少用）|
| `Parse(s string) (int64, error)` | 解析字符串 ID |
| `Decompose(id) (Components, error)` | 拆解 → `Components{Time, MachineID, Sequence}` |
| `Generator` | 显式实例类型（测试或 DI 用）|
| `NewGenerator(opts...) (*Generator, error)` | 构造 |
| `DefaultMachineID()` | 默认 machineID 策略（多层 fallback）|
| `WithMachineID(fn) / WithCheckMachineID(fn)` | 自定义 machineID 提供与校验 |
| `WithMaxWaitDuration / WithRetryInterval` | 重试参数 |

## 设计要点

- **machineID 策略多层 fallback**（环境变量 → IP → hostname 哈希）
- **错误链双 wrap**：`%w: %w` 保留多 cause（如 hostname 失败 + IP 失败）
- **`ErrInvalidConfig` 包装**：config 验证错误统一类型
- **Init 失败可重试**：`initOnce` 在错误时不锁死（设计意图）
- **碰撞概率**：hostname 哈希可能碰撞，强建议显式设置 `XID_MACHINE_ID` 环境变量
- **测试辅助**：`newOverflowGenerator` 触发 `ErrOverTimeLimit`；`resetGlobal()` 重置 sync.Once

## 相关

- 概念：
  - [错误处理策略](../../05-concepts/02-error-handling-policy.md)（双 `%w` 用法）
- ADR：
  - [0001 构造函数返回 error](../../01-decisions/0001-constructor-returns-error-not-panic.md)
- API：[api.md#pkgutilxid](../../03-conventions/01-api.md#pkgutilxid)
