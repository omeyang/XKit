---
package: pkg/context/xplatform
stability: Stable
coverage: 100.0%
tags: [context, platform]
---

# pkg/context/xplatform

服务启动时通过 `Init(Config)` 设置平台信息。请求处理代码可读取当前平台 ID、上级平台标记和未分类区域 ID。

## 使用入口

示例文件中的 `Example_quickStart`、`ExampleConfig_Validate` 展示实际调用方式。完整签名可在仓库根目录用 `go doc ./pkg/context/xplatform` 查看。

[包说明](../../../pkg/context/xplatform/doc.go) · [使用示例](../../../tests/unit/pkg/context/xplatform/example_test.go)

## 相关

- 包：[xctx](01-xctx.md)（同样有 platform getter）
- API：[api.md#pkgcontextxplatform](../../03-conventions/01-api.md#pkgcontextxplatform)
