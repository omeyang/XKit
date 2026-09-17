---
title: Leader Election 模式
tags: [pattern, distributed, election, leader]
---

# Leader Election

## 问题

多副本服务里选一个作为 leader 长期持有某身份（如执行 cron、协调任务、维持连接）。leader 失联后备机自动接管。

## XKit 实现

[xelection](../04-packages/05-distributed/03-xelection.md) 基于 etcd concurrency。

## 模板

```go
elec, _ := xelection.NewEtcdElection(etcdClient, "/elections/cron",
    xelection.WithTTL(15),
)

leader, err := elec.Campaign(ctx, "pod-1")
if err != nil { return err }

// 此时已是 leader
go runLeaderWork(ctx, leader)

select {
case <-leader.Lost():           // ← lease 失联,自动让位
    slog.Warn("lost leadership, retrying...")
    // 可以重试 Campaign
case <-ctx.Done():
    _ = leader.Resign(context.Background())  // ← 主动让位
}
```

## 与 Distributed Lock 的区别

| | Distributed Lock | Leader Election |
|---|---|---|
| 持有期 | 短（秒级） | 长（小时-天） |
| 状态查询 | Lock 成功即持锁 | 持续监听 lease 状态 |
| 失联 | TTL 过期，他人抢 | `Lost()` 通知 |
| 用例 | 临界区互斥 | 主备切换、cron 去重 |

## 关键陷阱

### 1. 网络分区下的脑裂

A 持 leader，网络分区导致 A 失联，B 成为新 leader。但 A 自己**还以为是 leader**（没收到失联通知）。

**XKit 解法**：
- `observe` 在 watch 中断 / session 过期 / 被抢占时调用 `releaseSession`
- `Lost()` channel 通知调用方
- 调用方必须监听 `Lost()`，发现失联立即停止 leader 工作

### 2. Campaign 期间 Close

并发：另一 goroutine 调 `Close()`，正在 Campaign 的 goroutine 应立即返错。

**XKit 解法**：`etcdElection.Close` 通过 `closeCtx` 打断 in-flight Campaign。

### 3. Resign nil ctx

Resign 入口应防 nil ctx（FG-M fix，与 Campaign 对齐）。

## 相关

- 包：[xelection](../04-packages/05-distributed/03-xelection.md)
- 模式：[Distributed Lock](04-distributed-lock.md)
- 概念：[Nil context 防御](../05-concepts/06-nil-context-defense.md)
