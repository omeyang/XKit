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
// WithLocalCache(false) 统一禁用 Token 和平台数据的 L1 本地缓存。
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
// # Token 生命周期管理
//
// 依赖 Token 过期时间和后台刷新管理生命周期，不在每次请求前验证有效性。
//   - 如果服务端可能主动吊销 Token，启用 AutoRetryOn401
//   - 如果需要主动失效 Token 缓存（如权限变更后），调用 Client.InvalidateToken
//   - 如果需要主动失效平台数据缓存，调用 Client.InvalidatePlatformCache
//
// # Token 验证契约
//
// VerifyToken 完全委托认证服务端校验 Token 有效性（包括过期、受众等）。
// 客户端不做本地 exp/issuer/audience 校验，避免与服务端规则不一致。
//
// # URL 处理
//
// Request 方法的 URL 参数支持两种格式：
//   - 相对路径（"/api/users"）：与 baseURL 拼接
//   - 完整 URL（"https://other-host.com/api/users"）：直接使用
//
// 绝对 URL 必须使用 HTTPS（除非 AllowInsecure=true），
// 防止 Bearer Token 通过明文 HTTP 泄露。
//
// # 传输安全
//
// Config.Host 必须包含有效的 scheme 和主机名（如 "https://auth.example.com"），
// 无 scheme 的地址会在 Validate() 时返回 ErrInvalidHost。
// 非 HTTPS 地址返回 ErrInsecureHost，开发/测试环境可设置 Config.AllowInsecure = true 放行 http://。
//
// TLS 默认启用证书验证。开发/测试环境如需跳过验证，
// 可通过 Config.TLS 设置 InsecureSkipVerify: true，
// 或使用 NewSkipVerifyHTTPClient。
//
// # 默认行为
//
//   - TLS：Config.TLS 为 nil 时启用证书验证（MinVersion: TLS 1.2）
//   - Logger：WithLogger(nil) 使用 slog.Default()，禁用日志请用 slog.New(slog.NewTextHandler(io.Discard, nil))
//
// # 扩展点
//
//   - CacheStore 接口：自定义远程缓存实现（默认提供 NoopCacheStore 和 RedisCacheStore）
//   - WithHTTPClient：注入自定义 HTTP 客户端
//   - WithObserver：注入 xmetrics.Observer 实现自定义可观测性
//
// # Graceful Shutdown
//
// client.Close(ctx) 取消后台刷新任务、等待所有刷新 goroutine 完成，然后清理本地缓存。
// ctx 参数当前未使用，保留是为了符合项目约定 D-02（统一生命周期接口）。
package xauth
