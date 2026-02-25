//go:build !windows

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/urfave/cli/v3"
)

// errServerRejected 表示服务端拒绝执行命令。
// 用于 errors.Is 判断，使脚本可区分服务端拒绝与网络/协议错误。
var errServerRejected = fmt.Errorf("服务端拒绝执行")

// exitError 表示需要非零退出码但已完成输出的场景。
// 命令内部已完成所有输出，main 只需设置退出码。
type exitError struct {
	code int
}

func (e *exitError) Error() string { return fmt.Sprintf("exit status %d", e.code) }

// usageError 表示用户输入/参数错误（退出码 2）。
// 设计决策: 与运行时错误（退出码 1）区分，使自动化脚本可判断是否为用户输入问题。
type usageError struct {
	msg string
}

func (e *usageError) Error() string { return e.msg }

// isCLIUsageError 检测是否为 CLI 框架产生的参数错误（如未知 flag、未知命令）。
// 设计决策: urfave/cli/v3 对参数错误返回的是 plain error 或 ExitCoder（无统一自定义类型），
// 只能通过错误消息前缀识别。这些前缀是 urfave/cli 内部常量，变化概率极低。
func isCLIUsageError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "flag provided but not defined") ||
		strings.Contains(msg, "flag needs an argument") ||
		strings.Contains(msg, "invalid value") ||
		strings.Contains(msg, "No help topic for")
}

// maxCommLen 是 Linux TASK_COMM_LEN (16) 减去 null 终止符后的最大进程名长度。
const maxCommLen = 15

// validateTimeout 校验超时参数。
func validateTimeout(timeout time.Duration) error {
	if timeout <= 0 {
		return &usageError{msg: fmt.Sprintf("超时时间必须为正数，当前值: %v", timeout)}
	}
	return nil
}

// validateProcessName 校验进程名参数。
func validateProcessName(name string) error {
	if name == "" {
		return &usageError{msg: "进程名不能为空"}
	}
	if len(name) > maxCommLen {
		return &usageError{msg: fmt.Sprintf(
			"进程名 %q 超过 Linux 最大长度 %d 字符", name, maxCommLen)}
	}
	for _, r := range name {
		if r < 32 || r == 127 {
			return &usageError{msg: fmt.Sprintf("进程名包含非法控制字符: %q", name)}
		}
		if r == '/' || r == '\\' {
			return &usageError{msg: fmt.Sprintf("进程名包含非法路径分隔符: %q", name)}
		}
	}
	return nil
}

// 创建所有子命令。
func createCommands() []*cli.Command {
	return []*cli.Command{
		createToggleCommand(),
		createDisableCommand(),
		createExecCommand(),
		createStatusCommand(),
		createInteractiveCommand(),
		// 快捷命令（等价于 exec <command>）
		createShortcutCommand("setlog", "查看/设置日志级别", "[level]"),
		createShortcutCommand("stack", "打印所有 goroutine 堆栈", ""),
		createShortcutCommand("freemem", "释放内存到操作系统", ""),
		createShortcutCommand("pprof", "性能分析", "<subcommand>"),
		createShortcutCommand("breaker", "查看熔断器状态", "[name]"),
		createShortcutCommand("limit", "查看限流器状态", "[name]"),
		createShortcutCommand("cache", "查看缓存统计", "[name]"),
		createShortcutCommand("config", "查看运行时配置", ""),
	}
}

// createShortcutCommand 创建快捷命令（等价于 exec <command>）。
func createShortcutCommand(name, usage, argsUsage string) *cli.Command {
	cmd := &cli.Command{
		Name:  name,
		Usage: usage,
		Action: func(ctx context.Context, cmd *cli.Command) error {
			socketPath := cmd.String("socket")
			timeout := cmd.Duration("timeout")
			// 构建等价于 exec <name> [args...] 的参数
			args := append([]string{name}, cmd.Args().Slice()...)
			return cmdExec(ctx, socketPath, timeout, args)
		},
	}
	if argsUsage != "" {
		cmd.ArgsUsage = argsUsage
	}
	return cmd
}

// createToggleCommand 创建 toggle 子命令（切换调试服务状态）。
func createToggleCommand() *cli.Command {
	return &cli.Command{
		Name:    "toggle",
		Aliases: []string{"t"},
		Usage:   "切换调试服务状态（通过发送 SIGUSR1 信号）",
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:    "pid",
				Aliases: []string{"p"},
				Usage:   "目标进程 PID（优先级最高）",
			},
			&cli.StringFlag{
				Name:    "name",
				Aliases: []string{"n"},
				Usage:   "目标进程名称（从 /proc/*/comm 查找）",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			socketPath := cmd.String("socket")
			pidFlag := cmd.Int("pid")
			nameFlag := cmd.String("name")
			// 用户显式传入 --pid 0 时报错（未传 --pid 时默认值也是 0，需区分）
			if cmd.IsSet("pid") && pidFlag <= 0 {
				return &usageError{msg: fmt.Sprintf("无效的 PID: %d（PID 必须为正整数）", pidFlag)}
			}
			return cmdToggle(ctx, socketPath, pidFlag, nameFlag)
		},
	}
}

// createDisableCommand 创建 disable 子命令。
func createDisableCommand() *cli.Command {
	return &cli.Command{
		Name:    "disable",
		Aliases: []string{"d"},
		Usage:   "禁用调试服务",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			socketPath := cmd.String("socket")
			timeout := cmd.Duration("timeout")
			return cmdDisable(ctx, socketPath, timeout)
		},
	}
}

// createExecCommand 创建 exec 子命令。
func createExecCommand() *cli.Command {
	return &cli.Command{
		Name:      "exec",
		Aliases:   []string{"x"},
		Usage:     "执行调试命令",
		ArgsUsage: "<command> [args...]",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			socketPath := cmd.String("socket")
			timeout := cmd.Duration("timeout")
			args := cmd.Args().Slice()
			return cmdExec(ctx, socketPath, timeout, args)
		},
	}
}

// createStatusCommand 创建 status 子命令。
func createStatusCommand() *cli.Command {
	return &cli.Command{
		Name:    "status",
		Aliases: []string{"s"},
		Usage:   "查看服务状态",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			socketPath := cmd.String("socket")
			timeout := cmd.Duration("timeout")
			return cmdStatus(ctx, socketPath, timeout)
		},
	}
}

// createInteractiveCommand 创建 interactive 子命令。
func createInteractiveCommand() *cli.Command {
	return &cli.Command{
		Name:    "interactive",
		Aliases: []string{"i", "repl"},
		Usage:   "交互模式（REPL）",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			socketPath := cmd.String("socket")
			timeout := cmd.Duration("timeout")
			return cmdInteractive(ctx, socketPath, timeout)
		},
	}
}

// validateToggleArgs 校验 toggle 命令参数。
func validateToggleArgs(pidFlag int, nameFlag string) error {
	if pidFlag < 0 {
		return &usageError{msg: fmt.Sprintf("无效的 PID: %d（PID 必须为正整数）", pidFlag)}
	}
	if nameFlag != "" {
		return validateProcessName(nameFlag)
	}
	return nil
}

// postSignalCheckDelay 发送信号后等待一段时间再检查进程状态。
// 设计决策: 50ms 足够内核完成同步信号投递与默认处理（终止），
// 同时不会对 CLI 交互产生可感知的延迟。
var postSignalCheckDelay = 50 * time.Millisecond

// discoverTargetPID 按优先级发现目标进程 PID。
// 优先级：pidFlag > nameFlag > socket 文件发现。
func discoverTargetPID(ctx context.Context, socketPath string, pidFlag int, nameFlag string) (int, error) {
	switch {
	case pidFlag > 0:
		return pidFlag, nil
	case nameFlag != "":
		pid, err := findProcessByName(ctx, nameFlag)
		if err != nil {
			return 0, fmt.Errorf("通过进程名查找失败: %w", err)
		}
		return pid, nil
	default:
		pid, err := findProcessBySocket(ctx, socketPath)
		if err != nil {
			hint := "请使用 --pid 或 --name 参数指定目标进程"
			if isContainerEnvironment() {
				hint = "在容器环境中，请使用 --pid 1（如主进程是目标）、--name <进程名> 或指定具体 PID"
			}
			return 0, fmt.Errorf("无法自动发现进程: %w\n%s", err, hint)
		}
		return pid, nil
	}
}

// cmdToggle 切换调试服务状态（发送 SIGUSR1 信号触发 toggle）。
// 进程发现优先级：--pid > --name > socket 发现
func cmdToggle(ctx context.Context, socketPath string, pidFlag int, nameFlag string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if err := validateToggleArgs(pidFlag, nameFlag); err != nil {
		return err
	}

	pid, err := discoverTargetPID(ctx, socketPath, pidFlag, nameFlag)
	if err != nil {
		return err
	}

	// 验证进程存在
	if err := syscall.Kill(pid, 0); err != nil {
		return fmt.Errorf("进程 %d 不存在或无权限访问: %w", pid, err)
	}

	// 发送 SIGUSR1 信号
	if err := syscall.Kill(pid, syscall.SIGUSR1); err != nil {
		return fmt.Errorf("发送信号失败: %w", err)
	}

	// 安全检查: 等待短暂时间后验证目标进程仍然存活。
	// 设计决策: 若目标进程未注册 SIGUSR1 处理器，SIGUSR1 的默认行为（Linux）
	// 是终止进程。此检查可检测误杀，避免静默返回成功。
	time.Sleep(postSignalCheckDelay)
	if err := syscall.Kill(pid, 0); err != nil {
		return fmt.Errorf("警告: 进程 %d 在收到 SIGUSR1 后已退出（可能不是 xdbg 服务进程）", pid)
	}

	fmt.Printf("已向进程 %d 发送 SIGUSR1 信号（切换调试服务状态）\n", pid)
	return nil
}

// cmdDisable 禁用调试服务。
func cmdDisable(ctx context.Context, socketPath string, timeout time.Duration) error {
	if err := validateTimeout(timeout); err != nil {
		return err
	}
	client := NewClient(socketPath, timeout)

	resp, err := client.Execute(ctx, "exit", nil)
	if err != nil {
		return err
	}

	if !resp.Success {
		return fmt.Errorf("%w: %s", errServerRejected, resp.Error)
	}

	fmt.Println(resp.Output)
	return nil
}

// cmdExec 执行调试命令。
func cmdExec(ctx context.Context, socketPath string, timeout time.Duration, args []string) error {
	if err := validateTimeout(timeout); err != nil {
		return err
	}
	if len(args) == 0 {
		return &usageError{msg: "exec 命令需要指定要执行的调试命令"}
	}

	command := args[0]
	cmdArgs := args[1:]

	client := NewClient(socketPath, timeout)

	resp, err := client.Execute(ctx, command, cmdArgs)
	if err != nil {
		return err
	}

	if !resp.Success {
		return fmt.Errorf("%w: %s", errServerRejected, resp.Error)
	}

	if resp.Output != "" {
		fmt.Println(resp.Output)
	}

	if resp.Truncated {
		fmt.Fprintf(os.Stderr, "\n[警告: 输出已截断，原始大小: %d 字节]\n", resp.OriginalSize)
	}

	return nil
}

// cmdStatus 查看服务状态。
// 设计决策: 离线时返回非零退出码（通过 exitError），
// 使脚本和探针能正确检测服务状态。
func cmdStatus(ctx context.Context, socketPath string, timeout time.Duration) error {
	if err := validateTimeout(timeout); err != nil {
		return err
	}
	client := NewClient(socketPath, timeout)

	err := client.Ping(ctx)
	if err != nil {
		fmt.Printf("状态: 离线\n")
		fmt.Printf("Socket: %s\n", socketPath)
		fmt.Printf("详情: %v\n", err)
		return &exitError{code: 1}
	}

	fmt.Printf("状态: 在线\n")
	fmt.Printf("Socket: %s\n", socketPath)
	return nil
}

// findProcessByName 通过进程名查找进程 PID。
// 扫描 /proc/*/comm 文件匹配进程名。
// 当匹配多个进程时返回错误，要求使用 --pid 明确指定。
func findProcessByName(ctx context.Context, name string) (int, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0, fmt.Errorf("无法读取 /proc: %w", err)
	}

	var matches []int

	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return 0, fmt.Errorf("进程查找已取消: %w", err)
		}
		if !entry.IsDir() {
			continue
		}

		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue // 跳过非 PID 目录
		}

		// 读取 /proc/<pid>/comm 获取进程名
		commPath := fmt.Sprintf("/proc/%d/comm", pid)
		comm, err := os.ReadFile(commPath)
		if err != nil {
			continue // 可能进程已退出或无权限
		}

		// comm 文件以换行符结尾，需要 trim
		if strings.TrimSpace(string(comm)) == name {
			matches = append(matches, pid)
		}
	}

	switch len(matches) {
	case 0:
		return 0, fmt.Errorf("未找到名为 %q 的进程", name)
	case 1:
		return matches[0], nil
	default:
		return 0, fmt.Errorf("找到多个名为 %q 的进程 (PID: %v)，请使用 --pid 指定具体进程", name, matches)
	}
}

// findProcessBySocket 通过 Socket 文件查找进程 PID。
// 设计决策: 使用 /proc/net/unix 获取 socket inode，而非 os.Stat。
// 文件系统 inode（os.Stat 返回）和内核 socket inode（/proc/PID/fd 显示）
// 位于不同的编号空间，直接用 os.Stat inode 匹配永远不会成功。
func findProcessBySocket(ctx context.Context, socketPath string) (int, error) {
	absSocketPath, err := absPath(socketPath)
	if err != nil {
		return 0, fmt.Errorf("获取绝对路径失败: %w", err)
	}

	// 从 /proc/net/unix 查找 socket 的内核 inode
	socketIno, err := findSocketInode(absSocketPath)
	if err != nil {
		return 0, err
	}

	expectedLink := fmt.Sprintf("socket:[%d]", socketIno)
	matches, err := scanProcForSocket(ctx, expectedLink)
	if err != nil {
		return 0, err
	}

	switch len(matches) {
	case 0:
		return 0, fmt.Errorf("未找到监听 %s 的进程", socketPath)
	case 1:
		return matches[0], nil
	default:
		return 0, fmt.Errorf("找到多个持有 socket %s 的进程 (PID: %v)，请使用 --pid 指定具体进程",
			socketPath, matches)
	}
}

// scanProcForSocket 在 /proc 中扫描持有指定 socket 的进程。
func scanProcForSocket(ctx context.Context, expectedLink string) ([]int, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, fmt.Errorf("无法读取 /proc: %w", err)
	}

	var matches []int

	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("进程查找已取消: %w", err)
		}
		if !entry.IsDir() {
			continue
		}

		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue // 跳过非 PID 目录
		}

		if processHasSocket(pid, expectedLink) {
			matches = append(matches, pid)
		}
	}

	return matches, nil
}

// findSocketInode 从 /proc/net/unix 查找 Unix domain socket 的内核 inode。
func findSocketInode(absSocketPath string) (uint64, error) {
	data, err := os.ReadFile("/proc/net/unix")
	if err != nil {
		return 0, fmt.Errorf("无法读取 /proc/net/unix: %w", err)
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines[1:] { // 跳过表头
		fields := strings.Fields(line)
		// /proc/net/unix 格式: Num RefCount Protocol Flags Type St Inode [Path]
		if len(fields) >= 8 && fields[7] == absSocketPath {
			ino, parseErr := strconv.ParseUint(fields[6], 10, 64)
			if parseErr != nil {
				continue
			}
			return ino, nil
		}
	}

	return 0, fmt.Errorf("socket %s 未在 /proc/net/unix 中找到（服务可能未启动）", absSocketPath)
}

// processHasSocket 检查进程是否拥有指定的 socket fd。
func processHasSocket(pid int, expectedLink string) bool {
	fdDir := fmt.Sprintf("/proc/%d/fd", pid)

	entries, err := os.ReadDir(fdDir)
	if err != nil {
		return false
	}

	for _, entry := range entries {
		link, err := os.Readlink(fmt.Sprintf("%s/%s", fdDir, entry.Name()))
		if err != nil {
			continue
		}

		if link == expectedLink {
			return true
		}
	}

	return false
}

// absPath 获取绝对路径。
func absPath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("路径不能为空")
	}
	return filepath.Abs(path)
}

// setupSignalHandler 设置信号处理。
// 设计决策: 第一次信号优雅取消，第二次信号强制退出（退出码 130 = 128 + SIGINT）。
// 当命令阻塞时，用户可通过再次 Ctrl+C 强制退出。
// goroutine 在 ctx 取消时自动退出，避免测试中多次调用 run() 导致的泄漏。
func setupSignalHandler(ctx context.Context, cancel context.CancelFunc) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case <-sigCh:
			cancel() // 第一次信号: 优雅取消
		case <-ctx.Done():
			signal.Stop(sigCh)
			return
		}
		select {
		case <-sigCh:
			signal.Stop(sigCh)
			os.Exit(130) // 第二次信号: 强制退出
		case <-ctx.Done():
			signal.Stop(sigCh)
			return
		}
	}()
}

// isContainerEnvironment 检测是否运行在容器/K8s 环境中。
// 设计决策: 使用多种检测策略（环境变量、文件标志、cgroup），
// 容器标识符同时兼容 cgroup v1 和 v2（如 "kubepods" 在两种格式中均出现）。
func isContainerEnvironment() bool {
	// 检查 /.dockerenv 文件（Docker 容器标志）
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}

	// 检查 Kubernetes 环境变量
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		return true
	}

	// 检查 /proc/1/cgroup 是否包含容器相关信息（兼容 cgroup v1 和 v2）
	if data, err := os.ReadFile("/proc/1/cgroup"); err == nil {
		content := string(data)
		containerMarkers := []string{"docker", "kubepods", "containerd", "crio", "buildkit", "libpod", "lxc"}
		for _, marker := range containerMarkers {
			if strings.Contains(content, marker) {
				return true
			}
		}
	}

	return false
}
