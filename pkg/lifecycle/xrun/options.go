package xrun

import (
	"log/slog"
	"os"
)

// Option 配置 Group 的选项函数。
type Option func(*groupOptions)

type groupOptions struct {
	logger          *slog.Logger
	name            string
	signals         []os.Signal
	noSignalHandler bool
}

func defaultOptions() *groupOptions {
	return &groupOptions{
		logger: slog.Default(),
		name:   "xrun",
	}
}

// WithLogger 设置日志记录器。
//
// 用于记录组件启动、关闭等生命周期事件。
// 默认使用 slog.Default()。
func WithLogger(logger *slog.Logger) Option {
	return func(o *groupOptions) {
		if logger != nil {
			o.logger = logger
		}
	}
}

// WithName 设置 Group 名称。
//
// 用于日志记录中标识不同的 Group。
// 默认值为 "xrun"。
func WithName(name string) Option {
	return func(o *groupOptions) {
		if name != "" {
			o.name = name
		}
	}
}

// WithSignals 设置 Run/RunWithOptions/RunServices/RunServicesWithOptions 监听的信号列表。
//
// 默认监听 DefaultSignals()（SIGHUP、SIGINT、SIGTERM、SIGQUIT）。
// 传入自定义列表可覆盖默认行为。
//
// 示例：
//
//	xrun.RunWithOptions(ctx, []xrun.Option{
//	    xrun.WithSignals([]os.Signal{syscall.SIGINT, syscall.SIGTERM}),
//	}, myService)
func WithSignals(signals []os.Signal) Option {
	// 设计决策: 在创建时拷贝，避免调用方后续修改切片导致配置漂移。
	copied := append([]os.Signal(nil), signals...)
	return func(o *groupOptions) {
		o.signals = copied
	}
}

// WithoutSignalHandler 禁用自动信号处理。
//
// 使用此选项后，Run/RunWithOptions/RunServices/RunServicesWithOptions
// 不会注册信号监听，调用方需自行管理信号处理。
//
// 示例：
//
//	xrun.RunWithOptions(ctx, []xrun.Option{
//	    xrun.WithoutSignalHandler(),
//	}, myService)
func WithoutSignalHandler() Option {
	return func(o *groupOptions) {
		o.noSignalHandler = true
	}
}
