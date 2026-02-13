package xlimit

import (
	"net/http"
)

// HTTPMiddleware 创建 HTTP 限流中间件
//
// 示例:
//
//	limiter, _ := xlimit.New(redisClient, xlimit.WithRules(...))
//	mux := http.NewServeMux()
//	mux.Handle("/api/", xlimit.HTTPMiddleware(limiter)(apiHandler))
func HTTPMiddleware(limiter Limiter, opts ...MiddlewareOption) func(http.Handler) http.Handler {
	// 应用选项
	mopts := defaultMiddlewareOptions()
	for _, opt := range opts {
		opt(mopts)
	}
	mopts.sanitize()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 检查是否跳过
			if mopts.SkipFunc != nil && mopts.SkipFunc(r) {
				next.ServeHTTP(w, r)
				return
			}

			// 提取限流键
			key := mopts.KeyExtractor.Extract(r)

			// 执行限流检查
			result, err := limiter.Allow(r.Context(), key)
			if err != nil {
				// 设计决策: 优先检查 result 是否携带拒绝信息（如 FallbackClose 策略
				// 返回 Allowed=false + ErrRedisUnavailable）。仅当 result 为空时
				// 才 fail-open（限流器内部错误不阻塞业务请求）。
				if result != nil && !result.Allowed {
					if mopts.EnableHeaders {
						result.SetHeaders(w)
					}
					mopts.DenyHandler(w, r, result)
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			// 添加限流头（如果启用）
			if mopts.EnableHeaders {
				result.SetHeaders(w)
			}

			// 检查是否被限流
			if !result.Allowed {
				mopts.DenyHandler(w, r, result)
				return
			}

			// 放行请求
			next.ServeHTTP(w, r)
		})
	}
}

// HTTPMiddlewareFunc 创建 HTTP 限流中间件（函数式）
// 适用于需要 http.HandlerFunc 的场景
func HTTPMiddlewareFunc(limiter Limiter, opts ...MiddlewareOption) func(http.HandlerFunc) http.HandlerFunc {
	middleware := HTTPMiddleware(limiter, opts...)
	return func(next http.HandlerFunc) http.HandlerFunc {
		return middleware(next).ServeHTTP
	}
}
