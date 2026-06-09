---
package: pkg/util/xfile
stability: Stable
coverage: 96.9%
tags: [util, file, path]
---

# pkg/util/xfile

文件操作工具。重点是**路径安全**：防 directory traversal、符号链接逃逸等。

## 用途

- 安全读写文件（拒绝越权路径）
- 原子写入（temp file + rename 模式）
- 路径标准化与校验

## 快速上手

```go
import "github.com/omeyang/xkit/pkg/util/xfile"

data, err := xfile.SafeReadFile("/data/configs", "user.json")
// 会拒绝 "../../etc/passwd"

err = xfile.AtomicWriteFile("/data/output.json", data, 0644)
```

## 关键类型与函数

| 名称 | 说明 |
|---|---|
| `SafeReadFile(baseDir, name) ([]byte, error)` | 限制在 baseDir 内 |
| `AtomicWriteFile(path, data, perm) error` | temp + rename |
| `EnsureDir(path, perm) error` | 幂等创建 |
| `IsSubPath(parent, child) bool` | 安全断言 |

## 设计要点

- **拒绝 `..` 转义**：先 `filepath.Clean`，再判前缀
- **原子写**：避免半写状态被读到
- **跨平台**：处理 Windows 与 Unix 路径分隔符差异

## 相关

- API：[api.md#pkgutilxfile](../../03-conventions/01-api.md#pkgutilxfile)
