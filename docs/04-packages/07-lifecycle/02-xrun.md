---
package: pkg/lifecycle/xrun
stability: Stable
coverage: 98.2%
tags: [lifecycle, errgroup, signal]
---

# pkg/lifecycle/xrun

进程生命周期管理。`errgroup` 加信号处理，统一管理多 goroutine 的启动与优雅退出。

## 用途

- main 函数里 spawn 多个长期服务（HTTP/gRPC/cron/MQ）统一管理
- SIGTERM / SIGINT 触发优雅 shutdown
- 任一服务失败带动整体退出

## 快速上手

```go
import "github.com/omeyang/xkit/pkg/lifecycle/xrun"

if err := xrun.Run(ctx,
    xrun.WithShutdownTimeout(10*time.Second),
    xrun.Job("http", func(ctx context.Context) error {
        return httpServer.Serve(ctx)
    }),
    xrun.Job("grpc", func(ctx context.Context) error {
        return grpcServer.Serve(ctx)
    }),
); err != nil {
    log.Fatal(err)
}
```

## 关键类型与函数

| 名称 | 说明 |
|---|---|
| `Run(ctx, opts...) error` | 入口 |
| `Job(name, fn) Option` | 注册一个长任务 |
| `WithShutdownTimeout(d)` | 优雅停超时 |
| `WithSignals(...)` | 自定义触发信号集 |

## 设计要点

- **errgroup 语义**：任一返错触发 ctx cancel
- **信号 → ctx cancel**：统一信号处理
- **shutdownTimeout**：到点强制返回，避免卡死

## 相关

- API：[api.md#pkglifecyclexrun](../../03-conventions/01-api.md#pkglifecyclexrun)
