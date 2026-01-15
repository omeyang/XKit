// Package xmetrics 提供统一的可观测性接口（metrics + tracing）。
//
// # 设计理念
//
// xmetrics 仅定义最小化接口：Observer/Span/Attr，
// 业务代码只依赖接口；具体实现可替换。
// 默认实现基于 OpenTelemetry，兼容主流可观测栈。
//
// # 使用示例
//
//	obs, _ := xmetrics.NewOTelObserver()
//	ctx, span := xmetrics.Start(ctx, obs, xmetrics.SpanOptions{
//		Component: "xmongo",
//		Operation: "find_page",
//		Kind:      xmetrics.KindClient,
//	})
//	defer span.End(xmetrics.Result{Err: err})
//
// # 指标命名
//
// 统一指标：
//   - xkit.operation.total
//   - xkit.operation.duration
//
// 统一属性：component / operation / status。
package xmetrics
