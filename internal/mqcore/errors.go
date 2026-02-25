package mqcore

import "errors"

// 共享错误定义（xkafka 和 xpulsar 共同使用）。
var (
	// ErrNilClient 表示传入的客户端为空。
	ErrNilClient = errors.New("mq: nil client")

	// ErrNilMessage 表示传入的消息为空。
	ErrNilMessage = errors.New("mq: nil message")

	// ErrNilHandler 表示传入的处理函数为空。
	ErrNilHandler = errors.New("mq: nil handler")

	// ErrClosed 表示客户端已关闭。
	ErrClosed = errors.New("mq: client closed")
)
