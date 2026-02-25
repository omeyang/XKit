package xmetrics

import "errors"

// NewOTelObserver 返回的错误。
var (
	// ErrCreateCounter 表示创建 OTel Counter 失败。
	ErrCreateCounter = errors.New("xmetrics: create counter failed")
	// ErrCreateHistogram 表示创建 OTel Histogram 失败。
	ErrCreateHistogram = errors.New("xmetrics: create histogram failed")
	// ErrInvalidBuckets 表示 Histogram 桶边界配置无效。
	ErrInvalidBuckets = errors.New("xmetrics: invalid histogram buckets")
	// ErrNilOption 表示传入了 nil 的 Option 函数。
	ErrNilOption = errors.New("xmetrics: nil option")
)
