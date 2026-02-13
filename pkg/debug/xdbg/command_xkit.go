package xdbg

import (
	"context"
	"fmt"
	"strings"

	"github.com/omeyang/xkit/pkg/util/xjson"
)

// 注册 xkit 集成命令。
func (s *Server) registerXkitCommands() {
	// 熔断器命令
	if s.opts.BreakerRegistry != nil {
		s.registry.Register(newBreakerCommand(s))
	}

	// 限流器命令
	if s.opts.LimiterRegistry != nil {
		s.registry.Register(newLimitCommand(s))
	}

	// 缓存命令
	if s.opts.CacheRegistry != nil {
		s.registry.Register(newCacheCommand(s))
	}

	// 配置命令
	if s.opts.ConfigProvider != nil {
		s.registry.Register(newConfigCommand(s))
	}
}

// breakerCommand breaker 命令。
type breakerCommand struct {
	server *Server
}

func newBreakerCommand(s *Server) *breakerCommand {
	return &breakerCommand{server: s}
}

func (c *breakerCommand) Name() string {
	return "breaker"
}

func (c *breakerCommand) Help() string {
	return "查看/重置熔断器状态 (breaker [name] [reset])"
}

func (c *breakerCommand) Execute(_ context.Context, args []string) (string, error) {
	registry := c.server.opts.BreakerRegistry
	if registry == nil {
		return "", fmt.Errorf("熔断器注册表未配置")
	}

	// 列出所有熔断器
	if len(args) == 0 {
		return c.listBreakers(registry)
	}

	name := args[0]

	// 检查是否是重置操作
	if len(args) > 1 && args[1] == "reset" {
		return c.resetBreaker(registry, name)
	}

	// 查看特定熔断器
	return c.showBreaker(registry, name)
}

func (c *breakerCommand) listBreakers(registry BreakerRegistry) (string, error) {
	names := registry.List()
	if len(names) == 0 {
		return "没有注册的熔断器", nil
	}

	var sb strings.Builder
	sb.WriteString("熔断器列表:\n")

	for _, name := range names {
		info, ok := registry.Get(name)
		if !ok {
			continue
		}
		fmt.Fprintf(&sb, "  %-20s [%s] requests=%d successes=%d failures=%d\n",
			info.Name, info.State, info.Requests, info.TotalSuccesses, info.TotalFailures)
	}

	return sb.String(), nil
}

func (c *breakerCommand) showBreaker(registry BreakerRegistry, name string) (string, error) {
	info, ok := registry.Get(name)
	if !ok {
		return "", fmt.Errorf("熔断器不存在: %s", name)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "熔断器: %s\n", info.Name)
	fmt.Fprintf(&sb, "  状态:     %s\n", info.State)
	fmt.Fprintf(&sb, "  总请求:   %d\n", info.Requests)
	fmt.Fprintf(&sb, "  总成功:   %d\n", info.TotalSuccesses)
	fmt.Fprintf(&sb, "  总失败:   %d\n", info.TotalFailures)
	fmt.Fprintf(&sb, "  连续成功: %d\n", info.ConsecutiveSuccesses)
	fmt.Fprintf(&sb, "  连续失败: %d\n", info.ConsecutiveFailures)

	return sb.String(), nil
}

func (c *breakerCommand) resetBreaker(registry BreakerRegistry, name string) (string, error) {
	if err := registry.Reset(name); err != nil {
		return "", fmt.Errorf("重置熔断器失败: %w", err)
	}
	return fmt.Sprintf("熔断器 %s 已重置", name), nil
}

// limitCommand limit 命令。
type limitCommand struct {
	server *Server
}

func newLimitCommand(s *Server) *limitCommand {
	return &limitCommand{server: s}
}

func (c *limitCommand) Name() string {
	return "limit"
}

func (c *limitCommand) Help() string {
	return "查看限流器状态 (limit [name])"
}

func (c *limitCommand) Execute(_ context.Context, args []string) (string, error) {
	registry := c.server.opts.LimiterRegistry
	if registry == nil {
		return "", fmt.Errorf("限流器注册表未配置")
	}

	// 列出所有限流器
	if len(args) == 0 {
		return c.listLimiters(registry)
	}

	// 查看特定限流器
	return c.showLimiter(registry, args[0])
}

func (c *limitCommand) listLimiters(registry LimiterRegistry) (string, error) {
	names := registry.List()
	if len(names) == 0 {
		return "没有注册的限流器", nil
	}

	var sb strings.Builder
	sb.WriteString("限流器列表:\n")

	for _, name := range names {
		info, ok := registry.Get(name)
		if !ok {
			continue
		}
		fmt.Fprintf(&sb, "  %-20s [%s] limit=%d remaining=%d\n",
			info.Name, info.Type, info.Limit, info.Remaining)
	}

	return sb.String(), nil
}

func (c *limitCommand) showLimiter(registry LimiterRegistry, name string) (string, error) {
	info, ok := registry.Get(name)
	if !ok {
		return "", fmt.Errorf("限流器不存在: %s", name)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "限流器: %s\n", info.Name)
	fmt.Fprintf(&sb, "  类型:   %s\n", info.Type)
	fmt.Fprintf(&sb, "  配额:   %d\n", info.Limit)
	fmt.Fprintf(&sb, "  剩余:   %d\n", info.Remaining)
	fmt.Fprintf(&sb, "  重置于: %d\n", info.Reset)

	return sb.String(), nil
}

// cacheCommand cache 命令。
type cacheCommand struct {
	server *Server
}

func newCacheCommand(s *Server) *cacheCommand {
	return &cacheCommand{server: s}
}

func (c *cacheCommand) Name() string {
	return "cache"
}

func (c *cacheCommand) Help() string {
	return "查看缓存统计 (cache [name])"
}

func (c *cacheCommand) Execute(_ context.Context, args []string) (string, error) {
	registry := c.server.opts.CacheRegistry
	if registry == nil {
		return "", fmt.Errorf("缓存注册表未配置")
	}

	// 列出所有缓存
	if len(args) == 0 {
		return c.listCaches(registry)
	}

	// 查看特定缓存
	return c.showCache(registry, args[0])
}

func (c *cacheCommand) listCaches(registry CacheRegistry) (string, error) {
	names := registry.List()
	if len(names) == 0 {
		return "没有注册的缓存", nil
	}

	var sb strings.Builder
	sb.WriteString("缓存列表:\n")

	for _, name := range names {
		stats, ok := registry.Get(name)
		if !ok {
			continue
		}
		hitRate := float64(0)
		total := stats.Hits + stats.Misses
		if total > 0 {
			hitRate = float64(stats.Hits) / float64(total) * 100
		}
		fmt.Fprintf(&sb, "  %-20s [%s] hits=%d misses=%d hitRate=%.1f%%\n",
			stats.Name, stats.Type, stats.Hits, stats.Misses, hitRate)
	}

	return sb.String(), nil
}

func (c *cacheCommand) showCache(registry CacheRegistry, name string) (string, error) {
	stats, ok := registry.Get(name)
	if !ok {
		return "", fmt.Errorf("缓存不存在: %s", name)
	}

	hitRate := float64(0)
	total := stats.Hits + stats.Misses
	if total > 0 {
		hitRate = float64(stats.Hits) / float64(total) * 100
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "缓存: %s\n", stats.Name)
	fmt.Fprintf(&sb, "  类型:     %s\n", stats.Type)
	fmt.Fprintf(&sb, "  命中:     %d\n", stats.Hits)
	fmt.Fprintf(&sb, "  未命中:   %d\n", stats.Misses)
	fmt.Fprintf(&sb, "  命中率:   %.1f%%\n", hitRate)
	fmt.Fprintf(&sb, "  当前大小: %d\n", stats.Size)
	fmt.Fprintf(&sb, "  最大大小: %d\n", stats.MaxSize)

	return sb.String(), nil
}

// configCommand config 命令。
type configCommand struct {
	server *Server
}

func newConfigCommand(s *Server) *configCommand {
	return &configCommand{server: s}
}

func (c *configCommand) Name() string {
	return "config"
}

func (c *configCommand) Help() string {
	return "查看运行时配置"
}

// 设计决策: 框架层不添加自动脱敏兜底。框架无法预知哪些配置键是敏感的（密码/Token/DSN
// 的命名因业务而异），强行匹配关键字会产生大量误报或漏报。脱敏责任由 ConfigProvider.Dump()
// 实现方承担（见 interfaces.go 安全警告），这与 Go 标准库 database/sql.DB.Stats() 等
// "返回者负责脱敏"的惯例一致。如需禁用 config 命令，可使用 WithCommandWhitelist 排除。
func (c *configCommand) Execute(_ context.Context, _ []string) (string, error) {
	provider := c.server.opts.ConfigProvider
	if provider == nil {
		return "", fmt.Errorf("配置提供者未配置")
	}

	config := provider.Dump()
	if config == nil {
		return "配置为空", nil
	}

	s, err := xjson.PrettyE(config)
	if err != nil {
		return "", fmt.Errorf("序列化配置失败: %w", err)
	}

	return s, nil
}
