//go:build !windows

package xdbg

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// Server 调试服务器。
type Server struct {
	opts     *options
	registry *CommandRegistry

	state           atomic.Int32
	transport       Transport
	transportMu     sync.Mutex
	customTransport bool // 标记是否使用了用户自定义的 Transport
	trigger         Trigger

	ctx    context.Context
	cancel context.CancelFunc

	wg              sync.WaitGroup
	sessionCount    atomic.Int32
	commandSlots    chan struct{}
	shutdownTimer   *time.Timer
	shutdownTimerMu sync.Mutex

	// pprofCmd 保存 pprof 命令引用，用于在 Stop 时清理资源
	pprofCmd *pprofCommand
}

// New 创建调试服务器。
//
// 设计决策: 返回具体类型 *Server 而非接口。xdbg 通过构建标签（!windows/windows）
// 在编译期选择平台实现，不需要运行时多态。测试可通过 WithTransport 注入自定义传输层。
// 这与 xpool、xbreaker、xlru 等包的构造函数签名一致。
func New(opts ...Option) (*Server, error) {
	options := defaultOptions()
	for _, opt := range opts {
		opt(options)
	}

	// 验证配置选项
	if err := validateOptions(options); err != nil {
		return nil, fmt.Errorf("invalid options: %w", err)
	}

	s := &Server{
		opts:         options,
		registry:     NewCommandRegistry(),
		commandSlots: make(chan struct{}, options.MaxConcurrentCommands),
	}

	// 设置命令白名单
	if options.CommandWhitelist != nil {
		s.registry.SetWhitelist(options.CommandWhitelist)
	}

	// 初始化命令槽
	for i := 0; i < options.MaxConcurrentCommands; i++ {
		s.commandSlots <- struct{}{}
	}

	// 注册内置命令
	s.registerBuiltinCommands()

	// 注册 xkit 集成命令
	s.registerXkitCommands()

	return s, nil
}

// Start 启动服务器。
// 服务器会在后台等待触发事件，收到触发后开始监听连接。
func (s *Server) Start(ctx context.Context) error {
	if !s.state.CompareAndSwap(int32(ServerStateCreated), int32(ServerStateStarted)) {
		return ErrAlreadyRunning
	}

	s.ctx, s.cancel = context.WithCancel(ctx)

	// 创建或使用自定义传输层
	if s.opts.Transport != nil {
		s.transport = s.opts.Transport
		s.customTransport = true
	} else {
		s.transport = NewUnixTransport(s.opts.SocketPath, os.FileMode(s.opts.SocketPerm))
	}

	// 创建触发器
	if !s.opts.BackgroundMode {
		s.trigger = NewSignalTrigger()
		s.wg.Add(1)
		go s.watchTrigger()
	}

	return nil
}

// Stop 停止服务器。
func (s *Server) Stop() error {
	// 设计决策: 使用 CAS 循环确保并发 Stop() 仅一个执行清理逻辑。
	// Load+Store 模式有竞态窗口（两个 goroutine 同时通过检查），
	// 可能导致 transport/auditLogger 的 double-close。
	for {
		state := ServerState(s.state.Load())
		if state == ServerStateStopped {
			return nil
		}
		if s.state.CompareAndSwap(int32(state), int32(ServerStateStopped)) {
			break
		}
	}

	// 取消上下文
	if s.cancel != nil {
		s.cancel()
	}

	// 停止自动关闭定时器
	s.stopShutdownTimer()

	// 清理 pprof 资源
	s.cleanupPprof()

	// 关闭传输层和触发器
	closeErr := s.closeTransportAndTrigger()

	// 等待所有 goroutine 完成
	s.waitForGoroutines()

	// 记录停止
	s.audit(AuditEventServerStop, nil, "", nil, 0, nil)

	// 关闭审计日志
	s.closeAuditLogger()

	return closeErr
}

// Enable 启用调试服务（开始监听）。
func (s *Server) Enable() error {
	return s.startListening()
}

// Disable 禁用调试服务（停止监听）。
func (s *Server) Disable() error {
	return s.stopListening()
}

// IsListening 返回是否正在监听。
func (s *Server) IsListening() bool {
	return ServerState(s.state.Load()) == ServerStateListening
}

// State 返回当前状态。
func (s *Server) State() ServerState {
	return ServerState(s.state.Load())
}

// RegisterCommand 注册自定义命令。
func (s *Server) RegisterCommand(cmd Command) {
	s.registry.Register(cmd)
}
