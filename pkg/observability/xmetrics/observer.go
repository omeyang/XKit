package xmetrics

import (
	"context"
	"strconv"
)

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

// String 返回 Kind 的可读字符串表示，用于调试和日志输出。
func (k Kind) String() string {
	switch k {
	case KindInternal:
		return "Internal"
	case KindServer:
		return "Server"
	case KindClient:
		return "Client"
	case KindProducer:
		return "Producer"
	case KindConsumer:
		return "Consumer"
	default:
		return "Kind(" + strconv.Itoa(int(k)) + ")"
	}
}

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

// Start 返回 ctx 和空跨度。若 ctx 为 nil，返回 context.Background()。
func (NoopObserver) Start(ctx context.Context, _ SpanOptions) (context.Context, Span) {
	if ctx == nil {
		ctx = context.Background()
	}
	return ctx, NoopSpan{}
}

// NoopSpan 是空跨度实现。
type NoopSpan struct{}

// End 空实现，不做任何处理。
func (NoopSpan) End(_ Result) {}

// Start 使用 observer 开始观测，nil observer 时返回空跨度。
// Start 保证返回非 nil 的 context.Context 和非 nil 的 Span。
// nil ctx 会被替换为 context.Background()；
// 若自定义 Observer 返回 nil Span，Start 会兜底为 [NoopSpan]。
//
// 设计决策: ctx 在入口统一归一化，而非仅在 observer == nil 分支处理。
// 这确保即使自定义 Observer 未处理 nil context，也不会导致 panic。
// 同时对返回的 context 和 span 做兜底检查，防止自定义 Observer 返回 nil 值。
func Start(ctx context.Context, observer Observer, opts SpanOptions) (context.Context, Span) {
	if ctx == nil {
		ctx = context.Background()
	}
	if observer == nil {
		return ctx, NoopSpan{}
	}
	retCtx, span := observer.Start(ctx, opts)
	if retCtx == nil {
		retCtx = ctx
	}
	if span == nil {
		span = NoopSpan{}
	}
	return retCtx, span
}
