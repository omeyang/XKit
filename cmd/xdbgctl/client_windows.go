//go:build windows

package main

import (
	"context"
	"fmt"
	"time"
)

// Response 响应消息。
type Response struct {
	Success      bool   `json:"success"`
	Output       string `json:"output,omitempty"`
	Error        string `json:"error,omitempty"`
	Truncated    bool   `json:"truncated,omitempty"`
	OriginalSize int    `json:"original_size,omitempty"`
}

// Client xdbg 客户端。
type Client struct {
	socketPath string
	timeout    time.Duration
}

// NewClient 创建客户端。
func NewClient(socketPath string, timeout time.Duration) *Client {
	return &Client{
		socketPath: socketPath,
		timeout:    timeout,
	}
}

// Execute 执行命令（Windows 不支持）。
func (c *Client) Execute(_ context.Context, _ string, _ []string) (*Response, error) {
	return nil, fmt.Errorf("xdbgctl: Windows 平台不支持")
}

// Ping 测试连接（Windows 不支持）。
func (c *Client) Ping(_ context.Context) error {
	return fmt.Errorf("xdbgctl: Windows 平台不支持")
}
