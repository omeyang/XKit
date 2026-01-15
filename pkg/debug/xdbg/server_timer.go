//go:build !windows

package xdbg

import (
	"time"
)

// startShutdownTimer 启动自动关闭定时器。
func (s *Server) startShutdownTimer() {
	if s.opts.AutoShutdown <= 0 {
		return
	}

	s.shutdownTimerMu.Lock()
	defer s.shutdownTimerMu.Unlock()

	s.shutdownTimer = time.AfterFunc(s.opts.AutoShutdown, func() {
		if err := s.stopListening(); err != nil {
			s.audit(AuditEventCommandFailed, nil, "auto-shutdown", nil, 0, err)
		}
	})
}

// stopShutdownTimer 停止自动关闭定时器。
func (s *Server) stopShutdownTimer() {
	s.shutdownTimerMu.Lock()
	defer s.shutdownTimerMu.Unlock()

	if s.shutdownTimer != nil {
		s.shutdownTimer.Stop()
		s.shutdownTimer = nil
	}
}

// resetShutdownTimer 重置自动关闭定时器。
func (s *Server) resetShutdownTimer() {
	if s.opts.AutoShutdown <= 0 {
		return
	}

	s.shutdownTimerMu.Lock()
	defer s.shutdownTimerMu.Unlock()

	if s.shutdownTimer != nil {
		s.shutdownTimer.Reset(s.opts.AutoShutdown)
	}
}
