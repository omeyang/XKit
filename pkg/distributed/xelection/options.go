package xelection

import (
	"time"

	"github.com/omeyang/xkit/pkg/observability/xlog"
)

// electionOptions Election 构造参数集合。
type electionOptions struct {
	ttlSeconds int
	logger     xlog.Logger
}

// Option 配置 Election 的函数选项。
type Option func(*electionOptions)

const (
	// defaultTTLSeconds 默认 Session TTL（秒）。
	// 15s 与 etcd 官方建议和 Kubernetes leader-election 默认值接近。
	// 过短：KeepAlive 抖动导致误触发；过长：脑裂窗口过大。
	defaultTTLSeconds = 15
)

// defaultOptions 返回默认选项。
func defaultOptions() *electionOptions {
	return &electionOptions{
		ttlSeconds: defaultTTLSeconds,
		logger:     xlog.Default(),
	}
}

// WithTTL 设置 Session TTL（秒）。
// 设计决策：<=0 时静默忽略，保持默认值。与 xdlock.WithEtcdTTL 一致。
func WithTTL(seconds int) Option {
	return func(o *electionOptions) {
		if seconds > 0 {
			o.ttlSeconds = seconds
		}
	}
}

// WithTTLDuration 以 time.Duration 形式设置 Session TTL。
// 小于 1 秒时静默忽略（etcd Session TTL 粒度为秒）。
func WithTTLDuration(d time.Duration) Option {
	return func(o *electionOptions) {
		if s := int(d / time.Second); s > 0 {
			o.ttlSeconds = s
		}
	}
}

// WithLogger 注入结构化日志器。nil 时保留默认 logger。
func WithLogger(l xlog.Logger) Option {
	return func(o *electionOptions) {
		if l != nil {
			o.logger = l
		}
	}
}
