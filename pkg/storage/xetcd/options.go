package xetcd

import (
	"context"
	"crypto/tls"
	"time"
)

// defaultHealthCheckKey 默认健康检查 key。
// 设计决策: 使用可配置的 key 而非硬编码，以支持 RBAC 前缀授权场景。
// 在 RBAC 中，账号可能仅被授权访问特定前缀（如 "/app/"），
// 此时可通过 WithHealthCheckKey 设置为授权范围内的 key。
const defaultHealthCheckKey = "xetcd-health-check"

// options 内部选项结构。
type options struct {
	ctx            context.Context
	healthCheck    bool
	healthTimeout  time.Duration
	healthCheckKey string
	tlsConfig      *tls.Config
}

// defaultOptions 返回默认选项。
func defaultOptions() *options {
	return &options{
		ctx:            context.Background(),
		healthCheck:    false,
		healthTimeout:  10 * time.Second,
		healthCheckKey: defaultHealthCheckKey,
	}
}

// Option 定义客户端配置选项。
type Option func(*options)

// WithContext 设置客户端初始化上下文，用于健康检查等创建阶段的操作。
// 设计决策: 此 context 仅影响 NewClient 内部的操作（如健康检查），
// 不控制客户端的长期连接生命周期。客户端关闭请使用 Close()。
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
//
// ⚠️ 注意：健康检查使用 Get 操作（而非 gRPC 健康检查协议），
// 在启用 RBAC 认证的 etcd 集群中，如果凭证无效会直接导致 NewClient 失败。
// 这是预期行为——认证失败应在初始化阶段尽早暴露。
func WithHealthCheck(enabled bool, timeout time.Duration) Option {
	return func(o *options) {
		o.healthCheck = enabled
		if timeout > 0 {
			o.healthTimeout = timeout
		}
	}
}

// WithHealthCheckKey 设置健康检查使用的 key。
// 默认为 "xetcd-health-check"。
// 在启用 RBAC 前缀授权的 etcd 集群中，如果账号仅被授权访问特定前缀，
// 应将此 key 设置为授权范围内的路径（如 "/app/health"），
// 否则健康检查会因权限不足而失败。
func WithHealthCheckKey(key string) Option {
	return func(o *options) {
		if key != "" {
			o.healthCheckKey = key
		}
	}
}

// WithTLS 设置 TLS 配置，用于启用安全连接。
func WithTLS(config *tls.Config) Option {
	return func(o *options) {
		o.tlsConfig = config
	}
}
