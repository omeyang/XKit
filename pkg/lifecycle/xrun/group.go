package xrun

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"time"

	"golang.org/x/sync/errgroup"
)

// Group 基于 errgroup + context 管理多个服务的并发运行和协调关闭。
//
// 当任一服务返回错误或 context 被取消时，所有服务都会收到取消信号。
//
// Go、GoWithName、Cancel 可安全地从多个 goroutine 并发调用。
// Wait 应仅调用一次。
//
// 使用方式：
//
//	g, ctx := xrun.NewGroup(ctx)
//	g.Go(func(ctx context.Context) error {
//	    return runServer(ctx)
//	})
//	g.Go(func(ctx context.Context) error {
//	    return runWorker(ctx)
//	})
//	if err := g.Wait(); err != nil {
//	    log.Fatal(err)
//	}
type Group struct {
	eg       *errgroup.Group
	ctx      context.Context
	causeCtx context.Context
	cancel   context.CancelCauseFunc
	opts     *groupOptions
}

// NewGroup 创建新的 Group。
//
// 返回 Group 和派生的 context。当任一 goroutine 返回错误时，
// 返回的 context 会被取消。
//
// 示例：
//
//	g, ctx := xrun.NewGroup(context.Background(),
//	    xrun.WithName("my-service"),
//	    xrun.WithLogger(logger),
//	)
func NewGroup(ctx context.Context, opts ...Option) (*Group, context.Context) {
	// 设计决策: nil context 归一化为 context.Background()，
	// 防止 context.WithCancelCause(nil) panic（Go 标准库会 panic）。
	// 不改变 API 签名（保持与 errgroup.WithContext 对齐），因此选择静默归一化。
	if ctx == nil {
		ctx = context.Background()
	}

	options := defaultOptions()
	for _, opt := range opts {
		// 设计决策: 静默跳过 nil Option，与 WithLogger(nil)/WithName("") 的防御性
		// 行为一致。不返回错误以保持与 errgroup.WithContext 对齐的 API 签名。
		if opt == nil {
			continue
		}
		opt(options)
	}

	causeCtx, cancel := context.WithCancelCause(ctx)
	eg, egCtx := errgroup.WithContext(causeCtx)

	return &Group{
		eg:       eg,
		ctx:      egCtx,
		causeCtx: causeCtx,
		cancel:   cancel,
		opts:     options,
	}, egCtx
}

// Go 启动一个 goroutine 执行 fn。
//
// fn 应该监听 ctx.Done() 以响应取消信号：
//
//	g.Go(func(ctx context.Context) error {
//	    for {
//	        select {
//	        case <-ctx.Done():
//	            return ctx.Err()
//	        default:
//	            doWork()
//	        }
//	    }
//	})
//
// 当 fn 返回非 nil 错误时，会触发所有其他 goroutine 的取消。
func (g *Group) Go(fn func(ctx context.Context) error) {
	g.eg.Go(func() error {
		if fn == nil {
			return ErrNilFunc
		}
		return fn(g.ctx)
	})
}

// GoWithName 与 Go 相同，但会在日志中记录名称。
// name 为空字符串时仍有效，但日志中会显示 service=""，建议传入有意义的名称。
func (g *Group) GoWithName(name string, fn func(ctx context.Context) error) {
	g.eg.Go(func() error {
		if fn == nil {
			return ErrNilFunc
		}
		g.opts.logger.Debug("service starting",
			slog.String("group", g.opts.name),
			slog.String("service", name),
		)
		err := fn(g.ctx)
		if err != nil && !errors.Is(err, context.Canceled) {
			g.opts.logger.Warn("service exited with error",
				slog.String("group", g.opts.name),
				slog.String("service", name),
				slog.Any("error", err),
			)
		} else {
			g.opts.logger.Debug("service stopped",
				slog.String("group", g.opts.name),
				slog.String("service", name),
			)
		}
		return err
	})
}

// Wait 等待所有 goroutine 完成。
//
// 返回第一个非 nil 错误（如果有）。
// 如果错误是 context.Canceled，则优先返回 context.Cause ——
// 这样 Cancel(cause) 或信号处理设置的退出原因不会丢失。
// 如果没有显式原因（普通的 context 取消），返回 nil。
//
// 即使所有服务返回 nil，Cancel(cause) 设置的退出原因仍然会被返回。
// 这确保调用方始终能基于退出原因做分类决策。
func (g *Group) Wait() error {
	// 设计决策: 释放 causeCtx 的 context 资源。CancelCauseFunc 是幂等的——
	// 若已通过 Cancel() 或信号处理调用过则为空操作。defer 确保在所有
	// cause 检查完成后才执行，不影响返回值语义。
	defer g.cancel(nil)

	g.opts.logger.Debug("waiting for services",
		slog.String("group", g.opts.name),
	)

	err := g.eg.Wait()

	g.opts.logger.Debug("all services stopped",
		slog.String("group", g.opts.name),
	)

	// 过滤 context.Canceled，但保留显式的 cancel cause。
	// 通过 causeCtx（而非 errgroup 的 ctx）判断取消来源：
	//   - causeCtx 被取消 → Group 主动 Cancel 或父 context 取消，按原逻辑过滤
	//   - causeCtx 未被取消 → context.Canceled 来自服务内部，不过滤
	if errors.Is(err, context.Canceled) {
		if g.causeCtx.Err() != nil {
			// Group 被主动取消，返回显式 cause（如 SignalError），否则返回 nil
			if cause := context.Cause(g.causeCtx); cause != nil && !errors.Is(cause, context.Canceled) {
				return cause
			}
			return nil
		}
		// causeCtx 未被取消 → context.Canceled 来自服务内部，不过滤
		return err
	}

	// 当所有服务返回 nil 时，仍需检查是否有显式 Cancel(cause)。
	// 例如：Cancel(customErr) 后服务返回 nil 而非 ctx.Err()，
	// errgroup.Wait() 返回 nil，但 cause 不应丢失。
	if err == nil && g.causeCtx.Err() != nil {
		if cause := context.Cause(g.causeCtx); cause != nil && !errors.Is(cause, context.Canceled) {
			return cause
		}
	}

	return err
}

// Cancel 主动取消所有 goroutine。
//
// cause 会作为 context 的取消原因，Wait() 会通过 context.Cause
// 返回该原因（而非 nil）。如果 cause 为 nil，Wait() 返回 nil。
//
// 注意：cause 不应包装 context.Canceled（例如 fmt.Errorf("...: %w", context.Canceled)），
// 否则 Wait() 会将其视为普通取消而过滤掉。有语义的退出原因应使用独立错误类型
// （如 SignalError、自定义业务错误）。
//
// 用于主动触发关闭，比如收到信号后。
func (g *Group) Cancel(cause error) {
	g.cancel(cause)
}

// Context 返回 Group 的 context。
func (g *Group) Context() context.Context {
	return g.ctx
}

// ----------------------------------------------------------------------------
// 便捷函数
// ----------------------------------------------------------------------------

// runGroup 是 Run/RunWithOptions/RunServices/RunServicesWithOptions 的共享实现。
//
// 默认自动注册信号监听服务：当收到配置的信号（默认 DefaultSignals）时，
// 通过 Cancel(&SignalError{Signal: sig}) 传播退出原因，
// Wait() 会返回 *SignalError。
//
// 可通过 WithSignals 自定义信号列表，或通过 WithoutSignalHandler 禁用信号处理。
func runGroup(ctx context.Context, opts []Option, setup func(g *Group)) error {
	g, _ := NewGroup(ctx, opts...)

	// 信号处理服务（可通过 WithoutSignalHandler 禁用）
	if !g.opts.noSignalHandler {
		signals := g.opts.signals
		// 设计决策: 空切片与 nil 等价，均使用默认信号列表。
		// signal.Notify(ch) 无参调用会订阅所有信号，这不是用户预期行为。
		// 如需禁用信号处理，应使用 WithoutSignalHandler()。
		if len(signals) == 0 {
			signals = DefaultSignals()
		}

		g.Go(func(ctx context.Context) error {
			testc := testSigChan(ctx)
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, signals...)
			defer signal.Stop(sigCh)

			var sig os.Signal
			select {
			case sig = <-testc:
			case sig = <-sigCh:
			case <-ctx.Done():
				return ctx.Err()
			}

			g.opts.logger.Info("received signal",
				slog.String("group", g.opts.name),
				slog.String("signal", sig.String()),
			)
			g.cancel(&SignalError{Signal: sig})
			return nil
		})
	}

	setup(g)
	return g.Wait()
}

// Run 是最常用的启动模式：监听信号 + 运行服务。
//
// 当收到 SIGHUP/SIGINT/SIGTERM/SIGQUIT 时，ctx 会被取消，
// 所有服务应该优雅关闭。Run 返回 *SignalError 表示信号退出。
//
// 示例：
//
//	err := xrun.Run(context.Background(), func(ctx context.Context) error {
//	    server := &http.Server{Addr: ":8080"}
//	    go func() {
//	        <-ctx.Done()
//	        server.Shutdown(context.Background())
//	    }()
//	    return server.ListenAndServe()
//	})
//	if errors.Is(err, xrun.ErrSignal) {
//	    log.Println("received signal, shutting down")
//	}
//
// 对于 HTTP 服务器，推荐使用 [HTTPServer] 辅助函数，
// 它封装了完整的优雅关闭和错误传播逻辑。
func Run(ctx context.Context, services ...func(ctx context.Context) error) error {
	return runGroup(ctx, nil, func(g *Group) {
		for _, svc := range services {
			g.Go(svc)
		}
	})
}

// RunWithOptions 与 Run 相同，但支持配置选项。
func RunWithOptions(ctx context.Context, opts []Option, services ...func(ctx context.Context) error) error {
	return runGroup(ctx, opts, func(g *Group) {
		for _, svc := range services {
			g.Go(svc)
		}
	})
}

// ----------------------------------------------------------------------------
// Service 接口
// ----------------------------------------------------------------------------

// Service 定义可管理的服务接口。
//
// 实现此接口的服务可以使用 RunServices 统一管理。
type Service interface {
	// Run 启动服务，阻塞直到 ctx 被取消或发生错误。
	// 当 ctx 被取消时，应该优雅关闭并返回。
	Run(ctx context.Context) error
}

// ServiceFunc 将函数转换为 Service 接口。
type ServiceFunc func(ctx context.Context) error

// Run 实现 Service 接口。
func (f ServiceFunc) Run(ctx context.Context) error {
	return f(ctx)
}

// RunServices 运行多个 Service，监听信号并协调关闭。
//
// 普通函数可通过 ServiceFunc 适配为 Service 接口：
//
//	svc := xrun.ServiceFunc(func(ctx context.Context) error { ... })
//
// 示例：
//
//	err := xrun.RunServices(ctx,
//	    httpServer,
//	    grpcServer,
//	    kafkaConsumer,
//	)
func RunServices(ctx context.Context, services ...Service) error {
	return runGroup(ctx, nil, func(g *Group) {
		addServices(g, services)
	})
}

// RunServicesWithOptions 与 RunServices 相同，但支持配置选项。
//
// 示例：
//
//	err := xrun.RunServicesWithOptions(ctx, []xrun.Option{
//	    xrun.WithName("my-app"),
//	    xrun.WithSignals([]os.Signal{syscall.SIGINT, syscall.SIGTERM}),
//	}, httpServer, grpcServer)
func RunServicesWithOptions(ctx context.Context, opts []Option, services ...Service) error {
	return runGroup(ctx, opts, func(g *Group) {
		addServices(g, services)
	})
}

// addServices 将服务列表注册到 Group，nil service 返回 ErrNilService。
func addServices(g *Group, services []Service) {
	for _, svc := range services {
		if svc == nil {
			g.Go(func(ctx context.Context) error { return ErrNilService })
			continue
		}
		g.Go(svc.Run)
	}
}

// ----------------------------------------------------------------------------
// HTTP Server 辅助
// ----------------------------------------------------------------------------

// HTTPServer 将 http.Server 包装为支持优雅关闭的服务函数。
//
// shutdownTimeout 为 0 或负数时表示无超时限制，Shutdown 将等待所有在途请求
// 完成后才返回。如需禁用等待，请传入一个较短的超时值。
//
// 示例：
//
//	server := &http.Server{Addr: ":8080", Handler: mux}
//	err := xrun.Run(ctx, xrun.HTTPServer(server, 10*time.Second))
func HTTPServer(server HTTPServerInterface, shutdownTimeout time.Duration) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		if server == nil {
			return ErrNilServer
		}
		// 用 buffered channel 传递 shutdown 结果
		shutdownErrCh := make(chan error, 1)
		// listenDone 用于通知 shutdown goroutine: ListenAndServe 已返回，
		// 避免在外部关闭或启动失败场景下 goroutine 永久阻塞。
		listenDone := make(chan struct{})

		// 启动关闭监听
		go func() {
			select {
			case <-ctx.Done():
				shutdownCtx := context.Background()
				if shutdownTimeout > 0 {
					var cancel context.CancelFunc
					shutdownCtx, cancel = context.WithTimeout(shutdownCtx, shutdownTimeout)
					defer cancel()
				}
				shutdownErrCh <- server.Shutdown(shutdownCtx)
			case <-listenDone:
				// ListenAndServe 已返回（外部关闭或启动失败），无需 Shutdown。
			}
		}()

		// 启动服务器
		err := server.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			// 设计决策: 通过三路 select 区分关闭来源：
			//   1. shutdownErrCh 有值 → ctx 驱动的关闭已完成，返回 shutdown 结果
			//   2. ctx.Done() 已关闭 → ctx 驱动的关闭进行中，等待 shutdown 结果
			//   3. default → 外部直接调用 server.Shutdown/Close，ctx 未取消，
			//      通知 goroutine 退出并返回 nil
			select {
			case shutdownErr := <-shutdownErrCh:
				return shutdownErr
			case <-ctx.Done():
				return <-shutdownErrCh
			default:
				close(listenDone)
				return nil
			}
		}
		// 非 ErrServerClosed 错误（如端口占用），通知 goroutine 退出。
		close(listenDone)
		return err
	}
}

// HTTPServerInterface 定义 HTTP 服务器接口。
//
// 设计决策: 接口名使用 Interface 后缀是因为 HTTPServer 已被同名便捷函数占用。
// 重命名函数为 ServeHTTP 会与 http.Handler.ServeHTTP 混淆，权衡后保持现状。
//
// *http.Server 天然满足此接口。导出此接口以支持自定义服务器实现和测试 mock。
type HTTPServerInterface interface {
	ListenAndServe() error
	Shutdown(ctx context.Context) error
}
