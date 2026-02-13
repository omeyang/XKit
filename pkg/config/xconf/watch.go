package xconf

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"runtime/debug"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// WatchCallback 文件变更回调函数
// 当配置文件发生变更时调用，err 表示重载是否成功
type WatchCallback func(cfg Config, err error)

// Watcher 配置文件监视器
// 监控配置文件变更并自动重载
type Watcher struct {
	cfg      *koanfConfig
	watcher  *fsnotify.Watcher
	callback WatchCallback
	debounce time.Duration
	ctx      context.Context
	cancel   context.CancelFunc
	mu       sync.Mutex
	running  bool
	stopped  bool        // 标记资源是否已释放，确保 Stop() 幂等
	timer    *time.Timer // debounce 定时器，Stop() 时需要取消

	// 设计决策: 两层 WaitGroup 确保 Stop() 返回后不再有回调执行。
	// runWg 跟踪 run() goroutine 生命周期（handleError 等直接回调）。
	// callbackWg 跟踪 debounce AfterFunc 中的 in-flight 回调。
	runWg      sync.WaitGroup // run goroutine 生命周期
	callbackWg sync.WaitGroup // in-flight 防抖回调
}

// WatchOption 监视器配置选项
type WatchOption func(*watchOptions)

type watchOptions struct {
	debounce time.Duration
}

func defaultWatchOptions() *watchOptions {
	return &watchOptions{
		debounce: 100 * time.Millisecond, // 默认防抖时间
	}
}

// validate 校验监视器选项。
func (o *watchOptions) validate() error {
	if o.debounce <= 0 {
		return fmt.Errorf("%w: %v", ErrInvalidDebounce, o.debounce)
	}
	return nil
}

// WithDebounce 设置防抖时间
// 在指定时间内的多次变更只触发一次重载
// 默认值为 100ms，适合大多数场景
func WithDebounce(d time.Duration) WatchOption {
	return func(o *watchOptions) {
		o.debounce = d
	}
}

// Watch 创建配置文件监视器
//
// 监控配置文件变更并自动调用 Reload() 重新加载配置。
// 当配置文件变更时，会调用 callback 通知调用方。
//
// 参数:
//   - cfg: 要监视的配置实例（必须是通过 New() 从文件创建的）
//   - callback: 变更回调函数
//   - opts: 可选配置
//
// 注意:
//   - 只能监视从文件创建的 Config（通过 New() 创建）
//   - 从 bytes 创建的 Config 不支持监视
//   - 返回的 Watcher 需要调用 Start() 开始监视，Stop() 停止监视
//   - Stop() 保证返回后不再有回调执行
//
// 设计决策: cfg 参数类型为 Config 接口，但内部需要类型断言为 *koanfConfig。
// 这是因为 Watch 需要访问内部字段（path、isBytes），而这些不属于 Config 接口。
// 保留 Config 接口参数是为了 API 一致性——用户传入 New() 返回的 Config 即可。
// 如果传入非 xconf 实现的 Config，会返回 ErrWatchFailed 错误。
//
// 示例:
//
//	cfg, _ := xconf.New("/etc/app/config.yaml")
//	w, err := xconf.Watch(cfg, func(c xconf.Config, err error) {
//	    if err != nil {
//	        log.Printf("reload failed: %v", err)
//	        return
//	    }
//	    log.Println("config reloaded")
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer w.Stop()
//	w.Start()
func Watch(cfg Config, callback WatchCallback, opts ...WatchOption) (*Watcher, error) {
	kc, ok := cfg.(*koanfConfig)
	if !ok {
		return nil, fmt.Errorf("%w: unsupported config type %T", ErrWatchFailed, cfg)
	}

	if callback == nil {
		return nil, ErrNilCallback
	}

	if kc.isBytes {
		return nil, ErrNotFromFile
	}

	if kc.path == "" {
		return nil, ErrEmptyPath
	}

	// 应用选项
	options := defaultWatchOptions()
	for _, opt := range opts {
		opt(options)
	}
	if err := options.validate(); err != nil {
		return nil, err
	}

	// 创建 fsnotify watcher
	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrWatchFailed, err)
	}

	// 监视配置文件所在目录（而非文件本身）
	// 因为编辑器保存文件时可能先删除再创建，直接监视文件会丢失事件
	dir := filepath.Dir(kc.path)
	if err := fsWatcher.Add(dir); err != nil {
		closeErr := fsWatcher.Close()
		return nil, errors.Join(
			fmt.Errorf("%w: failed to watch directory %s: %w", ErrWatchFailed, dir, err),
			closeErr,
		)
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Watcher{
		cfg:      kc,
		watcher:  fsWatcher,
		callback: callback,
		debounce: options.debounce,
		ctx:      ctx,
		cancel:   cancel,
	}, nil
}

// Start 启动监视
// 此方法会阻塞，通常应在 goroutine 中调用
func (w *Watcher) Start() {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return
	}
	w.running = true
	w.runWg.Add(1)
	w.mu.Unlock()

	defer w.runWg.Done()
	w.run()
}

// StartAsync 异步启动监视
// 在后台 goroutine 中运行，立即返回
// 解决与 Stop() 的竞态：先设置 running 标志再启动 goroutine
func (w *Watcher) StartAsync() {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return
	}
	w.running = true
	w.runWg.Add(1)
	w.mu.Unlock()

	go func() {
		defer w.runWg.Done()
		w.run()
	}()
}

// Stop 停止监视并释放 fsnotify 资源。
// Stop 返回后保证不再有回调执行。
//
// 设计决策: Stop() 无论是否调用过 Start()，都会释放 fsnotify.Watcher。
// Watch() 创建 fsnotify.Watcher 时已占用文件描述符，不释放会导致 fd 泄漏。
// stopped 标志确保 Stop() 幂等，多次调用不会重复关闭。
func (w *Watcher) Stop() error {
	w.mu.Lock()

	if w.stopped {
		w.mu.Unlock()
		return nil
	}
	w.stopped = true

	if !w.running {
		w.cancel()
		w.mu.Unlock()
		return w.watcher.Close()
	}

	// 停止 debounce 定时器，防止 Stop 后仍触发回调
	if w.timer != nil {
		w.timer.Stop()
		w.timer = nil
	}

	w.cancel()
	w.running = false
	w.mu.Unlock()

	// 等待 run() goroutine 退出和 in-flight 防抖回调完成
	w.runWg.Wait()
	w.callbackWg.Wait()

	return w.watcher.Close()
}

// run 运行监视循环
func (w *Watcher) run() {
	filename := filepath.Base(w.cfg.path)

	for {
		select {
		case <-w.ctx.Done():
			return

		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			w.handleEvent(event, filename)

		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			w.handleError(err)
		}
	}
}

// handleEvent 处理文件系统事件
func (w *Watcher) handleEvent(event fsnotify.Event, filename string) {
	// 只处理目标配置文件的事件
	if filepath.Base(event.Name) != filename {
		return
	}

	// 处理可能表示配置更新的事件
	// - Write: 直接修改
	// - Create: 新建文件（部分编辑器）
	// - Rename: 原子写入模式（vim/emacs 写临时文件后 rename）
	// - Remove: 文件被删除（Reload 会失败并通过 callback 通知）
	if !event.Has(fsnotify.Write) && !event.Has(fsnotify.Create) &&
		!event.Has(fsnotify.Rename) && !event.Has(fsnotify.Remove) {
		return
	}

	// 防抖处理：重置计时器
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.timer != nil {
		w.timer.Stop()
	}

	w.timer = time.AfterFunc(w.debounce, func() {
		// 检查 watcher 是否已停止（加锁确保与 Stop 互斥）
		w.mu.Lock()
		if !w.running {
			w.mu.Unlock()
			return
		}
		w.callbackWg.Add(1)
		w.mu.Unlock()
		defer w.callbackWg.Done()

		err := w.cfg.Reload()
		w.safeCallback(err)
	})
}

// handleError 处理 watcher 错误
func (w *Watcher) handleError(err error) {
	w.safeCallback(fmt.Errorf("xconf: watch error: %w", err))
}

// safeCallback 安全地调用用户回调，捕获 panic 防止进程崩溃。
func (w *Watcher) safeCallback(err error) {
	if w.callback == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			slog.Error("xconf: watch callback panicked",
				"panic", r,
				"stack", string(debug.Stack()),
			)
		}
	}()
	w.callback(w.cfg, err)
}

// WatchConfig 配置监视的便捷接口。
// 扩展 Config 接口，添加监视能力。
//
// 设计决策: WatchConfig 与包级函数 Watch() 提供两种等价的使用方式。
// Watch() 函数适合不关心接口层次的简单场景；
// WatchConfig 接口适合需要在类型系统中表达"可监视"能力的场景。
type WatchConfig interface {
	Config

	// Watch 监视配置文件变更
	// 当配置文件变更时自动重载并调用 callback
	Watch(callback WatchCallback, opts ...WatchOption) (*Watcher, error)
}

// koanfConfig 实现 WatchConfig 接口
func (c *koanfConfig) Watch(callback WatchCallback, opts ...WatchOption) (*Watcher, error) {
	return Watch(c, callback, opts...)
}
