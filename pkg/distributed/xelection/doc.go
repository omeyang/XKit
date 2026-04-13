// Package xelection 提供分布式选主抽象与基于 etcd 的实现。
//
// # 场景
//
// 适用于多实例服务中仅允许一个活跃 leader 的场景：定时任务唯一执行、
// 集群控制面 controller 单写、有状态服务主备切换等。
//
// # 接口
//
//   - Election：选主协调者，Campaign 发起竞选，阻塞直到当选或 ctx 取消。
//   - Leader：一次当选句柄，提供 IsLeader/CheckLeader/Lost/Resign。
//     每次 Campaign 独立 Session，Resign 或 session 失效后该 Leader 作废；
//     需重新当选请再次调用 Campaign。
//
// # 基本用法
//
//	elec, err := xelection.NewEtcdElection(client, "/myapp/leader/")
//	if err != nil { return err }
//	defer elec.Close(context.Background())
//
//	ldr, err := elec.Campaign(ctx, hostID) // 阻塞到当选
//	if err != nil { return err }
//	defer ldr.Resign(context.Background())
//
//	for {
//	    select {
//	    case <-ldr.Lost():
//	        return errors.New("lost leadership")
//	    case <-ctx.Done():
//	        return ctx.Err()
//	    default:
//	        doWork(ctx)
//	    }
//	}
//
// # 语义保证
//
//   - 同一 prefix 下 at-most-one leader（由 etcd concurrency 语义保障）。
//   - Lost 通道关闭触发于：被抢占、session 过期、observe ctx 取消、Resign。
//   - Resign 幂等；Close 幂等。
//
// # 已知限制
//
// etcd Session 基于 TTL 续约；网络分区时 leader 可能在感知到 session 过期
// 前仍误以为持有 leadership。关键路径应始终调用 CheckLeader 或监听 Lost。
package xelection
