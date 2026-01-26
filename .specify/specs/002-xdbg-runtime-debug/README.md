# 002-xdbg-runtime-debug

运行时调试模块，提供生产环境动态调试能力。

## 概述

xdbg 是 xkit 的运行时调试模块，参考 gobase/mdbg 设计，但针对 K8s 环境和底层库场景进行了重大改进。

## 核心创新点

1. **K8s 原生设计**：多种触发方式（信号 + 命令）
2. **Unix Socket 优先**：无需暴露网络端口，更安全
3. **xkit 深度集成**：一键查看 xbreaker/xlimit/xcache 状态
4. **安全可审计**：文件权限控制、SO_PEERCRED 身份识别、命令白名单、操作审计
5. **轻量级设计**：不引入 HTTP/Protobuf 依赖

## 与 gobase/mdbg 对比

| 特性 | gobase/mdbg | xdbg |
|------|-------------|------|
| 触发方式 | 仅 SIGUSR1 | 信号 + 命令 |
| 传输协议 | TCP + Protobuf | Unix Socket + 轻量二进制 |
| 安全机制 | 简单权限分级 | 文件权限 + SO_PEERCRED + 白名单 + 审计 |
| 模块集成 | 无 | xlog/xbreaker/xlimit/xcache |

## 文档

- [spec.md](./spec.md) - 需求规格（已完成）
- [plan.md](./plan.md) - 技术计划（待开始）
- [tasks.md](./tasks.md) - 任务拆解（待开始）

## 状态

**当前阶段**：Phase 1 - 需求规格

**负责人**：TBD
