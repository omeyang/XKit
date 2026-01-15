package xpulsar

import (
	"context"
	"time"

	"github.com/omeyang/xkit/pkg/observability/xmetrics"

	"github.com/apache/pulsar-client-go/pulsar"
)

// =============================================================================
// Client 接口
// =============================================================================

// Client 定义 Pulsar 客户端接口。
// 通过 Client() 方法暴露底层 pulsar.Client，可使用所有原生 API。
type Client interface {
	// Client 返回底层的 pulsar.Client。
	// 用于执行所有 Pulsar 操作。
	Client() pulsar.Client

	// Health 执行健康检查。
	// 通过获取 Broker 元数据验证连接状态。
	Health(ctx context.Context) error

	// CreateProducer 创建 Pulsar 生产者。
	// 这是 Client().CreateProducer 的便捷方法。
	CreateProducer(options pulsar.ProducerOptions) (pulsar.Producer, error)

	// Subscribe 创建 Pulsar 消费者。
	// 这是 Client().Subscribe 的便捷方法。
	Subscribe(options pulsar.ConsumerOptions) (pulsar.Consumer, error)

	// Stats 返回客户端统计信息。
	Stats() Stats

	// Close 优雅关闭客户端。
	// 会关闭所有生产者和消费者。
	Close() error
}

// Stats 包含 Pulsar 客户端的统计信息。
type Stats struct {
	// Connected 是否已连接。
	Connected bool
	// ProducersCount 活跃的生产者数量。
	ProducersCount int
	// ConsumersCount 活跃的消费者数量。
	ConsumersCount int
}

// =============================================================================
// 工厂函数
// =============================================================================

// NewClient 创建 Pulsar 客户端实例。
// url 是 Pulsar 服务地址，如 "pulsar://localhost:6650"。
func NewClient(url string, opts ...Option) (Client, error) {
	if url == "" {
		return nil, ErrEmptyURL
	}

	options := defaultOptions()
	for _, opt := range opts {
		opt(options)
	}

	clientOptions := pulsar.ClientOptions{
		URL:                     url,
		ConnectionTimeout:       options.ConnectionTimeout,
		OperationTimeout:        options.OperationTimeout,
		MaxConnectionsPerBroker: options.MaxConnectionsPerBroker,
	}

	// 可选认证配置
	if options.Authentication != nil {
		clientOptions.Authentication = options.Authentication
	}

	// 可选 TLS 配置
	if options.TLSTrustCertsFilePath != "" {
		clientOptions.TLSTrustCertsFilePath = options.TLSTrustCertsFilePath
	}
	if options.TLSAllowInsecureConnection {
		clientOptions.TLSAllowInsecureConnection = true
	}

	client, err := pulsar.NewClient(clientOptions)
	if err != nil {
		return nil, err
	}

	return &clientWrapper{
		client:  client,
		options: options,
	}, nil
}

// =============================================================================
// 选项
// =============================================================================

// clientOptions 包含 Pulsar 客户端的配置选项。
type clientOptions struct {
	Tracer                     Tracer
	Observer                   xmetrics.Observer
	ConnectionTimeout          time.Duration
	OperationTimeout           time.Duration
	MaxConnectionsPerBroker    int
	Authentication             pulsar.Authentication
	TLSTrustCertsFilePath      string
	TLSAllowInsecureConnection bool
	HealthTimeout              time.Duration
}

func defaultOptions() *clientOptions {
	return &clientOptions{
		Tracer:                  NoopTracer{},
		Observer:                xmetrics.NoopObserver{},
		ConnectionTimeout:       10 * time.Second,
		OperationTimeout:        30 * time.Second,
		MaxConnectionsPerBroker: 1,
		HealthTimeout:           5 * time.Second,
	}
}

// Option 定义 Pulsar 客户端的配置选项函数类型。
type Option func(*clientOptions)

// WithTracer 设置链路追踪器。
func WithTracer(tracer Tracer) Option {
	return func(o *clientOptions) {
		if tracer != nil {
			o.Tracer = tracer
		}
	}
}

// WithObserver 设置统一观测接口。
func WithObserver(observer xmetrics.Observer) Option {
	return func(o *clientOptions) {
		if observer != nil {
			o.Observer = observer
		}
	}
}

// WithConnectionTimeout 设置连接超时时间。
func WithConnectionTimeout(d time.Duration) Option {
	return func(o *clientOptions) {
		if d > 0 {
			o.ConnectionTimeout = d
		}
	}
}

// WithOperationTimeout 设置操作超时时间。
func WithOperationTimeout(d time.Duration) Option {
	return func(o *clientOptions) {
		if d > 0 {
			o.OperationTimeout = d
		}
	}
}

// WithMaxConnectionsPerBroker 设置每个 Broker 的最大连接数。
func WithMaxConnectionsPerBroker(n int) Option {
	return func(o *clientOptions) {
		if n > 0 {
			o.MaxConnectionsPerBroker = n
		}
	}
}

// WithAuthentication 设置认证方式。
func WithAuthentication(auth pulsar.Authentication) Option {
	return func(o *clientOptions) {
		o.Authentication = auth
	}
}

// WithTLS 设置 TLS 配置。
func WithTLS(trustCertsFilePath string, allowInsecure bool) Option {
	return func(o *clientOptions) {
		o.TLSTrustCertsFilePath = trustCertsFilePath
		o.TLSAllowInsecureConnection = allowInsecure
	}
}

// WithHealthTimeout 设置健康检查超时时间。
func WithHealthTimeout(d time.Duration) Option {
	return func(o *clientOptions) {
		if d > 0 {
			o.HealthTimeout = d
		}
	}
}
