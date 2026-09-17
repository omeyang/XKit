//go:build windows

package xdbg

import (
	"context"
	"errors"
)

// ErrWindowsNotSupported 表示 xdbg 在 Windows 上不支持。
var ErrWindowsNotSupported = errors.New("xdbg: not supported on Windows")

// ServerState 服务器状态。
type ServerState int32

const (
	ServerStateCreated ServerState = iota
	ServerStateStarted
	ServerStateListening
	ServerStateStopped
)

func (s ServerState) String() string {
	return "Unsupported"
}

// Server 调试服务器（Windows stub）。
type Server struct {
	opts     *options
	registry *CommandRegistry
}

// New 创建调试服务器（Windows 不支持）。
func New(opts ...Option) (*Server, error) {
	return nil, ErrWindowsNotSupported
}

// Start 启动服务器（Windows 不支持）。
func (s *Server) Start(_ context.Context) error {
	return ErrWindowsNotSupported
}

// Stop 停止服务器。
func (s *Server) Stop() error {
	return nil
}

// Enable 启用调试服务。
func (s *Server) Enable() error {
	return ErrWindowsNotSupported
}

// Disable 禁用调试服务。
func (s *Server) Disable() error {
	return nil
}

// IsListening 返回是否正在监听。
func (s *Server) IsListening() bool {
	return false
}

// State 返回当前状态。
func (s *Server) State() ServerState {
	return ServerStateStopped
}

// RegisterCommand 注册自定义命令。
func (s *Server) RegisterCommand(cmd Command) {
	if s.registry != nil {
		s.registry.Register(cmd)
	}
}
