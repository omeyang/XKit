---
package: pkg/util/xsys
stability: Stable
coverage: 96.0%
tags: [util, sys, rlimit]
---

# pkg/util/xsys

系统资源限制管理。当前仅 `RLIMIT_NOFILE`（最大打开文件数）。**跨平台**：Linux + BSD/Darwin 通过 build tag 分别实现。

## 用途

- 服务启动时把 NOFILE 拉到 hard limit
- 提前发现 ulimit 配置问题

## 快速上手

```go
import "github.com/omeyang/xkit/pkg/util/xsys"

if err := xsys.SetFileLimit(65535); err != nil {
    slog.Warn("cannot raise NOFILE", "err", err)
}

soft, hard, _ := xsys.GetFileLimit()
slog.Info("ulimit -n", "soft", soft, "hard", hard)
```

## 关键类型与函数

| 名称 | 说明 |
|---|---|
| `SetFileLimit(limit uint64) error` | 设置 NOFILE（不超过 hard）|
| `GetFileLimit() (soft, hard uint64, err error)` | 查询 |

## 设计要点

- **`rlimFromUint64 / rlimToUint64`** 抽出，Linux `int64` 与 BSD `uint64` 类型差异隔离
- **build tag 分离**：`resource_unix.go` (Linux) vs `rlim_int64.go` / `rlim_uint64.go`
- **测试 cleanup 完整恢复 hard limit**：避免污染并发测试
- **`validateFileLimit`** 统一前置校验

## 相关

- API：[api.md#pkgutilxsys](../../03-conventions/01-api.md#pkgutilxsys)
