//go:build !windows

package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/omeyang/xkit/pkg/debug/xdbg"
)

// executor 定义调试命令执行接口。
// 设计决策: 使用接口而非直接依赖 *Client，使 REPL 核心逻辑可被测试。
type executor interface {
	Execute(ctx context.Context, command string, args []string) (*xdbg.Response, error)
	Ping(ctx context.Context) error
}

// Client xdbg 客户端。
type Client struct {
	socketPath string
	timeout    time.Duration
	codec      *xdbg.Codec
}

// NewClient 创建客户端。
func NewClient(socketPath string, timeout time.Duration) *Client {
	return &Client{
		socketPath: socketPath,
		timeout:    timeout,
		codec:      xdbg.NewCodec(),
	}
}

// validateSocket 校验目标路径是否为 Unix Socket 文件，并验证所有者。
// 设计决策: 校验 socket 所有者匹配当前用户或 root，防止连接到非预期服务。
// 在 K8s 环境中通常以 root 运行，此检查不会产生误报。
func (c *Client) validateSocket() error {
	info, err := os.Lstat(c.socketPath)
	if err != nil {
		return fmt.Errorf("无法访问 Socket 路径 %s: %w", c.socketPath, err)
	}
	if info.Mode().Type()&os.ModeSocket == 0 {
		return fmt.Errorf("路径 %s 不是 Unix Socket 文件（类型: %s）", c.socketPath, info.Mode().Type())
	}

	// 校验 socket 所有者
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		uid := os.Getuid()
		if int(stat.Uid) != uid && stat.Uid != 0 {
			return fmt.Errorf("socket %s 的所有者 (UID=%d) 与当前用户 (UID=%d) 不匹配",
				c.socketPath, stat.Uid, uid)
		}
	}

	return nil
}

// setConnDeadline 设置基于 context 或超时的连接 deadline。
func (c *Client) setConnDeadline(ctx context.Context, conn net.Conn) error {
	if deadline, ok := ctx.Deadline(); ok {
		return conn.SetDeadline(deadline)
	}
	if c.timeout > 0 {
		return conn.SetDeadline(time.Now().Add(c.timeout))
	}
	return nil
}

// Execute 执行命令。
// 设计决策: 每次调用建立新连接（无连接复用）。
// 对 CLI 工具而言，无状态连接更简单且容错——目标进程可能随时重启或关闭 socket，
// 长连接反而需要断线重连逻辑。REPL 模式下的额外系统调用开销对交互式场景可忽略。
func (c *Client) Execute(ctx context.Context, command string, args []string) (_ *xdbg.Response, retErr error) {
	// 前置校验: 确认目标路径是 Unix Socket 文件
	if err := c.validateSocket(); err != nil {
		return nil, err
	}

	// 使用支持 context 的拨号器
	dialer := net.Dialer{Timeout: c.timeout}
	conn, err := dialer.DialContext(ctx, "unix", c.socketPath)
	if err != nil {
		return nil, fmt.Errorf("连接失败: %w", err)
	}
	defer func() {
		if closeErr := conn.Close(); closeErr != nil && retErr == nil {
			retErr = fmt.Errorf("关闭连接失败: %w", closeErr)
		}
	}()

	if err := c.setConnDeadline(ctx, conn); err != nil {
		return nil, fmt.Errorf("设置超时失败: %w", err)
	}

	// 编码并发送请求
	data, err := c.codec.EncodeRequest(&xdbg.Request{Command: command, Args: args})
	if err != nil {
		return nil, fmt.Errorf("编码请求失败: %w", err)
	}

	if _, err := conn.Write(data); err != nil {
		return nil, fmt.Errorf("发送请求失败: %w", err)
	}

	// 接收并解码响应
	resp, err := c.codec.DecodeResponse(conn)
	if err != nil {
		return nil, wrapDecodeError(err)
	}

	return resp, nil
}

// wrapDecodeError 包装解码错误，对版本不匹配场景提供更友好的提示。
func wrapDecodeError(err error) error {
	if errors.Is(err, xdbg.ErrInvalidMessage) && strings.Contains(err.Error(), "unsupported version") {
		return fmt.Errorf("协议版本不兼容: 服务端可能运行了不同版本的 xdbg，"+
			"请确保 xdbgctl 与目标进程使用相同版本的 XKit: %w", err)
	}
	return fmt.Errorf("接收响应失败: %w", err)
}

// Ping 测试连接。
// 设计决策: 通过发送 help 命令测试连通性。
// help 在 xdbg 服务端属于 essentialCommands，不受 WithCommandWhitelist 过滤，
// 因此始终可用（见 xdbg/command_registry.go:essentialCommands）。
func (c *Client) Ping(ctx context.Context) error {
	resp, err := c.Execute(ctx, "help", nil)
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf("ping 失败: %s", resp.Error)
	}
	return nil
}
