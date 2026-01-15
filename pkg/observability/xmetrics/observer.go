package xmetrics

import "context"

// Kind 表示观测跨度类型。
type Kind int

const (
	// KindInternal 表示内部操作。
	KindInternal Kind = iota
	// KindServer 表示服务端处理。
	KindServer
	// KindClient 表示客户端调用。
	KindClient
	// KindProducer 表示消息生产。
	KindProducer
	// KindConsumer 表示消息消费。
	KindConsumer
)

// Status 表示观测结果状态。
type Status string

const (
	// StatusOK 表示成功。
	StatusOK Status = "ok"
	// StatusError 表示失败。
	StatusError Status = "error"
)

// Attr 表示观测属性。
type Attr struct {
	Key   string
	Value any
}

// SpanOptions 定义观测跨度的创建参数。
type SpanOptions struct {
	// Component 标识组件名称。
	Component string
	// Operation 标识操作名称。
	Operation string
	// Kind 标识跨度类型。
	Kind Kind
	// Attrs 附加属性。
	Attrs []Attr
}

// Result 表示观测跨度结束时的结果。
type Result struct {
	// Status 表示操作状态；为空时根据 Err 推导。
	Status Status
	// Err 表示操作错误。
	Err error
	// Attrs 附加属性。
	Attrs []Attr
}

// Span 表示一次观测跨度。
type Span interface {
	// End 结束观测并记录结果。
	End(result Result)
}

// Observer 定义统一观测接口。
type Observer interface {
	// Start 开始一次观测跨度。
	Start(ctx context.Context, opts SpanOptions) (context.Context, Span)
}

// NoopObserver 是空实现。
type NoopObserver struct{}

// Start 返回原始 ctx 和空跨度。
func (NoopObserver) Start(ctx context.Context, _ SpanOptions) (context.Context, Span) {
	return ctx, NoopSpan{}
}

// NoopSpan 是空跨度实现。
type NoopSpan struct{}

// End 空实现，不做任何处理。
func (NoopSpan) End(_ Result) {}

// Start 使用 observer 开始观测，nil observer 时返回空跨度。
func Start(ctx context.Context, observer Observer, opts SpanOptions) (context.Context, Span) {
	if observer == nil {
		return ctx, NoopSpan{}
	}
	return observer.Start(ctx, opts)
}
