//go:build !windows

package xdbg

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"
)

// Session 调试会话。
type Session struct {
	conn     net.Conn
	identity *IdentityInfo
	codec    *Codec
	server   *Server

	ctx    context.Context
	cancel context.CancelFunc

	mu     sync.Mutex
	closed bool // 阻止进一步写入（由 writeData 在写失败时设置，或由 Close 设置）

	// 设计决策: closed 与 connClosed 分离。writeData 写失败时仅设置 closed 阻止后续写入，
	// 但不执行资源清理。connClosed 确保 cancel()/audit(SessionEnd)/conn.Close() 恰好执行一次。
	// 若合并为单个标志，writeData 设置 closed=true 后，Run() defer 的 Close() 会因已关闭
	// 而跳过资源清理，导致 FD 泄漏和会话审计丢失。
	connClosed bool // 确保资源清理（cancel + audit + conn.Close）仅执行一次
}

// newSession 创建新会话。
func newSession(ctx context.Context, conn net.Conn, identity *PeerIdentity, server *Server) *Session {
	sessionCtx, cancel := context.WithCancel(ctx)

	return &Session{
		conn:     conn,
		identity: ResolveIdentity(identity),
		codec:    NewCodec(),
		server:   server,
		ctx:      sessionCtx,
		cancel:   cancel,
	}
}

// Run 运行会话。
func (s *Session) Run() {
	defer func() {
		if err := s.Close(); err != nil {
			s.server.audit(AuditEventCommandFailed, s.identity, "session:close", nil, 0, err)
		}
	}()

	// 记录会话开始
	s.server.audit(AuditEventSessionStart, s.identity, "", nil, 0, nil)

	for {
		if s.shouldExit() {
			return
		}

		req, ok := s.readRequest()
		if !ok {
			return
		}

		// 处理请求
		s.handleRequest(req)
	}
}

// shouldExit 检查会话是否应该退出。
func (s *Session) shouldExit() bool {
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		return true
	}

	select {
	case <-s.ctx.Done():
		return true
	default:
		return false
	}
}

// readRequest 读取一个请求。
// 返回读取到的请求和是否成功。如果返回 false，调用方应该退出循环。
func (s *Session) readRequest() (*Request, bool) {
	// 设置读超时防止 DoS
	if s.server.opts.SessionReadTimeout > 0 {
		if err := s.conn.SetReadDeadline(time.Now().Add(s.server.opts.SessionReadTimeout)); err != nil {
			s.server.audit(AuditEventCommandFailed, s.identity, "", nil, 0,
				fmt.Errorf("set read deadline failed: %w", err))
			return nil, false
		}
	}

	// 读取请求
	req, err := s.codec.DecodeRequest(s.conn)
	if err != nil {
		if errors.Is(err, ErrConnectionClosed) {
			return nil, false
		}
		// 检查是否是超时错误
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			s.sendError(ErrTimeout)
			return nil, false
		}
		s.sendError(fmt.Errorf("decode request: %w", err))
		return nil, false
	}

	// 清除读超时（命令执行有单独的超时控制）
	if err := s.conn.SetReadDeadline(time.Time{}); err != nil {
		s.server.audit(AuditEventCommandFailed, s.identity, "", nil, 0,
			fmt.Errorf("clear read deadline failed: %w", err))
		return nil, false
	}

	return req, true
}

// handleRequest 处理单个请求。
//
// 设计决策: 命令执行前仅校验"命令白名单"，不校验调用者身份（UID/GID/PID）。
// 安全边界由多层机制保证：Unix Socket 文件权限（0600）限制访问、SO_PEERCRED 记录
// 调用者身份到审计日志、命令白名单限制可用命令集。在 Kubernetes 环境中，
// kubectl exec 的 RBAC 策略提供外层访问控制。如需更细粒度的命令级授权，
// 可通过自定义 Command 实现在 Execute 内部检查身份。
func (s *Session) handleRequest(req *Request) {
	startTime := time.Now()

	// 记录命令开始
	s.server.audit(AuditEventCommand, s.identity, req.Command, req.Args, 0, nil)

	// 获取命令
	cmd := s.server.registry.Get(req.Command)
	if cmd == nil {
		err := ErrCommandNotFound
		s.server.audit(AuditEventCommandFailed, s.identity, req.Command, req.Args, time.Since(startTime), err)
		s.sendError(err)
		return
	}

	// 检查白名单
	if !s.server.registry.IsAllowed(req.Command) {
		err := ErrCommandForbidden
		s.server.audit(AuditEventCommandForbidden, s.identity, req.Command, req.Args, time.Since(startTime), err)
		s.sendError(err)
		return
	}

	// 尝试获取执行许可
	if !s.server.acquireCommandSlot() {
		err := ErrTooManyCommands
		s.server.audit(AuditEventCommandFailed, s.identity, req.Command, req.Args, time.Since(startTime), err)
		s.sendError(err)
		return
	}
	defer s.server.releaseCommandSlot()

	// 设计决策: CommandTimeout 通过 context.WithTimeout 实现，依赖命令协作式检查 ctx.Done()。
	// Go 无法强制终止 goroutine，若命令忽略 ctx 则会持续占用命令槽直到自然返回。
	// 这是 Go 并发模型的固有约束，与 http.Server、database/sql 等标准库一致。
	// 非协作命令的防护由 MaxConcurrentCommands 槽位限制和 SessionReadTimeout 提供。
	cmdCtx, cancel := context.WithTimeout(s.ctx, s.server.opts.CommandTimeout)
	defer cancel()

	// 执行命令（带 panic 保护）
	output, err := s.executeCommand(cmdCtx, cmd, req.Args)
	duration := time.Since(startTime)

	if err != nil {
		// 检查是否是超时
		if cmdCtx.Err() == context.DeadlineExceeded {
			err = ErrTimeout
		}
		s.server.audit(AuditEventCommandFailed, s.identity, req.Command, req.Args, duration, err)
		s.sendError(err)
		return
	}

	// 记录成功
	s.server.audit(AuditEventCommandSuccess, s.identity, req.Command, req.Args, duration, nil)

	// 检查是否需要截断输出
	resp := TruncateOutput(output, s.server.opts.MaxOutputSize)
	s.sendResponse(resp)
}

// executeCommand 在隔离边界内执行命令，捕获 panic 防止调试命令崩溃主进程。
//
// 设计决策: 调试通道不应成为故障放大器。任何命令（尤其是用户自定义命令）的 panic
// 应被转换为错误响应并记录审计日志，而非传播到主进程导致崩溃。
func (s *Session) executeCommand(ctx context.Context, cmd Command, args []string) (output string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("command panicked: %v", r)
		}
	}()
	return cmd.Execute(ctx, args)
}

// sendError 发送错误响应。
func (s *Session) sendError(err error) {
	s.sendResponse(NewErrorResponse(err))
}

// sendResponse 发送响应。
func (s *Session) sendResponse(resp *Response) {
	data, err := s.codec.EncodeResponse(resp)
	if err != nil {
		// 记录编码错误到审计日志
		s.server.audit(AuditEventCommandFailed, s.identity, "", nil, 0,
			fmt.Errorf("encode response failed: %w", err))
		// 编码失败时发送简化的错误响应，避免客户端阻塞
		s.sendEncodingErrorResponse()
		return
	}

	s.writeData(data)
}

// sendEncodingErrorResponse 发送编码错误响应。
// 当原始响应编码失败时（如输出过大），发送一个简化的错误响应，避免客户端阻塞。
func (s *Session) sendEncodingErrorResponse() {
	// 构造一个简单的错误响应（无 output 字段，避免再次失败）
	errResp := &Response{
		Success: false,
		Error:   "response encoding failed: output too large after JSON encoding",
	}

	data, err := s.codec.EncodeResponse(errResp)
	if err != nil {
		// 极端情况：连错误响应都无法编码，只能放弃
		s.server.audit(AuditEventCommandFailed, s.identity, "", nil, 0,
			fmt.Errorf("encode error response also failed: %w", err))
		return
	}

	s.writeData(data)
}

// writeData 将已编码的数据写入连接，带写超时保护。
// 调用方已完成编码，此方法负责加锁、设置超时、写入、清除超时。
func (s *Session) writeData(data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return
	}

	// 设置写超时，防止客户端不读取数据阻塞 goroutine
	if s.server.opts.SessionWriteTimeout > 0 {
		if err := s.conn.SetWriteDeadline(time.Now().Add(s.server.opts.SessionWriteTimeout)); err != nil {
			s.server.audit(AuditEventCommandFailed, s.identity, "", nil, 0,
				fmt.Errorf("set write deadline failed: %w", err))
			s.closed = true
			return
		}
	}

	if _, err := s.conn.Write(data); err != nil {
		s.server.audit(AuditEventCommandFailed, s.identity, "", nil, 0,
			fmt.Errorf("write response failed: %w", err))
		s.closed = true
		return
	}

	// 清除写超时
	if s.server.opts.SessionWriteTimeout > 0 {
		if err := s.conn.SetWriteDeadline(time.Time{}); err != nil {
			s.server.audit(AuditEventCommandFailed, s.identity, "", nil, 0,
				fmt.Errorf("clear write deadline failed: %w", err))
		}
	}
}

// Close 关闭会话，释放所有资源。
// 即使 writeData 已设置 closed=true（写失败），Close 仍会执行资源清理。
func (s *Session) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.connClosed {
		return nil
	}
	s.connClosed = true
	s.closed = true // 同时阻止后续写入

	s.cancel()

	// 记录会话结束
	s.server.audit(AuditEventSessionEnd, s.identity, "", nil, 0, nil)

	return s.conn.Close()
}
