// Package xauth 提供认证服务客户端，用于获取和管理访问 Token、平台信息等。
//
// # 功能概述
//
//   - Token 管理：获取、刷新、验证访问 Token
//   - 平台信息：获取平台 ID、判断是否有父平台、获取未归类组 Region ID
//   - 带认证的 HTTP 请求：自动添加 Authorization 头
//   - 双层缓存：L1 本地缓存（xlru）+ L2 Redis 缓存
//   - 可观测性：集成日志、指标、追踪
//
// # 缓存策略
//
// 双层缓存：
//   - L1 本地缓存：基于 xlru（LRU + TTL），高性能本地访问
//   - L2 Redis 缓存：分布式缓存，支持多实例共享，减少冷启动延迟
//
// Token 缓存 TTL 根据有效期动态计算，即将过期前触发后台刷新：
//   - 有效期 > 刷新阈值：TTL = 有效期 - 刷新阈值
//   - 有效期 <= 刷新阈值但 > 11秒：TTL = 有效期 - 10秒安全边际
//   - 有效期 <= 11秒：不缓存到 Redis，仅本地使用
//
// # 并发安全
//
// 使用 singleflight 防止缓存击穿，同一 tenantID 的并发请求只触发一次 API 调用。
//
// # 与 xctx 集成
//
// 通过 ContextClient 扩展接口从 context 获取租户信息。
// 预加载平台信息到 context。
//
// # Token 刷新与 401 重试
//
// 依赖 Token 过期时间和后台刷新管理生命周期，不在每次请求前验证有效性。
// 如果服务端可能主动吊销 Token，启用 AutoRetryOn401。
//
// # URL 处理
//
// Request 方法的 URL 参数支持两种格式：
//   - 相对路径（"/api/users"）：与 baseURL 拼接
//   - 完整 URL（"https://other-host.com/api/users"）：直接使用
//
// # TLS 安全配置
//
// 默认跳过证书验证（InsecureSkipVerify: true），生产环境建议显式配置。
//
// # 默认行为
//
//   - TLS：Config.TLS 为 nil 时跳过证书验证，生产环境请显式配置
//   - Logger：WithLogger(nil) 使用 slog.Default()，禁用日志请用 slog.New(slog.NewTextHandler(io.Discard, nil))
//
// # 扩展点
//
//   - CacheStore 接口：自定义远程缓存实现（默认提供 NoopCacheStore 和 RedisCacheStore）
//   - MetricsRecorder 接口：自定义细粒度指标收集
//   - WithHTTPClient：注入自定义 HTTP 客户端
//
// # Graceful Shutdown
//
// client.Close() 取消后台刷新任务并清理本地缓存。
package xauth
