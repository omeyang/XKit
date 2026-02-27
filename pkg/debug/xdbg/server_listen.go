//go:build !windows

package xdbg

import (
	"fmt"
	"os"
)

// startListening 开始监听连接。
func (s *Server) startListening() error {
	if !s.state.CompareAndSwap(int32(ServerStateStarted), int32(ServerStateListening)) {
		// CAS 失败，检查当前状态以返回适当的错误
		currentState := ServerState(s.state.Load())
		switch currentState {
		case ServerStateCreated:
			return fmt.Errorf("%w: server not started, call Start() first", ErrInvalidState)
		case ServerStateListening:
			return nil // 幂等：已在监听
		case ServerStateStopped:
			return fmt.Errorf("%w: server has been stopped", ErrInvalidState)
		default:
			return fmt.Errorf("%w: unexpected state %v", ErrInvalidState, currentState)
		}
	}

	// 开始监听
	s.transportMu.Lock()
	transport := s.transport
	s.transportMu.Unlock()

	if err := transport.Listen(s.ctx); err != nil {
		// 设计决策: 使用 CAS 而非 Store 回滚状态。若并发 Stop() 已将状态设为 Stopped，
		// 无条件 Store(Started) 会将终态回退为 Started，破坏生命周期语义。
		// CAS 仅在当前仍为 Listening 时才回滚到 Started，保持 Stopped 终态不变。
		s.state.CompareAndSwap(int32(ServerStateListening), int32(ServerStateStarted))
		return fmt.Errorf("start listening: %w", err)
	}

	// 记录启动
	s.audit(AuditEventServerStart, nil, "", nil, 0, nil)

	// 启动自动关闭定时器
	s.startShutdownTimer()

	// 启动接受连接的 goroutine
	s.wg.Add(1)
	go s.acceptLoop()

	return nil
}

// stopListening 停止监听连接。
func (s *Server) stopListening() error {
	if !s.state.CompareAndSwap(int32(ServerStateListening), int32(ServerStateStarted)) {
		// CAS 失败，检查当前状态以返回适当的错误
		currentState := ServerState(s.state.Load())
		switch currentState {
		case ServerStateCreated:
			return fmt.Errorf("%w: server not started, call Start() first", ErrInvalidState)
		case ServerStateStarted:
			return nil // 幂等：已停止监听
		case ServerStateStopped:
			return fmt.Errorf("%w: server has been stopped", ErrInvalidState)
		default:
			return fmt.Errorf("%w: unexpected state %v", ErrInvalidState, currentState)
		}
	}

	// 停止自动关闭定时器
	s.stopShutdownTimer()

	// 关闭传输层（这会导致 acceptLoop 退出）
	// 设计决策: transport.Close 错误仅写审计日志，不向调用方返回。
	// 此时状态已从 Listening 转为 Started，新的 transport 即将创建（非自定义场景），
	// 旧 transport 的关闭错误不影响后续 Enable→Listen 流程。
	// 与 Stop() 不同，Disable() 是可恢复操作，调用方无需感知关闭细节。
	s.transportMu.Lock()
	if s.transport != nil {
		if err := s.transport.Close(); err != nil {
			s.audit(AuditEventCommandFailed, nil, "stopListening:transport", nil, 0, err)
		}
		// 设计决策: 仅当不是自定义传输层时才重新创建。
		// 内置 UnixTransport 的 Close 是终态（设置 closed=true），因此需要重建。
		// 自定义传输层由用户管理生命周期：若需 Enable→Disable→Enable 循环，
		// 其 Close 实现应仅关闭监听器而非标记终态，以支持后续 Listen 调用。
		// 参见 WithTransport 文档。
		if !s.customTransport {
			s.transport = NewUnixTransport(s.opts.SocketPath, os.FileMode(s.opts.SocketPerm))
		}
	}
	s.transportMu.Unlock()

	return nil
}
