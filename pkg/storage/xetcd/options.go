package xetcd

import (
	"context"
	"crypto/tls"
	"time"
)

// options 内部选项结构。
type options struct {
	ctx           context.Context
	healthCheck   bool
	healthTimeout time.Duration
	tlsConfig     *tls.Config
}

// defaultOptions 返回默认选项。
func defaultOptions() *options {
	return &options{
		ctx:           context.Background(),
		healthCheck:   false,
		healthTimeout: 10 * time.Second,
	}
}

// Option 定义客户端配置选项。
type Option func(*options)

// WithContext 设置客户端上下文，用于控制客户端的生命周期。
func WithContext(ctx context.Context) Option {
	return func(o *options) {
		if ctx != nil {
			o.ctx = ctx
		}
	}
}

// WithHealthCheck 创建后执行健康检查。
// 设置为 true 时，会在创建客户端后执行一次 Get 操作验证连接。
// timeout 为健康检查超时时间，默认 10 秒。
func WithHealthCheck(enabled bool, timeout time.Duration) Option {
	return func(o *options) {
		o.healthCheck = enabled
		if timeout > 0 {
			o.healthTimeout = timeout
		}
	}
}

// WithTLS 设置 TLS 配置，用于启用安全连接。
func WithTLS(config *tls.Config) Option {
	return func(o *options) {
		o.tlsConfig = config
	}
}
