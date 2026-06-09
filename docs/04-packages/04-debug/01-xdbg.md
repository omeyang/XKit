---
package: pkg/debug/xdbg
stability: Beta
coverage: 90.5%
tags: [debug, unix-socket]
---

# pkg/debug/xdbg

运行时调试服务。通过 **Unix Socket**（不开 HTTP 端口）暴露 goroutine / heap / stack / metrics 等内省接口。

## 用途

- 生产环境排障：无需重启即可访问进程内部状态
- 安全：Unix Socket 仅本机可访问，不外露
- 配套 CLI：`cmd/xdbgctl`

## 快速上手

```go
import "github.com/omeyang/xkit/pkg/debug/xdbg"

srv, _ := xdbg.New(xdbg.Config{
    SocketPath: "/var/run/myapp/debug.sock",
})
go srv.Serve()
defer srv.Close()
```

CLI：
```bash
xdbgctl --socket /var/run/myapp/debug.sock goroutines
xdbgctl --socket /var/run/myapp/debug.sock heap
```

## 关键类型与函数

| 名称 | 说明 |
|---|---|
| `Server / New(cfg)` | 入口 |
| `Config` | SocketPath / 权限 / 处理器注册 |
| `Handler` interface | 自定义内省命令 |

## 设计要点

- **Unix Socket 默认权限 0600**：仅 owner 可访问
- **`safeCallback` 嵌套 recover**：与 xpool 同样防 logger panic 二次崩溃
- **配套 `cmd/xdbgctl`** CLI 简化访问

## 相关

- 包：[xrun](../07-lifecycle/02-xrun.md)（main 中通常和 xrun 一起启动）
- API：[api.md#pkgdebugxdbg](../../03-conventions/01-api.md#pkgdebugxdbg)
