//go:build !windows

package xdbg

import (
	"errors"
	"fmt"
	"os"
	"time"
)

// cleanupPprof 清理 pprof 资源。
func (s *Server) cleanupPprof() {
	if s.pprofCmd != nil {
		s.pprofCmd.Cleanup()
	}
}

// closeTransportAndTrigger 关闭传输层和触发器，返回聚合错误。
func (s *Server) closeTransportAndTrigger() error {
	var errs []error

	// 关闭传输层
	s.transportMu.Lock()
	if s.transport != nil {
		if err := s.transport.Close(); err != nil {
			s.audit(AuditEventCommandFailed, nil, "shutdown:transport", nil, 0, err)
			errs = append(errs, fmt.Errorf("close transport: %w", err))
		}
	}
	s.transportMu.Unlock()

	// 关闭触发器
	if s.trigger != nil {
		if err := s.trigger.Close(); err != nil {
			s.audit(AuditEventCommandFailed, nil, "shutdown:trigger", nil, 0, err)
			errs = append(errs, fmt.Errorf("close trigger: %w", err))
		}
	}

	return errors.Join(errs...)
}

// waitForGoroutines 等待所有 goroutine 完成。
func (s *Server) waitForGoroutines() {
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// 所有 goroutine 已正常退出
	case <-time.After(s.opts.ShutdownTimeout):
		// 超时警告：记录到审计日志
		s.audit(AuditEventCommandFailed, nil, "shutdown", nil, s.opts.ShutdownTimeout,
			fmt.Errorf("shutdown timeout after %v, some goroutines may still be running", s.opts.ShutdownTimeout))
	}
}

// closeAuditLogger 关闭审计日志。
func (s *Server) closeAuditLogger() {
	if s.opts.AuditLogger != nil {
		if err := s.opts.AuditLogger.Close(); err != nil {
			// 审计日志关闭失败，使用统一格式输出到 stderr 作为最后手段
			fmt.Fprintf(os.Stderr, "[%s] [XDBG] [%s] command=%s error=%q\n",
				time.Now().Format(time.RFC3339),
				AuditEventCommandFailed,
				"audit:close",
				err.Error())
		}
	}
}

// acquireCommandSlot 获取命令执行槽。
func (s *Server) acquireCommandSlot() bool {
	select {
	case <-s.commandSlots:
		return true
	default:
		return false
	}
}

// releaseCommandSlot 释放命令执行槽。
// 注意：必须保证每次 acquire 都有对应的 release，不能丢失槽位。
func (s *Server) releaseCommandSlot() {
	s.commandSlots <- struct{}{}
}

// audit 记录审计日志。
func (s *Server) audit(event AuditEvent, identity *IdentityInfo, command string, args []string, duration time.Duration, err error) {
	if s.opts.AuditLogger == nil {
		return
	}

	// 应用脱敏函数
	sanitizedArgs := args
	if s.opts.AuditSanitizer != nil && command != "" && len(args) > 0 {
		sanitizedArgs = s.opts.AuditSanitizer(command, args)
	}

	record := &AuditRecord{
		Timestamp: time.Now(),
		Event:     event,
		Identity:  identity,
		Command:   command,
		Args:      sanitizedArgs,
		Duration:  duration,
	}

	if err != nil {
		record.Error = err.Error()
	}

	s.opts.AuditLogger.Log(record)
}
