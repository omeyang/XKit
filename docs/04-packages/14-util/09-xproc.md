---
package: pkg/util/xproc
stability: Stable
coverage: 100.0%
tags: [util, process]
---

# pkg/util/xproc

进程信息查询。极简：PID + 进程名。

## 用途

- 日志/监控字段补充
- 多进程协作时识别自己

## 快速上手

```go
import "github.com/omeyang/xkit/pkg/util/xproc"

pid := xproc.ProcessID()
name := xproc.ProcessName()  // 极端情况下可能空串
```

## 关键类型与函数

| 名称 | 说明 |
|---|---|
| `ProcessID() int` | 当前 PID |
| `ProcessName() string` | 进程名（不含路径），失败返空 |

## 设计要点

- **`baseName` + `resolveProcessName`** 分层，便于测试覆盖
- **失败兜底**：`os.Executable` 失败时返空串，调用方应做兜底

## 相关

- API：[api.md#pkgutilxproc](../../03-conventions/01-api.md#pkgutilxproc)
