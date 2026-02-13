# XKit 代码审查指南

本目录包含 XKit 各模块的代码审查 prompts，按依赖顺序编号。

## 审查哲学

XKit 是**全新的基础工具库**，追求最优架构设计，不考虑向后兼容。详见 [00-methodology.md](00-methodology.md)。

## 审查流程

```
1. 阅读 00-methodology.md → 理解方法论、15 个评估维度、输出规范
2. 按编号顺序选择模块 → 依赖关系由低到高
3. 对每个模块：阅读源码 → 运行工具 → 使用 Skills → 输出报告
4. 跨模块审查 → 检查包间依赖、接口一致性、命名统一性
```

## 模块列表（按依赖顺序）

| 编号 | 模块 | 文件 | 包 | 依赖层级 |
|------|------|------|-----|---------|
| 00 | 方法论 | [00-methodology.md](00-methodology.md) | — | 通用框架 |
| 01 | 工具包 | [01-util.md](01-util.md) | xid, xfile, xjson, xkeylock, xlru, xmac, xnet, xpool, xproc, xsys, xutil | L1（无依赖） |
| 02 | 上下文 | [02-context.md](02-context.md) | xctx, xenv, xplatform, xtenant | L1-2 |
| 03 | 配置 | [03-config.md](03-config.md) | xconf | L1 |
| 04 | 可观测性 | [04-observability.md](04-observability.md) | xlog, xmetrics, xtrace, xsampling, xrotate | L1-3 |
| 05 | 韧性 | [05-resilience.md](05-resilience.md) | xretry, xbreaker, xlimit | L1-3 |
| 06 | 存储 | [06-storage.md](06-storage.md) | xcache, xmongo, xclickhouse, xetcd | L1-3 |
| 07 | 分布式 | [07-distributed.md](07-distributed.md) | xdlock, xcron, xsemaphore | L2-4 |
| 08 | 消息队列 | [08-mq.md](08-mq.md) | xkafka, xpulsar | L3 |
| 09 | 业务与生命周期 | [09-business-lifecycle.md](09-business-lifecycle.md) | xauth, xrun, xdbg | L3+ |

## 包覆盖率汇总

共覆盖 **37 个包**（含全部新增包 xid, xsemaphore, xlimit, xmac, xjson, xkeylock, xlru, xnet, xproc, xsys, xutil）。

## Skills 速查

| 阶段 | 推荐 Skill | 用途 |
|------|-----------|------|
| 综合扫描 | `/code-reviewer` | 快速定位高优先级问题 |
| 惯用法 | `/go-style` + `/golang-patterns` | Go 规范与模式 |
| 领域专项 | `/redis-go`, `/otel-go`, `/kafka-go` 等 | 特定技术栈审查 |
| 测试质量 | `/go-test` | 覆盖率与测试设计 |
| 架构总结 | `/design-patterns` | SOLID、架构模式 |
