package xkafka

import (
	"errors"

	"github.com/omeyang/xkit/internal/mqcore"
)

// 重导出共享错误（xkafka 和 xpulsar 共同使用）
var (
	// ErrNilClient 表示传入的客户端为空。
	ErrNilClient = mqcore.ErrNilClient

	// ErrNilMessage 表示传入的消息为空。
	ErrNilMessage = mqcore.ErrNilMessage

	// ErrNilHandler 表示传入的处理函数为空。
	ErrNilHandler = mqcore.ErrNilHandler

	// ErrClosed 表示客户端已关闭。
	ErrClosed = mqcore.ErrClosed
)

// Kafka 特有错误
var (
	// ErrNilConfig 表示传入的配置为空。
	ErrNilConfig = errors.New("xkafka: nil config")

	// ErrHealthCheckFailed 表示健康检查失败。
	ErrHealthCheckFailed = errors.New("xkafka: health check failed")

	// ErrDLQPolicyRequired 表示 DLQ 策略不能为空。
	ErrDLQPolicyRequired = errors.New("xkafka: DLQ policy is required")

	// ErrDLQTopicRequired 表示 DLQ Topic 不能为空。
	ErrDLQTopicRequired = errors.New("xkafka: DLQ topic is required")

	// ErrRetryPolicyRequired 表示重试策略不能为空。
	ErrRetryPolicyRequired = errors.New("xkafka: retry policy is required")

	// ErrFlushTimeout 表示消息刷新超时。
	ErrFlushTimeout = errors.New("xkafka: flush timeout")

	// ErrEmptyTopics 表示订阅的主题列表为空。
	ErrEmptyTopics = errors.New("xkafka: empty topics")
)
