package mqcore

import "context"

// Tracer 定义链路追踪接口。
// 用于在消息生产/消费时注入和提取追踪信息。
//
// 实现者应使用 W3C Trace Context 标准的 Header 名称：
//   - traceparent: 追踪 ID 和 Span ID
//   - tracestate: 厂商特定信息
type Tracer interface {
	// Inject 将追踪信息注入到消息头。
	// ctx 包含当前的追踪上下文。
	// headers 是消息头的 key-value 映射，实现者应向其中添加追踪信息。
	Inject(ctx context.Context, headers map[string]string)

	// Extract 从消息头提取追踪信息。
	// headers 是消息头的 key-value 映射。
	// 返回包含追踪上下文的 Context。
	Extract(headers map[string]string) context.Context
}

// NoopTracer 是 Tracer 的空实现。
// 当用户不需要链路追踪时使用。
type NoopTracer struct{}

// Inject 空实现，不做任何操作。
func (NoopTracer) Inject(_ context.Context, _ map[string]string) {}

// Extract 空实现，返回空的 Background Context。
func (NoopTracer) Extract(_ map[string]string) context.Context {
	return context.Background()
}

// 确保 NoopTracer 实现 Tracer 接口。
var _ Tracer = NoopTracer{}
