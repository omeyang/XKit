package xconf

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
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
	timer    *time.Timer // debounce 定时器，Stop() 时需要取消
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
//   - cfg: 要监视的配置实例（必须是从文件创建的）
//   - callback: 变更回调函数
//   - opts: 可选配置
//
// 注意:
//   - 只能监视从文件创建的 Config（通过 New() 创建）
//   - 从 bytes 创建的 Config 不支持监视
//   - 返回的 Watcher 需要调用 Start() 开始监视，Stop() 停止监视
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
		return nil, fmt.Errorf("xconf: unsupported config type")
	}

	if kc.isBytes {
		return nil, fmt.Errorf("xconf: cannot watch config created from bytes")
	}

	if kc.path == "" {
		return nil, fmt.Errorf("xconf: config path is empty")
	}

	// 应用选项
	options := defaultWatchOptions()
	for _, opt := range opts {
		opt(options)
	}

	// 创建 fsnotify watcher
	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("xconf: failed to create watcher: %w", err)
	}

	// 监视配置文件所在目录（而非文件本身）
	// 因为编辑器保存文件时可能先删除再创建，直接监视文件会丢失事件
	dir := filepath.Dir(kc.path)
	if err := fsWatcher.Add(dir); err != nil {
		closeErr := fsWatcher.Close()
		return nil, errors.Join(
			fmt.Errorf("xconf: failed to watch directory %s: %w", dir, err),
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
	w.mu.Unlock()

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
	w.mu.Unlock()

	go w.run()
}

// Stop 停止监视
func (w *Watcher) Stop() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.running {
		return nil
	}

	// 停止 debounce 定时器，防止 Stop 后仍触发回调
	if w.timer != nil {
		w.timer.Stop()
		w.timer = nil
	}

	w.cancel()
	w.running = false
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
	if !event.Has(fsnotify.Write) && !event.Has(fsnotify.Create) && !event.Has(fsnotify.Rename) {
		return
	}

	// 防抖处理：重置计时器
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.timer != nil {
		w.timer.Stop()
	}

	w.timer = time.AfterFunc(w.debounce, func() {
		// 检查 watcher 是否已停止
		select {
		case <-w.ctx.Done():
			return
		default:
		}

		err := w.cfg.Reload()
		if w.callback != nil {
			w.callback(w.cfg, err)
		}
	})
}

// handleError 处理 watcher 错误
func (w *Watcher) handleError(err error) {
	if w.callback != nil {
		w.callback(w.cfg, fmt.Errorf("xconf: watch error: %w", err))
	}
}

// WatchConfig 配置监视的便捷接口
// 扩展 Config 接口，添加监视能力
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
