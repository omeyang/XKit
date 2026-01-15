package mqcore

import "errors"

// 共享错误定义。
// 这些错误可以被 xkafka 和 xpulsar 包重导出。
var (
	// ErrNilConfig 表示传入的配置为空。
	ErrNilConfig = errors.New("mq: nil config")

	// ErrNilClient 表示传入的客户端为空。
	ErrNilClient = errors.New("mq: nil client")

	// ErrNilMessage 表示传入的消息为空。
	ErrNilMessage = errors.New("mq: nil message")

	// ErrNilHandler 表示传入的处理函数为空。
	ErrNilHandler = errors.New("mq: nil handler")

	// ErrClosed 表示客户端已关闭。
	ErrClosed = errors.New("mq: client closed")

	// ErrHealthCheckFailed 表示健康检查失败。
	ErrHealthCheckFailed = errors.New("mq: health check failed")

	// ErrDLQPolicyRequired 表示 DLQ 策略不能为空。
	ErrDLQPolicyRequired = errors.New("mq: DLQ policy is required")

	// ErrDLQTopicRequired 表示 DLQ Topic 不能为空。
	ErrDLQTopicRequired = errors.New("mq: DLQ topic is required")

	// ErrRetryPolicyRequired 表示重试策略不能为空。
	ErrRetryPolicyRequired = errors.New("mq: retry policy is required")
)
