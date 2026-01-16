// Package xlimit 提供分布式限流功能，保护系统免受流量过载影响。
//
// # 设计理念
//
// xlimit 基于 [go-redis/redis_rate] 实现分布式限流，支持多租户、多维度限流，
// 并在 Redis 故障时自动降级到本地限流。
//
// # 核心概念
//
//   - Limiter：限流器接口，支持 Allow/AllowN 操作
//   - Key：限流键，包含租户、调用方、API 等维度信息
//   - Rule：限流规则，定义限流窗口、配额和覆盖策略
//   - Result：限流结果，包含是否允许、剩余配额、重试时间等
//
// # 限流维度
//
// 支持多维度组合限流：
//   - 租户（Tenant）：按租户 ID 限流
//   - 调用方（Caller）：按上游服务限流
//   - API（Method + Path）：按接口限流
//   - 资源（Resource）：按自定义资源名限流
//
// # 层级限流
//
// 支持层级限流策略（串行检查，任一层级拒绝则拒绝）：
//   - 全局限流 → 租户限流 → API 限流
//
// # 快速开始
//
//	// 创建分布式限流器
//	limiter, err := xlimit.New(redisClient,
//	    xlimit.WithRules(
//	        xlimit.TenantRule("tenant-limit", 1000, time.Minute),
//	    ),
//	)
//
//	// 执行限流检查
//	key := xlimit.Key{Tenant: "tenant-001"}
//	result, err := limiter.Allow(ctx, key)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	if !result.Allowed {
//	    log.Printf("限流触发，请在 %v 后重试", result.RetryAfter)
//	}
//
// # HTTP 中间件
//
//	mux := http.NewServeMux()
//	mux.Handle("/api/", xlimit.HTTPMiddleware(limiter)(handler))
//
// # 降级策略
//
// Redis 故障时支持三种降级策略：
//   - FallbackLocal：降级到本地限流（推荐）
//   - FallbackOpen：放行所有请求
//   - FallbackClose：拒绝所有请求
//
// # 动态配置
//
// 支持通过 xconf/etcd 热更新限流规则：
//
//	limiter, err := xlimit.New(redisClient,
//	    xlimit.WithConfigLoader(xlimit.NewEtcdLoader(etcdClient, "/config/xlimit")),
//	)
//
// [go-redis/redis_rate]: https://github.com/go-redis/redis_rate
package xlimit
