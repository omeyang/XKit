// Package xhealth 提供 Kubernetes 健康探针功能。
//
// # 设计理念
//
// xhealth 采用单端口三端点模式，在一个 HTTP 端口上同时提供 liveness、readiness
// 和 startup 三种健康探针。每个端点可注册多个命名检查项，支持同步检查（实时执行）
// 和异步检查（后台定期执行并缓存结果）。
//
// # 三态健康模型
//
// xhealth 使用三态模型描述服务健康状况：
//
//   - [StatusUp]: 所有检查项均通过，服务完全正常
//   - [StatusDegraded]: 存在 SkipOnErr=true 的检查项失败，服务可用但有降级
//   - [StatusDown]: 存在 SkipOnErr=false 的检查项失败，服务不可用
//
// StatusDegraded 返回 HTTP 200（服务仍可处理请求），StatusDown 返回 HTTP 503。
//
// # 同步与异步检查
//
// 同步检查在探针请求到达时实时执行，适合轻量级检查。可通过 [WithCacheTTL]
// 设置缓存 TTL，避免频繁请求对下游造成压力。
//
// 异步检查在后台定期执行（由 Interval 控制），探针请求直接读取缓存结果。
// 适合耗时较长的检查（如数据库连接），避免探针请求阻塞。
//
// # 优雅关闭
//
// 调用 [Health.Shutdown] 后，所有探针端点立即返回 503，配合 K8s 流量摘除。
// 异步检查的后台 goroutine 会被停止。
//
// # 子路径查询
//
// 支持查询单个检查项状态，例如 /readyz/db 只返回名为 "db" 的检查结果。
//
// # xrun 集成
//
// [Health] 实现了 xrun.Service 接口，可直接传入 xrun.RunServices 管理：
//
//	h, _ := xhealth.New(xhealth.WithAddr(":8081"))
//	h.AddReadinessCheck("db", xhealth.CheckConfig{
//	    Check: xhealth.DatabasePingCheck(db),
//	})
//	xrun.RunServices(ctx, h)
//
// # Context 安全
//
// 所有接受 context.Context 的公开入口方法均在入口处检查 nil context，
// 传入 nil 会返回 [ErrNilContext] 而非 panic。
//
// # 零外部依赖
//
// xhealth 仅依赖标准库，不引入 xlog 或其他日志库。状态变更通知通过
// [WithStatusListener] 回调实现，调用方自行选择日志方案。
package xhealth
