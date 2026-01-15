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
				// 限流器错误，可以选择放行或拒绝
				// 默认放行以避免限流器故障影响业务
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
