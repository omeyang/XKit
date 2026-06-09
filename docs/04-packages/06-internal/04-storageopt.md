---
package: internal/storageopt
stability: Internal
coverage: 100.0%
tags: [internal, storage, options]
---

# internal/storageopt

存储层共享选项。`xcache / xetcd / xmongo / xclickhouse` 共享的连接池/超时等参数。

## 用途

- 避免在多个存储包重复定义相同的 Option 类型
- 统一选项命名

## 设计要点

- **覆盖率 100%**：跨包共享，变更影响大
- **内部统一选项**：外部 API 上各 storage 包仍各自暴露自家 Option

## 相关

- [Internal MOC](00-index.md)
- 包：[xcache](../12-storage/01-xcache.md) / [xetcd](../12-storage/03-xetcd.md) / [xmongo](../12-storage/04-xmongo.md) / [xclickhouse](../12-storage/02-xclickhouse.md)
