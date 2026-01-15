package xlimit

import (
	"context"
	"sync"

	"github.com/omeyang/xkit/pkg/config/xconf"
)

// ConfigProvider 配置提供器接口
// 用于从外部源加载限流配置
type ConfigProvider interface {
	// Load 加载配置
	Load() (Config, error)
	// Watch 监视配置变更，返回变更通道
	// 调用方需要在不再需要时取消 context 以停止监视
	Watch(ctx context.Context) (<-chan ConfigChange, error)
}

// ConfigChange 配置变更事件
type ConfigChange struct {
	// NewConfig 新配置
	NewConfig Config
	// Err 如果加载失败
	Err error
}

// XConfProvider xconf 配置提供器
// 从 xconf.Config 加载限流配置
type XConfProvider struct {
	cfg  xconf.Config
	path string
	mu   sync.RWMutex
}

// NewXConfProvider 创建 xconf 配置提供器
// cfg: xconf 配置实例
// path: 配置路径，如 "ratelimit"
func NewXConfProvider(cfg xconf.Config, path string) *XConfProvider {
	return &XConfProvider{
		cfg:  cfg,
		path: path,
	}
}

// Load 从 xconf 加载配置
func (p *XConfProvider) Load() (Config, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var config Config
	if err := p.cfg.Unmarshal(p.path, &config); err != nil {
		return Config{}, err
	}

	// 验证配置
	if err := config.Validate(); err != nil {
		return Config{}, err
	}

	return config, nil
}

// Watch 监视配置变更
// 当配置文件变更时，通过 channel 发送新配置
func (p *XConfProvider) Watch(ctx context.Context) (<-chan ConfigChange, error) {
	ch := make(chan ConfigChange, 1)

	watcher, err := xconf.Watch(p.cfg, func(_ xconf.Config, watchErr error) {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if watchErr != nil {
			ch <- ConfigChange{Err: watchErr}
			return
		}

		newConfig, loadErr := p.Load()
		if loadErr != nil {
			ch <- ConfigChange{Err: loadErr}
			return
		}

		ch <- ConfigChange{NewConfig: newConfig}
	})

	if err != nil {
		close(ch)
		return nil, err
	}

	// 启动监视器
	go func() {
		watcher.StartAsync()
		<-ctx.Done()
		watcher.Stop() //nolint:errcheck,gosec // 关闭时忽略错误
		close(ch)
	}()

	return ch, nil
}

// WithConfigProvider 使用配置提供器加载配置
// 此选项会在创建限流器时立即加载配置
func WithConfigProvider(provider ConfigProvider) Option {
	return func(o *options) {
		if provider == nil {
			return
		}

		config, err := provider.Load()
		if err != nil {
			// 配置加载失败，使用默认配置
			return
		}

		o.config = config
	}
}
