package xpulsar

import (
	"errors"

	"github.com/omeyang/xkit/internal/mqcore"
)

// 共享错误（从 mqcore 重导出）
var (
	// ErrNilClient 客户端为 nil 错误
	ErrNilClient = mqcore.ErrNilClient

	// ErrNilMessage 消息为 nil 错误
	ErrNilMessage = mqcore.ErrNilMessage

	// ErrNilHandler 处理函数为 nil 错误
	ErrNilHandler = mqcore.ErrNilHandler
)

// Pulsar 特定错误
var (
	// ErrEmptyURL URL 为空错误
	ErrEmptyURL = errors.New("xpulsar: empty URL")

	// ErrNilProducer 生产者为 nil 错误
	ErrNilProducer = errors.New("xpulsar: nil producer")

	// ErrNilConsumer 消费者为 nil 错误
	ErrNilConsumer = errors.New("xpulsar: nil consumer")

	// ErrClosed 客户端已关闭错误（复用 mqcore.ErrClosed，与 xkafka 对齐）
	ErrClosed = mqcore.ErrClosed
)
