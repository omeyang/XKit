package xmetrics

import "errors"

// NewOTelObserver 返回的错误。
var (
	// ErrCreateCounter 表示创建 OTel Counter 失败。
	ErrCreateCounter = errors.New("xmetrics: create counter failed")
	// ErrCreateHistogram 表示创建 OTel Histogram 失败。
	ErrCreateHistogram = errors.New("xmetrics: create histogram failed")
)
