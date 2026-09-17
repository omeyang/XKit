---
package: pkg/observability/xrotate
stability: Stable
coverage: 96.9%
tags: [observability, log, rotation]
---

# pkg/observability/xrotate

日志文件轮转。基于 `natefinch/lumberjack`，按大小 + 时间轮转，自动压缩。

## 用途

- 日志写入文件而非 stdout 时的滚动管理
- 长期运行服务避免日志撑满磁盘

## 快速上手

```go
import (
    "github.com/omeyang/xkit/pkg/observability/xrotate"
    "log/slog"
)

w := xrotate.New(xrotate.Config{
    Filename:   "/var/log/app.log",
    MaxSize:    100, // MB
    MaxBackups: 10,
    MaxAge:     30, // days
    Compress:   true,
})
logger := slog.New(slog.NewJSONHandler(w, nil))
```

## 关键类型与函数

| 名称 | 说明 |
|---|---|
| `Config` | 配置（Filename / MaxSize / MaxBackups / MaxAge / Compress）|
| `New(cfg) io.Writer` | 构造（实现 io.Writer，可注入任意 logger）|

## 设计要点

- **lumberjack 久经考验**：选它不写自家轮转
- **不耦合日志框架**：返 io.Writer，slog/zap/任何 logger 都能用

## 相关

- API：[api.md#pkgobservabilityxrotate](../../03-conventions/01-api.md#pkgobservabilityxrotate)
