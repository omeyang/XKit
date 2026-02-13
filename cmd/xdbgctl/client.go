//go:build !windows

package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/omeyang/xkit/pkg/debug/xdbg"
)

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

// validateSocket 校验目标路径是否为 Unix Socket 文件。
func (c *Client) validateSocket() error {
	info, err := os.Lstat(c.socketPath)
	if err != nil {
		return fmt.Errorf("无法访问 Socket 路径 %s: %w", c.socketPath, err)
	}
	if info.Mode().Type()&os.ModeSocket == 0 {
		return fmt.Errorf("路径 %s 不是 Unix Socket 文件（类型: %s）", c.socketPath, info.Mode().Type())
	}
	return nil
}

// Execute 执行命令。
func (c *Client) Execute(ctx context.Context, command string, args []string) (*xdbg.Response, error) {
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
	defer func() { _ = conn.Close() }() //nolint:errcheck // defer cleanup: Close 错误在连接关闭时无法有效处理

	// 设置基于 context 的 deadline
	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetDeadline(deadline); err != nil {
			return nil, fmt.Errorf("设置超时失败: %w", err)
		}
	} else if c.timeout > 0 {
		if err := conn.SetDeadline(time.Now().Add(c.timeout)); err != nil {
			return nil, fmt.Errorf("设置超时失败: %w", err)
		}
	}

	// 构建请求
	req := &xdbg.Request{
		Command: command,
		Args:    args,
	}

	// 编码并发送请求
	data, err := c.codec.EncodeRequest(req)
	if err != nil {
		return nil, fmt.Errorf("编码请求失败: %w", err)
	}

	if _, err := conn.Write(data); err != nil {
		return nil, fmt.Errorf("发送请求失败: %w", err)
	}

	// 接收并解码响应
	resp, err := c.codec.DecodeResponse(conn)
	if err != nil {
		return nil, fmt.Errorf("接收响应失败: %w", err)
	}

	return resp, nil
}

// Ping 测试连接。
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
