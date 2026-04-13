package xlimit

import (
	"context"
	"fmt"
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

		var change ConfigChange
		if watchErr != nil {
			change = ConfigChange{Err: watchErr}
		} else {
			newConfig, loadErr := p.Load()
			if loadErr != nil {
				change = ConfigChange{Err: loadErr}
			} else {
				change = ConfigChange{NewConfig: newConfig}
			}
		}

		// 设计决策: 非阻塞投递，丢弃旧事件保新事件。
		// 配置变更是覆盖语义，只需最新配置。阻塞投递可能
		// 卡住 xconf 回调线程，影响后续变更通知和停止流程。
		select {
		case <-ch:
		default:
		}
		select {
		case ch <- change:
		default:
		}
	})

	if err != nil {
		close(ch)
		return nil, err
	}

	// 启动监视器
	go func() {
		watcher.StartAsync()
		<-ctx.Done()
		if stopErr := watcher.Stop(); stopErr != nil {
			// 尽力发送停止错误，但不阻塞关闭流程
			select {
			case ch <- ConfigChange{Err: stopErr}:
			default:
			}
		}
		close(ch)
	}()

	return ch, nil
}

// WithConfigProvider 使用配置提供器加载配置
// 此选项会在创建限流器时立即加载配置。
// 如果加载失败，错误将在 New/NewLocal 构造时返回，
// 避免在无规则状态下静默放行所有请求。
func WithConfigProvider(provider ConfigProvider) Option {
	return func(o *options) {
		if provider == nil {
			return
		}

		config, err := provider.Load()
		if err != nil {
			// 设计决策: 将配置加载错误上抛到 New/NewLocal，而不是静默降级到
			// 默认配置。默认配置无规则会导致所有请求被放行，在生产环境中
			// 这比明确的创建失败更危险。
			o.initErr = fmt.Errorf("config provider load failed: %w", err)
			return
		}

		o.config = config
	}
}
