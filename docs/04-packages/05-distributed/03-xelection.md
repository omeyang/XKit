---
package: pkg/distributed/xelection
stability: Stable
coverage: 95.4%
tags: [distributed, election, etcd, leader]
related:
  - ../../06-patterns/06-leader-election.md
  - ../../05-concepts/06-nil-context-defense.md
---

# pkg/distributed/xelection

基于 etcd concurrency 的分布式选主。竞选 → 持有 lease → 失联自动释放。

## 用途

- 多副本服务选一个 leader 长期持有
- 周期性任务在多副本中防重（leader 才执行）
- 主备切换：leader 失联立即释放，备机抢占

## 不适用场景

- 短期互斥：用 [xdlock](02-xdlock.md) 分布式锁更合适
- 无 etcd 环境：xelection 强依赖 etcd

## 快速上手

```go
import "github.com/omeyang/xkit/pkg/distributed/xelection"

elec, err := xelection.NewEtcdElection(etcdClient, "/elections/cron",
    xelection.WithTTL(15),
    xelection.WithLogger(logger),
)
if err != nil { /* ... */ }

leader, err := elec.Campaign(ctx, "pod-1")
if err != nil { /* ctx 取消或 etcd 异常 */ }

select {
case <-leader.Lost():
    slog.Warn("lost leadership")
case <-ctx.Done():
    _ = leader.Resign(context.Background())
}
```

## 关键类型与函数

| 名称 | 说明 |
|---|---|
| `Election` interface | `Campaign(ctx, id) (Leader, error)` |
| `Leader` interface | `Resign / IsLeader / Lost / Key` |
| `NewEtcdElection(client, prefix, opts...) (Election, error)` | 构造 |
| `WithTTL(seconds) / WithTTLDuration` | 租约 TTL |
| `WithLogger(xlog.Logger)` | 日志 |
| `MockSession / MockElection / NewExpiredMockSession()` | 同包测试桩 |

## 设计要点

- **`observe`** 在 watch 中断 / session 过期 / 被抢占时调用 `releaseSession` 释放 lease
- **`etcdElection.Close`** 通过 `closeCtx` 打断 in-flight Campaign + 成功 Campaign 后的 closed 竞态防御
- **Campaign 抽取** `acquireSession/runCampaign/handleCampaignErr` 降圈复杂度
- **Resign nil ctx 防御**：与 Campaign 入口对齐（FG-M fix）
- **`Resign` 错误链**：用 `errors.Join` 保留 `session.Close` 错误

## 相关

- 概念：
  - [Nil context 防御](../../05-concepts/06-nil-context-defense.md)
- 模式：
  - [Leader Election](../../06-patterns/06-leader-election.md)
- API：[api.md#pkgdistributedxelection](../../03-conventions/01-api.md#pkgdistributedxelection)
