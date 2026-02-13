package xpool

import "log/slog"

// Option 定义 Pool 可选配置函数类型。
type Option func(*options)

type options struct {
	logger *slog.Logger
}

func defaultOptions() options {
	return options{
		logger: slog.Default(),
	}
}

// WithLogger 设置自定义日志记录器。
// 默认使用 slog.Default()。传入 nil 将被忽略，保持使用默认值。
func WithLogger(logger *slog.Logger) Option {
	return func(o *options) {
		if logger != nil {
			o.logger = logger
		}
	}
}
