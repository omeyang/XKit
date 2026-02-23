//go:build !windows

package xdbg

import (
	"net"
	"runtime"
	"time"
)

// acceptLoop 接受连接循环。
func (s *Server) acceptLoop() {
	defer s.wg.Done()

	backoff := newAcceptBackoff()

	for {
		if s.shouldStopAccepting() {
			return
		}

		conn, identity, err := s.acceptConnection()
		if err != nil {
			if s.handleAcceptError(err, backoff) {
				return
			}
			continue
		}

		backoff.reset()
		s.handleNewConnection(conn, identity)
	}
}

// acceptBackoff 管理 Accept 错误时的指数退避。
type acceptBackoff struct {
	current time.Duration
	initial time.Duration
	max     time.Duration
}

func newAcceptBackoff() *acceptBackoff {
	return &acceptBackoff{
		initial: 5 * time.Millisecond,
		max:     1 * time.Second,
		current: 5 * time.Millisecond,
	}
}

func (b *acceptBackoff) reset() {
	b.current = b.initial
}

func (b *acceptBackoff) next() time.Duration {
	d := b.current
	b.current *= 2
	if b.current > b.max {
		b.current = b.max
	}
	return d
}

// shouldStopAccepting 检查是否应该停止接受连接。
func (s *Server) shouldStopAccepting() bool {
	select {
	case <-s.ctx.Done():
		return true
	default:
	}
	return ServerState(s.state.Load()) != ServerStateListening
}

// acceptConnection 接受一个连接。
// 注意：Accept 是阻塞操作，不持有锁以避免死锁。
// 如果在 Accept 期间 transport 被关闭，Accept 会返回错误。
func (s *Server) acceptConnection() (net.Conn, *PeerIdentity, error) {
	s.transportMu.Lock()
	transport := s.transport
	if transport == nil {
		s.transportMu.Unlock()
		return nil, nil, ErrNotRunning
	}
	s.transportMu.Unlock()

	// Accept 是阻塞操作，不能持有锁
	// 如果 transport 在 Accept 期间被关闭，Accept 会返回错误
	return transport.Accept()
}

// handleAcceptError 处理 Accept 错误，返回 true 表示应该停止循环。
func (s *Server) handleAcceptError(err error, backoff *acceptBackoff) bool {
	if ServerState(s.state.Load()) != ServerStateListening {
		return true
	}
	s.audit(AuditEventCommandFailed, nil, "accept", nil, 0, err)
	select {
	case <-s.ctx.Done():
		return true
	case <-time.After(backoff.next()):
		return false
	}
}

// handleNewConnection 处理新连接。
func (s *Server) handleNewConnection(conn net.Conn, identity *PeerIdentity) {
	// 设计决策: CAS 循环无硬性退出上限。循环必然终止——要么 CAS 成功，要么 sessionCount
	// 达到 MaxSessions 后 reject。调试服务并发度极低（MaxSessions 默认 1），CAS 竞争
	// 几乎不会发生。Gosched 退避防止极端情况下的 CPU 自旋。
	const maxCASRetries = 10
	for i := 0; ; i++ {
		current := s.sessionCount.Load()
		if int(current) >= s.opts.MaxSessions {
			s.rejectConnection(conn)
			return
		}
		if s.sessionCount.CompareAndSwap(current, current+1) {
			break
		}
		if i >= maxCASRetries {
			runtime.Gosched() // 让出 CPU 给其他 goroutine
		}
	}

	// 创建会话
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer s.sessionCount.Add(-1)

		session := newSession(s.ctx, conn, identity, s)
		session.Run()
	}()

	// 重置自动关闭定时器
	s.resetShutdownTimer()
}

// rejectConnection 拒绝连接（会话数超限）。
func (s *Server) rejectConnection(conn net.Conn) {
	// 设置写超时，防止慢客户端阻塞 accept 循环
	if s.opts.SessionWriteTimeout > 0 {
		if err := conn.SetWriteDeadline(time.Now().Add(s.opts.SessionWriteTimeout)); err != nil {
			s.audit(AuditEventCommandFailed, nil, "reject:deadline", nil, 0, err)
		}
	}
	codec := NewCodec()
	errResp := NewErrorResponse(ErrTooManySessions)
	if data, err := codec.EncodeResponse(errResp); err == nil {
		if _, writeErr := conn.Write(data); writeErr != nil {
			s.audit(AuditEventCommandFailed, nil, "reject:write", nil, 0, writeErr)
		}
	}
	if err := conn.Close(); err != nil {
		s.audit(AuditEventCommandFailed, nil, "reject:close", nil, 0, err)
	}
}
