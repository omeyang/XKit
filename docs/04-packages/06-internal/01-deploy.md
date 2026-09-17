---
package: internal/deploy
stability: Internal
coverage: 100.0%
tags: [internal, deploy]
---

# internal/deploy

部署相关共享逻辑。仅 XKit 内部使用。

## 用途

- 多个 pkg 共享的部署/环境信息逻辑
- 避免在多包间重复

## 设计要点

- **覆盖率 100%**：变更影响多个包，必须严格守护
- **不暴露**：受 Go `internal/` 可见性规则保护

## 相关

- [Internal MOC](00-index.md)
