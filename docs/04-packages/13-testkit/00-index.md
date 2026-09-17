---
title: Testkit 包
tags: [moc, packages, testkit]
---

# Testkit 包

测试辅助包。**不对生产环境使用**，仅用于业务方对 XKit 进行集成测试。

## 包列表

| 包 | 用途 | 稳定性 | 覆盖率 |
|---|---|---|---|
| [xetcdtest](01-xetcdtest.md) | 嵌入式 etcd server（集成测试用） | Internal | 70.5% |
| [xredismock](02-xredismock.md) | Redis Mock 客户端 | Internal | 87.0% |

## 其他位置的 Mock 包

- [pkg/distributed/xsemaphore/xsemaphoremock](../05-distributed/04-xsemaphore.md#mock-子包)：mockgen 生成的 gomock 桩

## 设计要点

按 ADR-0008 规定，**所有 mock 放在 `<pkg>mock/` 子包**，避免拖低主包覆盖率。

## 相关

- [Mock 隔离概念](../../05-concepts/05-mock-isolation.md)
- [ADR 0008 mock 放子包](../../01-decisions/0008-mock-in-subpackage.md)
