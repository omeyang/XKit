//go:build !windows

package xdbg

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"
)

// errConn 用于测试的 net.Conn mock，所有写操作返回错误。
type errConn struct {
	net.Conn
}

func (c *errConn) Write(_ []byte) (int, error) {
	return 0, errors.New("mock write error")
}

func (c *errConn) SetWriteDeadline(_ time.Time) error {
	return nil
}

func (c *errConn) SetReadDeadline(_ time.Time) error {
	return nil
}

func (c *errConn) Close() error {
	return nil
}

func TestSession_WriteData_WriteError(t *testing.T) {
	srv := &Server{
		opts: &options{AuditLogger: NewNoopAuditLogger()},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := &Session{
		conn:     &errConn{},
		codec:    NewCodec(),
		server:   srv,
		ctx:      ctx,
		cancel:   cancel,
		identity: &IdentityInfo{},
	}

	// Encode a valid response
	resp := NewSuccessResponse("hello")
	data, err := s.codec.EncodeResponse(resp)
	if err != nil {
		t.Fatalf("EncodeResponse() error = %v", err)
	}

	// writeData should handle the write error gracefully
	s.writeData(data)

	// Session should be marked as closed after write error
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()

	if !closed {
		t.Error("session should be closed after write error")
	}
}

func TestSession_WriteData_Closed(t *testing.T) {
	srv := &Server{
		opts: &options{AuditLogger: NewNoopAuditLogger()},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := &Session{
		conn:     &errConn{},
		codec:    NewCodec(),
		server:   srv,
		ctx:      ctx,
		cancel:   cancel,
		closed:   true, // Already closed
		identity: &IdentityInfo{},
	}

	// writeData on a closed session should return immediately (no panic)
	s.writeData([]byte("test"))
}

// setDeadlineErrConn 用于测试 SetWriteDeadline 错误。
type setDeadlineErrConn struct {
	net.Conn
}

func (c *setDeadlineErrConn) SetWriteDeadline(_ time.Time) error {
	return errors.New("mock deadline error")
}

func (c *setDeadlineErrConn) Close() error {
	return nil
}

func TestSession_WriteData_SetDeadlineError(t *testing.T) {
	srv := &Server{
		opts: &options{
			AuditLogger:         NewNoopAuditLogger(),
			SessionWriteTimeout: 30 * time.Second, // Enable write timeout
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := &Session{
		conn:     &setDeadlineErrConn{},
		codec:    NewCodec(),
		server:   srv,
		ctx:      ctx,
		cancel:   cancel,
		identity: &IdentityInfo{},
	}

	s.writeData([]byte("test"))

	// Session should be marked as closed after SetWriteDeadline error
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()

	if !closed {
		t.Error("session should be closed after SetWriteDeadline error")
	}
}

func TestSession_SendResponse_EncodeError(t *testing.T) {
	srv := &Server{
		opts: &options{AuditLogger: NewNoopAuditLogger()},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Use a pipe so writes can succeed
	clientConn, serverConn := net.Pipe()
	//nolint:errcheck // test cleanup: pipe 关闭失败不影响测试结果
	defer func() {
		_ = clientConn.Close() //nolint:errcheck // test cleanup
		_ = serverConn.Close() //nolint:errcheck // test cleanup
	}()

	s := &Session{
		conn:     serverConn,
		codec:    NewCodec(),
		server:   srv,
		ctx:      ctx,
		cancel:   cancel,
		identity: &IdentityInfo{},
	}

	// Create a response with output too large to encode
	largeOutput := make([]byte, MaxPayloadSize+1)
	for i := range largeOutput {
		largeOutput[i] = 'x'
	}
	resp := NewSuccessResponse(string(largeOutput))

	// Read in background to prevent blocking
	go func() {
		buf := make([]byte, 64*1024)
		for {
			if _, err := clientConn.Read(buf); err != nil {
				return
			}
		}
	}()

	// sendResponse should handle the encode error and send a fallback error response
	s.sendResponse(resp)
}

// readDeadlineErrConn 用于测试 SetReadDeadline 错误路径。
type readDeadlineErrConn struct {
	net.Conn
}

func (c *readDeadlineErrConn) SetReadDeadline(_ time.Time) error {
	return errors.New("mock read deadline error")
}

func (c *readDeadlineErrConn) Close() error {
	return nil
}

func TestSession_ReadRequest_SetReadDeadlineError(t *testing.T) {
	srv := &Server{
		opts: &options{
			AuditLogger:        NewNoopAuditLogger(),
			SessionReadTimeout: 30 * time.Second, // Enable read timeout
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := &Session{
		conn:     &readDeadlineErrConn{},
		codec:    NewCodec(),
		server:   srv,
		ctx:      ctx,
		cancel:   cancel,
		identity: &IdentityInfo{},
	}

	// readRequest should handle SetReadDeadline error gracefully
	req, ok := s.readRequest()
	if ok {
		t.Error("readRequest should return false on SetReadDeadline error")
	}
	if req != nil {
		t.Error("readRequest should return nil request on SetReadDeadline error")
	}
}

// closeErrConn 用于测试 Close 返回错误的场景。
type closeErrConn struct {
	net.Conn
	closeCalled bool
}

func (c *closeErrConn) Close() error {
	c.closeCalled = true
	return errors.New("mock close error")
}

func TestSession_Close_Error(t *testing.T) {
	srv := &Server{
		opts: &options{AuditLogger: NewNoopAuditLogger()},
	}

	ctx, cancel := context.WithCancel(context.Background())

	conn := &closeErrConn{}
	s := &Session{
		conn:     conn,
		codec:    NewCodec(),
		server:   srv,
		ctx:      ctx,
		cancel:   cancel,
		identity: &IdentityInfo{},
	}

	err := s.Close()
	if err == nil {
		t.Error("Close() should return error when conn.Close fails")
	}

	if !conn.closeCalled {
		t.Error("conn.Close() should have been called")
	}

	// Second close should be no-op
	err = s.Close()
	if err != nil {
		t.Errorf("second Close() should return nil, got %v", err)
	}
}

// FG-S1: 验证 writeData 写失败后 Close 仍然执行资源清理。
func TestSession_Close_AfterWriteError_StillCleansUp(t *testing.T) {
	srv := &Server{
		opts: &options{AuditLogger: NewNoopAuditLogger()},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	conn := &closeErrConn{}
	s := &Session{
		conn:     conn,
		codec:    NewCodec(),
		server:   srv,
		ctx:      ctx,
		cancel:   cancel,
		identity: &IdentityInfo{},
	}

	// 模拟 writeData 写失败：设置 closed=true 但不关闭连接
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()

	// Close 应该仍然执行资源清理（conn.Close 被调用）
	err := s.Close()
	if err == nil {
		t.Error("Close() should return conn.Close error")
	}

	if !conn.closeCalled {
		t.Error("conn.Close() should have been called even though closed=true from writeData")
	}
}

// FG-S2: 验证命令 panic 被捕获并转为错误响应。
func TestSession_ExecuteCommand_PanicRecovery(t *testing.T) {
	srv := &Server{
		opts: &options{AuditLogger: NewNoopAuditLogger()},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := &Session{
		server:   srv,
		ctx:      ctx,
		cancel:   cancel,
		identity: &IdentityInfo{},
	}

	panicCmd := NewCommandFunc("panic", "panics", func(_ context.Context, _ []string) (string, error) {
		panic("test panic")
	})

	output, err := s.executeCommand(context.Background(), panicCmd, nil)
	if err == nil {
		t.Fatal("executeCommand should return error on panic")
	}
	if output != "" {
		t.Errorf("output should be empty, got %q", output)
	}
	if !strings.Contains(err.Error(), "command panicked") {
		t.Errorf("error should mention panic, got %q", err.Error())
	}
}

// FG-S2: 验证正常命令不受 panic recovery 影响。
func TestSession_ExecuteCommand_Normal(t *testing.T) {
	srv := &Server{
		opts: &options{AuditLogger: NewNoopAuditLogger()},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := &Session{
		server:   srv,
		ctx:      ctx,
		cancel:   cancel,
		identity: &IdentityInfo{},
	}

	normalCmd := NewCommandFunc("normal", "works", func(_ context.Context, _ []string) (string, error) {
		return "ok", nil
	})

	output, err := s.executeCommand(context.Background(), normalCmd, nil)
	if err != nil {
		t.Fatalf("executeCommand should not error, got %v", err)
	}
	if output != "ok" {
		t.Errorf("output should be 'ok', got %q", output)
	}
}

// failCloseAuditLogger 用于测试 closeAuditLogger 错误路径。
type failCloseAuditLogger struct {
	noopAuditLogger
}

func (l *failCloseAuditLogger) Close() error {
	return errors.New("audit logger close failed")
}

func TestServer_CloseAuditLogger_Error(t *testing.T) {
	srv := &Server{
		opts: &options{AuditLogger: &failCloseAuditLogger{}},
	}

	// Should not panic, error is logged to stderr
	srv.closeAuditLogger()
}
