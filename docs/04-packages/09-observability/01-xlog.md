---
package: pkg/observability/xlog
stability: Stable
coverage: 96.8%
tags: [observability, log, slog]
related:
  - ../../01-decisions/0005-slog-as-sole-log-backend.md
---

# pkg/observability/xlog

结构化日志。**唯一日志后端是 `log/slog`**。提供薄薄的 Logger 接口 + 工厂构造。

## 用途

- 所有 XKit 包通过此接口注入日志
- 业务方可注入自定义 `*slog.Logger`（如带 OTel handler）

## 不适用场景

- 不引入 zap/zerolog/logrus（见 ADR-0005）

## 快速上手

```go
import (
    "log/slog"
    "github.com/omeyang/xkit/pkg/observability/xlog"
)

logger := xlog.NewWithHandler(slog.NewJSONHandler(os.Stdout, nil))

// 在 XKit 包内作为选项注入
sem, _ := xsemaphore.New(redisClient, "r",
    xsemaphore.WithLogger(logger),
)
```

## 关键类型与函数

| 名称 | 说明 |
|---|---|
| `Logger` interface | XKit 内部使用的最小接口（`Info / Error / Debug / Warn`）|
| `NewWithHandler(slog.Handler) Logger` | 包装 slog handler |
| `Discard()` | 黑洞 Logger（测试用）|

## 设计要点

- **接口由使用方定义**（ADR-0002）：XKit 各包定义自己最小化的 Logger 接口，xlog.Logger 是个统一可选实现
- **slog 唯一**（ADR-0005）：不引入其他日志栈

## 相关

- ADR：
  - [0005 slog 作为唯一日志后端](../../01-decisions/0005-slog-as-sole-log-backend.md)
- API：[api.md#pkgobservabilityxlog](../../03-conventions/01-api.md#pkgobservabilityxlog)
