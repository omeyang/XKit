---
package: pkg/util/xfile
stability: Stable
coverage: 96.9%
tags: [util, file, path]
---

# pkg/util/xfile

处理来自调用方的路径时，使用 `SanitizePath` 做路径校验，使用 `SafeJoin` 在指定目录下拼接路径。需要创建目录时可调用 `EnsureDir`。

## 使用入口

示例文件中的 `ExampleSanitizePath`、`ExampleSafeJoin` 展示实际调用方式。完整签名可在仓库根目录用 `go doc ./pkg/util/xfile` 查看。

[包说明](../../../pkg/util/xfile/doc.go) · [使用示例](../../../tests/unit/pkg/util/xfile/example_test.go)

## 相关

- API：[api.md#pkgutilxfile](../../03-conventions/01-api.md#pkgutilxfile)
