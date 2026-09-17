package xconf

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// WatchCallback 文件变更回调函数。
// 当配置文件发生变更时调用，err 表示重载是否成功。
//
// 错误场景:
//   - 文件被删除或权限变更导致不可读: err 包含 ErrLoadFailed
//   - 文件内容格式错误: err 包含 ErrParseFailed（旧配置被保留）
//   - fsnotify 自身错误: err 包含 "xconf: watch error [path]" 前缀
//   - err == nil: 重载成功，cfg 已更新为最新配置
type WatchCallback func(cfg Config, err error)

// Watcher 配置文件监视器。
// 必须通过 Watch 函数创建，零值不可用。
// 监控配置文件变更并自动重载。
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

	// 设计决策: runWg 跟踪 run() goroutine 生命周期。
	// 所有用户回调在独立 goroutine 中执行（防抖回调由 time.AfterFunc 创建，
	// 错误/通道关闭回调由 dispatchCallback 创建），因此 runWg.Wait() 不会死锁。
	runWg sync.WaitGroup // run goroutine 生命周期

	// 设计决策: callbackStates 通过 goroutine ID 追踪所有 in-flight 回调。
	// key=回调 goroutine ID（通过 goid() 获取），value=完成信号 channel。
	// 多个回调可能并发执行（防抖回调超时 + 新事件 / 错误回调分发）。
	// Stop() 通过对比当前 goroutine ID 跳过自身的 done channel，等待其他 in-flight 回调。
	// 安全性保证：读写都在 mu 保护下。
	callbackStates map[int64]chan struct{} // in-flight 回调 done channel 映射

	// 设计决策: 并发 Stop 语义 — 所有调用方观察同一清理完成点。
	// 第一个调用方执行清理并 close(stopDone)；并发的非首个调用方等待 stopDone。
	// 例外：若并发调用方自身是 in-flight 回调（首个 Stop 在等它），直接返回避免死锁。
	stopDone chan struct{} // 清理完成信号，由首个 Stop 调用方 close
	stopErr  error         // 清理错误（仅首个调用方写入，其他读取）
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

// maxDebounce 防抖时间上界。超过此值通常是配置错误，会导致热重载实质失效。
const maxDebounce = time.Minute

// validate 校验监视器选项。
func (o *watchOptions) validate() error {
	if o.debounce <= 0 {
		return fmt.Errorf("%w: %v", ErrInvalidDebounce, o.debounce)
	}
	if o.debounce > maxDebounce {
		return fmt.Errorf("%w: %v exceeds maximum %v", ErrInvalidDebounce, o.debounce, maxDebounce)
	}
	return nil
}

// WithDebounce 设置防抖时间。
// 在指定时间内的多次变更只触发一次重载。
// 默认值为 100ms，适合大多数场景。推荐范围 50ms ~ 5s。
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
//   - 在回调中调用 Stop() 是安全的，不会死锁
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
//	w.Start() // 阻塞当前 goroutine；非阻塞场景请用 w.StartAsync()
//
// validateWatchConfig 校验 Watch 入参并返回具体的 *koanfConfig。
func validateWatchConfig(cfg Config, callback WatchCallback) (*koanfConfig, error) {
	kc, ok := cfg.(*koanfConfig)
	if !ok {
		return nil, fmt.Errorf("%w: unsupported config type %T", ErrWatchFailed, cfg)
	}
	// 防 typed nil：var kc *koanfConfig; var cfg Config = kc 可通过类型断言。
	if kc == nil {
		return nil, fmt.Errorf("%w: nil *koanfConfig", ErrWatchFailed)
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
	return kc, nil
}

// applyWatchOptions 应用 WatchOption 并校验。
func applyWatchOptions(opts []WatchOption) (*watchOptions, error) {
	options := defaultWatchOptions()
	for _, opt := range opts {
		if opt == nil {
			return nil, ErrNilWatchOption
		}
		opt(options)
	}
	if err := options.validate(); err != nil {
		return nil, err
	}
	return options, nil
}

func Watch(cfg Config, callback WatchCallback, opts ...WatchOption) (*Watcher, error) {
	kc, err := validateWatchConfig(cfg, callback)
	if err != nil {
		return nil, err
	}

	options, err := applyWatchOptions(opts)
	if err != nil {
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
		stopDone: make(chan struct{}),
	}, nil
}

// Start 启动监视
// 此方法会阻塞，通常应在 goroutine 中调用
func (w *Watcher) Start() {
	w.mu.Lock()
	if w.stopped || w.running {
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
//
// 设计决策: runWg.Add(1) 必须在 mu 锁内执行，避免 StartAsync 与 Stop 竞态。
// 若 Add 移到锁外，Stop 可能在 Add 前 runWg.Wait() 并 Close watcher，
// 使 run goroutine 随后在已关闭的 fsnotify 上运行，触发"channel closed"回调。
func (w *Watcher) StartAsync() {
	w.mu.Lock()
	if w.stopped || w.running {
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
// Stop 返回后保证不再有回调执行（在回调中调用 Stop 时，当前回调是最后一次执行）。
// 在回调中调用 Stop() 是安全的，不会死锁。
// 零值 Watcher（未经 Watch 创建）调用 Stop 会返回 ErrWatchFailed。
//
// 设计决策: Stop() 无论是否调用过 Start()，都会释放 fsnotify.Watcher。
// Watch() 创建 fsnotify.Watcher 时已占用文件描述符，不释放会导致 fd 泄漏。
// stopped 标志 + stopDone channel 确保 Stop() 幂等且并发调用方共享同一完成点：
//   - 首个调用方执行清理并 close(stopDone)
//   - 并发的非首个调用方等待 stopDone 后返回相同结果
//   - 例外：非首个调用方若自身是 in-flight 回调（首个 Stop 在等它），直接返回避免死锁
//
// 设计决策: 所有用户回调（防抖/错误/通道关闭）均在独立 goroutine 中执行，
// 通过 callbackStates 追踪 goroutine ID 检测回调内调用 Stop() 的场景。
// Stop() 对比当前 goroutine ID 跳过自身的 done channel，等待其他 in-flight 回调完成。
// run() goroutine 不直接执行用户回调，因此 runWg.Wait() 不会死锁。
func (w *Watcher) Stop() error {
	if w.cancel == nil {
		return fmt.Errorf("%w: Watcher not initialized (use Watch to create)", ErrWatchFailed)
	}
	w.mu.Lock()
	if w.stopped {
		// 非首个调用方：若自身是 in-flight 回调（首个 Stop 在等我完成），
		// 直接返回避免死锁；否则等首个调用方完成清理后返回相同结果。
		currentGID := goid()
		_, inCallback := w.callbackStates[currentGID]
		w.mu.Unlock()
		if inCallback {
			return nil
		}
		<-w.stopDone
		return w.stopErr
	}
	w.stopped = true
	w.mu.Unlock()

	w.stopErr = w.doStop()
	close(w.stopDone)
	return w.stopErr
}

// doStop 执行实际清理工作。仅由首个 Stop 调用方执行（stopped 标志保护）。
func (w *Watcher) doStop() error {
	w.mu.Lock()
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

	// 收集"非自身"的 in-flight 回调 done channel。
	// 即使自身是 in-flight 回调，也必须等待其他并发回调完成，保证 Stop 契约。
	currentGID := goid()
	var othersDone []chan struct{}
	for gid, done := range w.callbackStates {
		if gid != currentGID {
			othersDone = append(othersDone, done)
		}
	}

	w.cancel()
	w.running = false
	w.mu.Unlock()

	// 等待 run() goroutine 退出。
	// 所有用户回调在独立 goroutine 中执行，run() 不直接调用用户回调，
	// 因此 runWg.Wait() 不会死锁（即使回调内调用 Stop）。
	w.runWg.Wait()

	// 等待其他 in-flight 回调完成（自身除外，避免自锁）
	for _, d := range othersDone {
		<-d
	}

	return w.watcher.Close()
}

// run 运行监视循环。
// run 不直接调用用户回调——所有回调通过 dispatchCallback 或 time.AfterFunc
// 在独立 goroutine 中执行，避免回调内调用 Stop 导致 runWg.Wait 死锁。
func (w *Watcher) run() {
	filename := filepath.Base(w.cfg.path)

	for {
		select {
		case <-w.ctx.Done():
			return

		case event, ok := <-w.watcher.Events:
			if !ok {
				// 若 Stop 已启动（stopped=true 或 ctx 已取消），通道关闭是预期的正常关闭，
				// 不再触发"unexpected"回调，避免违反 Stop 契约。
				if w.isShuttingDown() {
					return
				}
				w.dispatchCallback(fmt.Errorf("xconf: watch event channel closed unexpectedly [%s]", w.cfg.path))
				return
			}
			w.handleEvent(event, filename)

		case err, ok := <-w.watcher.Errors:
			if !ok {
				if w.isShuttingDown() {
					return
				}
				w.dispatchCallback(fmt.Errorf("xconf: watch error channel closed unexpectedly [%s]", w.cfg.path))
				return
			}
			w.handleError(err)
		}
	}
}

// isShuttingDown 判断 Watcher 是否正在/已停止。用于 run() 通道关闭分支避免 Stop 后仍触发回调。
func (w *Watcher) isShuttingDown() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.stopped
}

// k8sConfigMapSymlink 是 K8s ConfigMap/Secret 挂载使用的 atomic symlink 名称。
// kubelet 更新时原子 rename 此 symlink 指向新 timestamp 目录；业务文件（如 config.yaml）
// 的 symlink 目标随之变化但自身未发生事件，因此需额外接收 ..data 的事件触发 Reload。
const k8sConfigMapSymlink = "..data"

// handleEvent 处理文件系统事件
func (w *Watcher) handleEvent(event fsnotify.Event, filename string) {
	// 只处理目标配置文件的事件，或 K8s ConfigMap 的 ..data symlink 事件
	// （K8s 原子更新只 rename ..data symlink，不直接触发 config.yaml 事件）
	base := filepath.Base(event.Name)
	if base != filename && base != k8sConfigMapSymlink {
		return
	}

	// 处理可能表示配置更新的事件
	// - Write: 直接修改
	// - Create: 新建文件（部分编辑器）
	// - Rename: 原子写入模式（vim/emacs 写临时文件后 rename；K8s 更新 ..data symlink）
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
		// 设计决策: callbackStates 注册必须在 mu 锁内执行。
		// 若移到锁外，Stop() 可能在注册前完成收集，导致 Stop 返回后仍有回调在执行。
		cbGID := goid()
		cbDone := make(chan struct{})
		if w.callbackStates == nil {
			w.callbackStates = make(map[int64]chan struct{})
		}
		w.callbackStates[cbGID] = cbDone
		w.mu.Unlock()

		defer func() {
			w.mu.Lock()
			delete(w.callbackStates, cbGID)
			w.mu.Unlock()
			close(cbDone)
		}()

		err := w.cfg.Reload()
		w.safeCallback(err)
	})
}

// handleError 处理 watcher 错误。
// 通过 dispatchCallback 在独立 goroutine 中执行回调，避免 run goroutine 内调用 Stop 死锁。
func (w *Watcher) handleError(err error) {
	w.dispatchCallback(fmt.Errorf("xconf: watch error [%s]: %w", w.cfg.path, err))
}

// dispatchCallback 在独立 goroutine 中执行用户回调并追踪其生命周期。
// 与防抖回调（handleEvent 中的 time.AfterFunc）使用相同的 callbackStates 追踪机制，
// 确保 Stop() 能等待所有 in-flight 回调完成。
func (w *Watcher) dispatchCallback(err error) {
	go func() {
		w.mu.Lock()
		if !w.running {
			w.mu.Unlock()
			return
		}
		cbGID := goid()
		cbDone := make(chan struct{})
		if w.callbackStates == nil {
			w.callbackStates = make(map[int64]chan struct{})
		}
		w.callbackStates[cbGID] = cbDone
		w.mu.Unlock()

		defer func() {
			w.mu.Lock()
			delete(w.callbackStates, cbGID)
			w.mu.Unlock()
			close(cbDone)
		}()

		w.safeCallback(err)
	}()
}

// safeCallback 安全地调用用户回调，捕获 panic 防止进程崩溃。
//
// 设计决策: panic 恢复使用全局 slog 输出日志。
// 作为工具库不应依赖上层日志组件（如 xlog），且回调 panic 属于编程错误，
// 使用标准库 slog 确保至少有一处日志输出，优于静默吞掉。
func (w *Watcher) safeCallback(err error) {
	if w.callback == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			// 防止外部 logger 实现 panic 导致回调 goroutine 崩溃。
			defer func() { recover() }()
			slog.Error("xconf: watch callback panicked",
				"panic", r,
				"stack", string(debug.Stack()),
			)
		}
	}()
	w.callback(w.cfg, err)
}

// goid 返回当前 goroutine 的 ID。
// 仅用于 callbackStates 中区分"自身"与"其他"in-flight 回调，非热路径。
//
// 设计决策: 依赖 runtime.Stack 输出格式（"goroutine NNN [running]:\n..."）。
// 该格式自 Go 1.0 至 1.25 保持稳定，但不属于 Go 兼容性承诺。
// 若未来 Go 版本变更格式导致解析失败，返回 0 并输出警告日志，
// 代价是多个并发回调时 Stop() 可能无法等待所有回调完成（概率极低）。
// 不会导致死锁：所有回调在独立 goroutine 中执行，run() 不直接调用用户回调。
func goid() int64 {
	var buf [32]byte
	n := runtime.Stack(buf[:], false)
	// format: "goroutine 123 [running]:\n..."
	s := string(buf[:n])
	const prefix = "goroutine "
	if len(s) <= len(prefix) || s[:len(prefix)] != prefix {
		slog.Warn("xconf: goid() runtime.Stack format changed, " +
			"Stop-from-callback deadlock detection disabled")
		return 0
	}
	s = s[len(prefix):]
	spaceIdx := 0
	for spaceIdx < len(s) && s[spaceIdx] != ' ' {
		spaceIdx++
	}
	id, err := strconv.ParseInt(s[:spaceIdx], 10, 64)
	if err != nil {
		slog.Warn("xconf: goid() failed to parse goroutine ID, " +
			"Stop-from-callback deadlock detection disabled")
		return 0
	}
	return id
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
