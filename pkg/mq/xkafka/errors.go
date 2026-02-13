package xkafka

import (
	"errors"

	"github.com/omeyang/xkit/internal/mqcore"
)

// 重导出共享错误
var (
	// ErrNilConfig 表示传入的配置为空。
	ErrNilConfig = mqcore.ErrNilConfig

	// ErrNilClient 表示传入的客户端为空。
	ErrNilClient = mqcore.ErrNilClient

	// ErrNilMessage 表示传入的消息为空。
	ErrNilMessage = mqcore.ErrNilMessage

	// ErrNilHandler 表示传入的处理函数为空。
	ErrNilHandler = mqcore.ErrNilHandler

	// ErrClosed 表示客户端已关闭。
	ErrClosed = mqcore.ErrClosed

	// ErrHealthCheckFailed 表示健康检查失败。
	ErrHealthCheckFailed = mqcore.ErrHealthCheckFailed

	// ErrDLQPolicyRequired 表示 DLQ 策略不能为空。
	ErrDLQPolicyRequired = mqcore.ErrDLQPolicyRequired

	// ErrDLQTopicRequired 表示 DLQ Topic 不能为空。
	ErrDLQTopicRequired = mqcore.ErrDLQTopicRequired

	// ErrRetryPolicyRequired 表示重试策略不能为空。
	ErrRetryPolicyRequired = mqcore.ErrRetryPolicyRequired
)

// Kafka 特有错误
var (
	// ErrFlushTimeout 表示消息刷新超时。
	ErrFlushTimeout = errors.New("xkafka: flush timeout")

	// ErrEmptyTopics 表示订阅的主题列表为空。
	ErrEmptyTopics = errors.New("xkafka: empty topics")
)
