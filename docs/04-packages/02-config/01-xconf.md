---
package: pkg/config/xconf
stability: Stable
coverage: 92.0%
tags: [config, koanf]
---

# pkg/config/xconf

应用可以从 YAML、JSON 文件或字节数据加载配置，使用 koanf 反序列化，并在配置变更后重新加载。

## 使用入口

示例文件中的 `ExampleNewFromBytes`、`ExampleConfig_Reload` 展示实际调用方式。完整签名可在仓库根目录用 `go doc ./pkg/config/xconf` 查看。

[包说明](../../../pkg/config/xconf/doc.go) · [使用示例](../../../tests/unit/pkg/config/xconf/example_test.go)

## 相关

- 包：[xenv](../03-context/02-xenv.md)（直接读 ENV 的简化场景）
- API：[api.md#pkgconfigxconf](../../03-conventions/01-api.md#pkgconfigxconf)
