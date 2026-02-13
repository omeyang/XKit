package xdbg

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"strings"
	"sync"
	"time"
)

// 注册内置命令。
func (s *Server) registerBuiltinCommands() {
	s.registry.Register(newHelpCommand(s))
	s.registry.Register(newExitCommand(s))
	s.registry.Register(newSetlogCommand(s))
	s.registry.Register(newStackCommand())
	s.registry.Register(newFreememCommand())

	// 保存 pprof 命令引用，用于在 Stop 时清理资源
	s.pprofCmd = newPprofCommand(s)
	s.registry.Register(s.pprofCmd)
}

// helpCommand help 命令。
type helpCommand struct {
	server *Server
}

func newHelpCommand(s *Server) *helpCommand {
	return &helpCommand{server: s}
}

func (c *helpCommand) Name() string {
	return "help"
}

func (c *helpCommand) Help() string {
	return "显示帮助信息"
}

func (c *helpCommand) Execute(_ context.Context, args []string) (string, error) {
	var sb strings.Builder

	if len(args) > 0 {
		// 显示特定命令的帮助
		cmdName := args[0]
		cmd := c.server.registry.Get(cmdName)
		if cmd == nil {
			return "", fmt.Errorf("未知命令: %s", cmdName)
		}
		sb.WriteString(fmt.Sprintf("%s - %s\n", cmd.Name(), cmd.Help()))
		return sb.String(), nil
	}

	// 显示所有命令
	sb.WriteString("可用命令:\n")
	for _, cmd := range c.server.registry.Commands() {
		sb.WriteString(fmt.Sprintf("  %-12s %s\n", cmd.Name(), cmd.Help()))
	}
	sb.WriteString("\n使用 'help <command>' 查看命令详情")
	return sb.String(), nil
}

// exitCommand exit 命令。
type exitCommand struct {
	server *Server
}

func newExitCommand(s *Server) *exitCommand {
	return &exitCommand{server: s}
}

func (c *exitCommand) Name() string {
	return "exit"
}

func (c *exitCommand) Help() string {
	return "关闭调试服务（不影响主应用）"
}

func (c *exitCommand) Execute(_ context.Context, _ []string) (string, error) {
	// 设计决策: 100ms 延迟确保"调试服务即将关闭"响应有时间写回客户端后再关闭 listener。
	// 如果立即 Disable，底层 transport.Close() 会关闭 socket，导致响应发送失败。
	// goroutine 纳入 WaitGroup 管理，确保 Stop() 的 waitForGoroutines
	// 能等待此 goroutine 完成，避免 Disable 在 Stop 之后执行产生非预期状态。
	c.server.wg.Add(1)
	go func() {
		defer c.server.wg.Done()
		time.Sleep(100 * time.Millisecond)
		if err := c.server.Disable(); err != nil {
			c.server.audit(AuditEventCommandFailed, nil, "exit:disable", nil, 0, err)
		}
	}()
	return "调试服务即将关闭", nil
}

// setlogCommand setlog 命令。
type setlogCommand struct {
	server *Server
}

func newSetlogCommand(s *Server) *setlogCommand {
	return &setlogCommand{server: s}
}

func (c *setlogCommand) Name() string {
	return "setlog"
}

func (c *setlogCommand) Help() string {
	return "修改日志级别 (trace/debug/info/warn/error)"
}

func (c *setlogCommand) Execute(_ context.Context, args []string) (string, error) {
	if c.server.opts.Leveler == nil {
		return "", fmt.Errorf("日志级别控制器未配置")
	}

	if len(args) == 0 {
		// 显示当前级别
		level := c.server.opts.Leveler.Level()
		return fmt.Sprintf("当前日志级别: %s", level), nil
	}

	level := strings.ToLower(args[0])
	validLevels := []string{"trace", "debug", "info", "warn", "error"}

	valid := false
	for _, v := range validLevels {
		if level == v {
			valid = true
			break
		}
	}

	if !valid {
		return "", fmt.Errorf("无效的日志级别: %s，支持: %s", level, strings.Join(validLevels, "/"))
	}

	if err := c.server.opts.Leveler.SetLevel(level); err != nil {
		return "", fmt.Errorf("设置日志级别失败: %w", err)
	}

	return fmt.Sprintf("日志级别已修改为: %s", level), nil
}

// stackCommand stack 命令。
type stackCommand struct{}

func newStackCommand() *stackCommand {
	return &stackCommand{}
}

func (c *stackCommand) Name() string {
	return "stack"
}

func (c *stackCommand) Help() string {
	return "打印所有 goroutine 堆栈"
}

func (c *stackCommand) Execute(ctx context.Context, _ []string) (string, error) {
	// 检查上下文是否已取消
	if err := ctx.Err(); err != nil {
		return "", err
	}

	// 使用渐进式缓冲区扩展，避免一开始就分配 1MB
	// 从 64KB 开始，每次翻倍，最大 1MB
	const (
		initialSize = 64 * 1024   // 64KB
		maxSize     = 1024 * 1024 // 1MB
	)

	for size := initialSize; ; size *= 2 {
		if size > maxSize {
			size = maxSize
		}
		buf := make([]byte, size)
		n := runtime.Stack(buf, true) // true 表示获取所有 goroutine
		if n < size || size >= maxSize {
			return string(buf[:n]), nil
		}
	}
}

// freememCommand freemem 命令。
type freememCommand struct{}

func newFreememCommand() *freememCommand {
	return &freememCommand{}
}

func (c *freememCommand) Name() string {
	return "freemem"
}

func (c *freememCommand) Help() string {
	return "释放内存到操作系统"
}

func (c *freememCommand) Execute(ctx context.Context, _ []string) (string, error) {
	// 检查上下文是否已取消
	if err := ctx.Err(); err != nil {
		return "", err
	}
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)

	debug.FreeOSMemory()

	runtime.ReadMemStats(&after)

	return fmt.Sprintf(
		"内存释放完成\n释放前 HeapInuse: %d MB\n释放后 HeapInuse: %d MB",
		before.HeapInuse/1024/1024,
		after.HeapInuse/1024/1024,
	), nil
}

// pprofCommand pprof 命令。
type pprofCommand struct {
	server       *Server
	mu           sync.Mutex
	cpuFile      *os.File
	cpuFilePath  string
	cpuActive    bool
	profileFiles []string // 所有已创建的 profile 文件路径，Cleanup 时删除
}

func newPprofCommand(s *Server) *pprofCommand {
	return &pprofCommand{server: s}
}

func (c *pprofCommand) Name() string {
	return "pprof"
}

func (c *pprofCommand) Help() string {
	return "性能分析 (cpu start/stop, heap, goroutine)"
}

func (c *pprofCommand) Execute(ctx context.Context, args []string) (string, error) {
	// 检查上下文是否已取消
	if err := ctx.Err(); err != nil {
		return "", err
	}

	if len(args) == 0 {
		return c.showUsage(), nil
	}

	subCmd := strings.ToLower(args[0])
	switch subCmd {
	case "cpu":
		if len(args) < 2 {
			return c.showUsage(), nil
		}
		action := strings.ToLower(args[1])
		switch action {
		case "start":
			return c.cpuStart()
		case "stop":
			return c.cpuStop()
		default:
			return "", fmt.Errorf("未知 CPU profile 操作: %s", action)
		}

	case "heap":
		return c.heapProfile(ctx)

	case "goroutine":
		return c.goroutineProfile(ctx)

	default:
		return "", fmt.Errorf("未知子命令: %s", subCmd)
	}
}

func (c *pprofCommand) showUsage() string {
	return `pprof 使用方法:
  pprof cpu start   - 开始 CPU profile
  pprof cpu stop    - 停止 CPU profile 并保存
  pprof heap        - 导出堆内存 profile
  pprof goroutine   - 导出 goroutine profile`
}

func (c *pprofCommand) cpuStart() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cpuActive {
		return "", fmt.Errorf("CPU profile 已在运行中")
	}

	// 使用 os.CreateTemp 创建随机文件名，防止 symlink 攻击
	f, err := os.CreateTemp("", "xdbg_cpu_*.pprof")
	if err != nil {
		return "", fmt.Errorf("创建 CPU profile 文件失败: %w", err)
	}
	c.cpuFilePath = f.Name()
	c.cpuFile = f

	// 开始 CPU profile
	if err := pprof.StartCPUProfile(f); err != nil {
		if closeErr := f.Close(); closeErr != nil {
			c.server.audit(AuditEventCommandFailed, nil, "pprof:cpu:cleanup:close", nil, 0, closeErr)
		}
		if removeErr := os.Remove(c.cpuFilePath); removeErr != nil {
			c.server.audit(AuditEventCommandFailed, nil, "pprof:cpu:cleanup:remove", nil, 0, removeErr)
		}
		c.cpuFile = nil
		c.cpuFilePath = ""
		return "", fmt.Errorf("启动 CPU profile 失败: %w", err)
	}

	c.cpuActive = true
	return fmt.Sprintf("CPU profile 已开始，将保存到: %s\n使用 'pprof cpu stop' 停止", c.cpuFilePath), nil
}

func (c *pprofCommand) cpuStop() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.cpuActive {
		return "", fmt.Errorf("CPU profile 未在运行")
	}

	// 停止 CPU profile
	pprof.StopCPUProfile()
	c.cpuActive = false

	// 关闭文件
	if c.cpuFile != nil {
		if err := c.cpuFile.Close(); err != nil {
			return "", fmt.Errorf("关闭 CPU profile 文件失败: %w", err)
		}
		c.cpuFile = nil
	}

	return fmt.Sprintf("CPU profile 已停止，保存到: %s\n使用 'go tool pprof %s' 分析", c.cpuFilePath, c.cpuFilePath), nil
}

func (c *pprofCommand) heapProfile(ctx context.Context) (string, error) {
	// 检查上下文是否已取消
	if err := ctx.Err(); err != nil {
		return "", err
	}
	// 使用 os.CreateTemp 创建随机文件名，防止 symlink 攻击
	f, err := os.CreateTemp("", "xdbg_heap_*.pprof")
	if err != nil {
		return "", fmt.Errorf("创建 heap profile 文件失败: %w", err)
	}
	filename := f.Name()

	// 写入 heap profile
	if err := pprof.WriteHeapProfile(f); err != nil {
		if closeErr := f.Close(); closeErr != nil {
			c.server.audit(AuditEventCommandFailed, nil, "pprof:heap:cleanup:close", nil, 0, closeErr)
		}
		if removeErr := os.Remove(filename); removeErr != nil {
			c.server.audit(AuditEventCommandFailed, nil, "pprof:heap:cleanup:remove", nil, 0, removeErr)
		}
		return "", fmt.Errorf("写入 heap profile 失败: %w", err)
	}

	// 关闭文件（数据已写入，关闭错误仅记录日志）
	if closeErr := f.Close(); closeErr != nil {
		c.server.audit(AuditEventCommandFailed, nil, "pprof:heap:close", nil, 0, closeErr)
	}

	// 记录文件路径，Cleanup 时统一删除
	c.trackProfileFile(filename)

	var sb strings.Builder
	fmt.Fprintf(&sb, "Heap profile 已导出到: %s\n\n", filename)

	// 获取内存统计
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	sb.WriteString("内存统计:\n")
	fmt.Fprintf(&sb, "  Alloc:      %d MB\n", m.Alloc/1024/1024)
	fmt.Fprintf(&sb, "  TotalAlloc: %d MB\n", m.TotalAlloc/1024/1024)
	fmt.Fprintf(&sb, "  Sys:        %d MB\n", m.Sys/1024/1024)
	fmt.Fprintf(&sb, "  NumGC:      %d\n", m.NumGC)
	fmt.Fprintf(&sb, "  HeapInuse:  %d MB\n", m.HeapInuse/1024/1024)
	fmt.Fprintf(&sb, "  HeapIdle:   %d MB\n", m.HeapIdle/1024/1024)
	fmt.Fprintf(&sb, "\n使用 'go tool pprof %s' 分析", filename)

	return sb.String(), nil
}

func (c *pprofCommand) goroutineProfile(ctx context.Context) (string, error) {
	// 检查上下文是否已取消
	if err := ctx.Err(); err != nil {
		return "", err
	}
	// 使用 os.CreateTemp 创建随机文件名，防止 symlink 攻击
	f, err := os.CreateTemp("", "xdbg_goroutine_*.pprof")
	if err != nil {
		return "", fmt.Errorf("创建 goroutine profile 文件失败: %w", err)
	}
	filename := f.Name()

	// 获取 goroutine profile 并写入
	p := pprof.Lookup("goroutine")
	if p == nil {
		if closeErr := f.Close(); closeErr != nil {
			c.server.audit(AuditEventCommandFailed, nil, "pprof:goroutine:cleanup:close", nil, 0, closeErr)
		}
		if removeErr := os.Remove(filename); removeErr != nil {
			c.server.audit(AuditEventCommandFailed, nil, "pprof:goroutine:cleanup:remove", nil, 0, removeErr)
		}
		return "", fmt.Errorf("获取 goroutine profile 失败")
	}
	if err := p.WriteTo(f, 0); err != nil {
		if closeErr := f.Close(); closeErr != nil {
			c.server.audit(AuditEventCommandFailed, nil, "pprof:goroutine:cleanup:close", nil, 0, closeErr)
		}
		if removeErr := os.Remove(filename); removeErr != nil {
			c.server.audit(AuditEventCommandFailed, nil, "pprof:goroutine:cleanup:remove", nil, 0, removeErr)
		}
		return "", fmt.Errorf("写入 goroutine profile 失败: %w", err)
	}

	// 关闭文件（数据已写入，关闭错误仅记录日志）
	if closeErr := f.Close(); closeErr != nil {
		c.server.audit(AuditEventCommandFailed, nil, "pprof:goroutine:close", nil, 0, closeErr)
	}

	// 记录文件路径，Cleanup 时统一删除
	c.trackProfileFile(filename)

	var sb strings.Builder
	fmt.Fprintf(&sb, "Goroutine profile 已导出到: %s\n\n", filename)
	fmt.Fprintf(&sb, "Goroutine 数量: %d\n", runtime.NumGoroutine())
	fmt.Fprintf(&sb, "Goroutine profile count: %d\n", p.Count())
	fmt.Fprintf(&sb, "\n使用 'go tool pprof %s' 分析", filename)

	return sb.String(), nil
}

// trackProfileFile 记录已创建的 profile 文件路径，Cleanup 时统一删除。
func (c *pprofCommand) trackProfileFile(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.profileFiles = append(c.profileFiles, path)
}

// Cleanup 清理资源，在 Server 关闭时调用。
// 如果 CPU profile 正在运行，会自动停止并保存。
// 同时删除所有已创建的临时 profile 文件，防止磁盘泄漏。
func (c *pprofCommand) Cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cpuActive {
		// 停止 CPU profile
		pprof.StopCPUProfile()
		c.cpuActive = false

		// 关闭文件
		if c.cpuFile != nil {
			if err := c.cpuFile.Close(); err != nil {
				// 在 Server 关闭期间，审计日志可能已关闭，输出到 stderr
				fmt.Fprintf(os.Stderr, "[XDBG] failed to close CPU profile file: %v\n", err)
			}
			c.cpuFile = nil
		}
	}

	// 删除所有已创建的临时 profile 文件
	for _, path := range c.profileFiles {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "[XDBG] failed to remove profile file %s: %v\n", path, err)
		}
	}
	c.profileFiles = nil
}
