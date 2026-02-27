//go:build !windows

// xdbgctl 是 xdbg 调试服务的命令行客户端。
//
// 用法:
//
//	xdbgctl [全局选项] <命令> [命令参数]
//
// 全局选项:
//
//	-s, --socket   Unix Socket 路径 (默认: /var/run/xdbg.sock)
//	-t, --timeout  命令超时时间 (默认: 30s, 上限: 5m)
//
// 命令:
//
//	toggle         切换调试服务状态（发送 SIGUSR1 信号）
//	disable        禁用调试服务
//	exec <cmd>     执行调试命令
//	status         查看服务状态
//	interactive    交互模式（REPL）
//	help           显示帮助信息
//
// toggle 命令说明:
//
//	SIGUSR1 信号触发的是 toggle 操作（启用↔禁用），而非单纯的 enable。
//	进程发现优先级：--pid > --name > socket 文件发现
//
//	注意：socket 文件发现需要调试服务已启用（即 socket 文件已创建）。
//	若服务未启用，需使用 --pid 或 --name 参数指定目标进程。
//
//	安全性：发送信号后会验证目标进程仍然存活。若进程在收到 SIGUSR1 后退出
//	（说明目标可能不是 xdbg 服务进程），将报告错误而非静默返回成功。
//
// 退出码:
//
//	0: 命令执行成功（status 命令: 服务在线）
//	1: 命令执行失败或服务离线（status 命令）
//	2: 参数错误（无效 PID、缺少必需参数、未知命令等）
//
// 示例:
//
//	xdbgctl toggle                        # 切换调试服务状态（通过 socket 发现进程）
//	xdbgctl toggle --pid 1234             # 切换指定 PID 进程的调试服务
//	xdbgctl toggle --name myapp           # 通过进程名查找并切换调试服务
//	xdbgctl exec help                     # 列出可用命令
//	xdbgctl exec setlog debug             # 设置日志级别
//	xdbgctl interactive                   # 进入交互模式
//	xdbgctl -s /tmp/app.sock exec help    # 使用自定义 Socket 路径
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/omeyang/xkit/pkg/debug/xdbg"
	"github.com/urfave/cli/v3"
)

// defaultTimeout 默认超时时间。
const defaultTimeout = 30 * time.Second

// 版本信息（可通过 -ldflags 注入，例如:
//
//	go build -ldflags "-X main.Version=1.0.0 -X main.GitCommit=$(git rev-parse --short HEAD) -X main.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
//
// ）。
var (
	Version   = "0.1.0-dev"
	GitCommit = "unknown"
	BuildTime = "unknown"
)

func main() {
	os.Exit(run())
}

// createApp 创建 CLI 应用。
func createApp() *cli.Command {
	return &cli.Command{
		Name:    "xdbgctl",
		Usage:   "xdbg 调试服务命令行客户端",
		Version: fmt.Sprintf("%s (commit: %s, built: %s)", Version, GitCommit, BuildTime),
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "socket",
				Aliases: []string{"s"},
				Usage:   "Unix Socket 路径",
				Value:   xdbg.DefaultSocketPath,
			},
			&cli.DurationFlag{
				Name:    "timeout",
				Aliases: []string{"t"},
				Usage:   "命令超时时间",
				Value:   defaultTimeout,
			},
		},
		Commands:       createCommands(),
		DefaultCommand: "help",
		Authors: []any{
			"XKit Team",
		},
		// 设计决策: 禁止 urfave/cli 直接调用 os.Exit，
		// 由 run() 统一处理退出码映射，确保与文档退出码契约一致。
		ExitErrHandler: func(_ context.Context, _ *cli.Command, err error) {
			// ExitCoder 错误（如未知命令）的消息需在此输出，
			// 替代 HandleExitCoder 的默认 os.Exit 行为。
			if _, ok := err.(cli.ExitCoder); ok {
				fmt.Fprintln(os.Stderr, err)
			}
		},
		Description: `xdbgctl 是 xdbg 调试服务的命令行客户端，用于在 K8s 环境中
对运行中的 Go 应用进行动态调试。

主要命令:
  toggle              切换调试服务状态（SIGUSR1 信号）
    --pid, -p         目标进程 PID（优先级最高）
    --name, -n        目标进程名称（从 /proc/*/comm 查找）

调试命令:
  help                显示可用命令列表
  setlog [级别]       查看/设置日志级别 (trace/debug/info/warn/error)
  stack               打印所有 goroutine 堆栈
  freemem             释放内存到操作系统
  pprof <子命令>      性能分析 (cpu start/stop, heap, goroutine)
  breaker [名称]      查看熔断器状态
  limit [名称]        查看限流器状态
  cache [名称]        查看缓存统计
  config              查看运行时配置
  exit                关闭调试服务`,
	}
}

func run() int {
	app := createApp()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 设置信号处理
	setupSignalHandler(ctx, cancel)

	if err := app.Run(ctx, os.Args); err != nil {
		var exitErr *exitError
		if errors.As(err, &exitErr) {
			return exitErr.code
		}
		var usageErr *usageError
		if errors.As(err, &usageErr) {
			fmt.Fprintf(os.Stderr, "参数错误: %v\n", usageErr)
			return 2
		}
		// CLI 框架产生的参数错误（如未知 flag、未知命令）也返回退出码 2，
		// 与文档契约"参数错误 → 退出码 2"保持一致。
		if isCLIUsageError(err) {
			// ExitErrHandler 或 flag 解析器已向 stderr 输出错误详情，此处仅设置退出码
			return 2
		}
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		return 1
	}

	return 0
}
