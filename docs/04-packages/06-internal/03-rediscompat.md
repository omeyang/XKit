---
package: internal/rediscompat
stability: Internal
coverage: 100.0%
tags: [internal, redis, compat, predixy]
---

# internal/rediscompat

Redis 代理脚本模式探测。某些 Redis 代理（如 **Predixy**）不支持 `EVAL` / `EVALSHA`，需在启动时探测并降级。

## 用途

- xsemaphore / xdlock / xcron 等依赖 Lua 的包启动时调用
- 探测代理类型，自动选择脚本模式（Inline EVAL / EVALSHA / 完全不用 Lua）

## 快速上手

```go
import "github.com/omeyang/xkit/internal/rediscompat"

mode, err := rediscompat.DetectScriptModeBounded(ctx, redisClient, 5*time.Second)
if err != nil { /* fallback */ }

// mode 可作为 xsemaphore.WithScriptMode 选项
sem, _ := xsemaphore.New(redisClient, "r",
    xsemaphore.WithScriptMode(mode),
)
```

## 关键类型与函数

| 名称 | 说明 |
|---|---|
| `ScriptMode` 枚举 | EVAL / EVALSHA / NoScript |
| `DetectScriptMode(ctx, client)` | 探测（可能阻塞）|
| `DetectScriptModeBounded(ctx, client, timeout)` | 有界版（goroutine + select + timer 保证返回）|
| `IsScriptUnsupportedError(err) bool` | 识别 NOPERM / ACL 错误（排除 OOM 瞬态）|

## 设计要点

- **goroutine 泄漏**：`DetectScriptModeBounded` 在超时后 goroutine 仍可能阻塞（文档化设计决策，doc.go:92-98）
- **NOPERM ACL 排除 OOM**：FG 修复识别精度

## 相关

- 包：[xsemaphore](../05-distributed/04-xsemaphore.md)（最大消费者）
- 文档化在 doc.go + script_mode.go
