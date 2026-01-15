//go:build !windows

package xdbg

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestServer_New(t *testing.T) {
	srv, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if srv == nil {
		t.Fatal("New() returned nil server")
	}

	if srv.State() != ServerStateCreated {
		t.Errorf("State() = %v, want %v", srv.State(), ServerStateCreated)
	}
}

func TestServer_NewWithOptions(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	srv, err := New(
		WithSocketPath(socketPath),
		WithAutoShutdown(1*time.Minute),
		WithMaxSessions(2),
		WithBackgroundMode(true),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if srv.opts.SocketPath != socketPath {
		t.Errorf("SocketPath = %q, want %q", srv.opts.SocketPath, socketPath)
	}

	if srv.opts.AutoShutdown != 1*time.Minute {
		t.Errorf("AutoShutdown = %v, want %v", srv.opts.AutoShutdown, 1*time.Minute)
	}

	if srv.opts.MaxSessions != 2 {
		t.Errorf("MaxSessions = %d, want %d", srv.opts.MaxSessions, 2)
	}

	if !srv.opts.BackgroundMode {
		t.Error("BackgroundMode should be true")
	}
}

func TestServer_NewWithInvalidOptions(t *testing.T) {
	tests := []struct {
		name string
		opt  Option
	}{
		{
			name: "zero MaxSessions",
			opt:  WithMaxSessions(0),
		},
		{
			name: "negative MaxSessions",
			opt:  WithMaxSessions(-1),
		},
		{
			name: "zero MaxConcurrentCommands",
			opt:  WithMaxConcurrentCommands(0),
		},
		{
			name: "zero MaxOutputSize",
			opt:  WithMaxOutputSize(0),
		},
		{
			name: "zero CommandTimeout",
			opt:  WithCommandTimeout(0),
		},
		{
			name: "zero ShutdownTimeout",
			opt:  WithShutdownTimeout(0),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.opt)
			if err == nil {
				t.Errorf("New() with %s should return error", tt.name)
			}
		})
	}
}

func TestServer_StartStop(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	srv, err := New(
		WithSocketPath(socketPath),
		WithBackgroundMode(true), // 后台模式，不监听信号
		WithAuditLogger(NewNoopAuditLogger()),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx := context.Background()

	// 启动
	err = srv.Start(ctx)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if srv.State() != ServerStateStarted {
		t.Errorf("State() = %v, want %v", srv.State(), ServerStateStarted)
	}

	// 停止
	err = srv.Stop()
	if err != nil {
		t.Errorf("Stop() error = %v", err)
	}

	if srv.State() != ServerStateStopped {
		t.Errorf("State() = %v, want %v", srv.State(), ServerStateStopped)
	}
}

func TestServer_EnableDisable(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	srv, err := New(
		WithSocketPath(socketPath),
		WithBackgroundMode(true),
		WithAutoShutdown(0), // 禁用自动关闭
		WithAuditLogger(NewNoopAuditLogger()),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx := context.Background()
	err = srv.Start(ctx)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	//nolint:errcheck // test cleanup
	defer func() { _ = srv.Stop() }()

	// 启用
	err = srv.Enable()
	if err != nil {
		t.Fatalf("Enable() error = %v", err)
	}

	if !srv.IsListening() {
		t.Error("IsListening() should be true after Enable()")
	}

	if srv.State() != ServerStateListening {
		t.Errorf("State() = %v, want %v", srv.State(), ServerStateListening)
	}

	// 禁用
	err = srv.Disable()
	if err != nil {
		t.Errorf("Disable() error = %v", err)
	}

	// 等待状态更新
	time.Sleep(100 * time.Millisecond)

	if srv.IsListening() {
		t.Error("IsListening() should be false after Disable()")
	}
}

func TestServer_StartTwice(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	srv, err := New(
		WithSocketPath(socketPath),
		WithBackgroundMode(true),
		WithAuditLogger(NewNoopAuditLogger()),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx := context.Background()
	err = srv.Start(ctx)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	//nolint:errcheck // test cleanup
	defer func() { _ = srv.Stop() }()

	// 第二次启动应该返回错误
	err = srv.Start(ctx)
	if err != ErrAlreadyRunning {
		t.Errorf("second Start() error = %v, want ErrAlreadyRunning", err)
	}
}

func TestServer_StopIdempotent(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	srv, err := New(
		WithSocketPath(socketPath),
		WithBackgroundMode(true),
		WithAuditLogger(NewNoopAuditLogger()),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx := context.Background()
	err = srv.Start(ctx)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// 第一次停止
	err = srv.Stop()
	if err != nil {
		t.Errorf("first Stop() error = %v", err)
	}

	// 第二次停止应该也是成功的（幂等）
	err = srv.Stop()
	if err != nil {
		t.Errorf("second Stop() error = %v", err)
	}
}

func TestServer_RegisterCommand(t *testing.T) {
	srv, err := New(
		WithBackgroundMode(true),
		WithAuditLogger(NewNoopAuditLogger()),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	cmd := NewCommandFunc("test", "test command", func(_ context.Context, _ []string) (string, error) {
		return "test output", nil
	})

	srv.RegisterCommand(cmd)

	if !srv.registry.Has("test") {
		t.Error("command 'test' should be registered")
	}
}

func TestServerState_String(t *testing.T) {
	tests := []struct {
		state ServerState
		want  string
	}{
		{ServerStateCreated, "Created"},
		{ServerStateStarted, "Started"},
		{ServerStateListening, "Listening"},
		{ServerStateStopped, "Stopped"},
		{ServerState(99), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.state.String(); got != tt.want {
				t.Errorf("ServerState.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

// mockTrigger 用于测试的模拟触发器。
type mockTrigger struct {
	eventCh chan TriggerEvent
	closed  bool
}

func newMockTrigger() *mockTrigger {
	return &mockTrigger{
		eventCh: make(chan TriggerEvent, 10),
	}
}

func (t *mockTrigger) Watch(_ context.Context) <-chan TriggerEvent {
	return t.eventCh
}

func (t *mockTrigger) Close() error {
	if !t.closed {
		t.closed = true
		close(t.eventCh)
	}
	return nil
}

func (t *mockTrigger) Send(event TriggerEvent) {
	if !t.closed {
		t.eventCh <- event
	}
}

func TestServer_HandleTriggerEvent_Enable(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	srv, err := New(
		WithSocketPath(socketPath),
		WithBackgroundMode(true),
		WithAutoShutdown(0),
		WithAuditLogger(NewNoopAuditLogger()),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx := context.Background()
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	//nolint:errcheck // test cleanup
	defer func() { _ = srv.Stop() }()

	// 使用 handleTriggerEvent 启用
	srv.handleTriggerEvent(TriggerEventEnable)

	// 等待状态更新
	time.Sleep(100 * time.Millisecond)

	if !srv.IsListening() {
		t.Error("server should be listening after TriggerEventEnable")
	}
}

func TestServer_HandleTriggerEvent_Disable(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	srv, err := New(
		WithSocketPath(socketPath),
		WithBackgroundMode(true),
		WithAutoShutdown(0),
		WithAuditLogger(NewNoopAuditLogger()),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx := context.Background()
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	//nolint:errcheck // test cleanup
	defer func() { _ = srv.Stop() }()

	// 先启用
	if err := srv.Enable(); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	if !srv.IsListening() {
		t.Fatal("server should be listening before disable test")
	}

	// 使用 handleTriggerEvent 禁用
	srv.handleTriggerEvent(TriggerEventDisable)

	// 等待状态更新
	time.Sleep(100 * time.Millisecond)

	if srv.IsListening() {
		t.Error("server should not be listening after TriggerEventDisable")
	}
}

func TestServer_HandleTriggerEvent_Toggle(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	srv, err := New(
		WithSocketPath(socketPath),
		WithBackgroundMode(true),
		WithAutoShutdown(0),
		WithAuditLogger(NewNoopAuditLogger()),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx := context.Background()
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	//nolint:errcheck // test cleanup
	defer func() { _ = srv.Stop() }()

	// 初始状态：未监听
	if srv.IsListening() {
		t.Fatal("server should not be listening initially")
	}

	// Toggle：应该启用
	srv.handleTriggerEvent(TriggerEventToggle)
	time.Sleep(100 * time.Millisecond)

	if !srv.IsListening() {
		t.Error("server should be listening after first toggle")
	}

	// Toggle：应该禁用
	srv.handleTriggerEvent(TriggerEventToggle)
	time.Sleep(100 * time.Millisecond)

	if srv.IsListening() {
		t.Error("server should not be listening after second toggle")
	}
}

func TestServer_WatchTrigger(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	srv, err := New(
		WithSocketPath(socketPath),
		WithBackgroundMode(true), // 使用后台模式先创建
		WithAutoShutdown(0),
		WithAuditLogger(NewNoopAuditLogger()),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// 手动设置模拟触发器
	mockTrig := newMockTrigger()
	srv.trigger = mockTrig

	ctx, cancel := context.WithCancel(context.Background())
	srv.ctx = ctx
	srv.cancel = cancel

	// 启动 watchTrigger
	srv.wg.Add(1)
	go srv.watchTrigger()

	// 发送 Enable 事件
	mockTrig.Send(TriggerEventEnable)

	// 等待处理
	time.Sleep(100 * time.Millisecond)

	// 验证状态（由于 transport 未初始化，可能不会真正监听，但事件应该被处理）

	// 关闭
	cancel()
	srv.wg.Wait()
}

func TestServer_AutoShutdown(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	srv, err := New(
		WithSocketPath(socketPath),
		WithBackgroundMode(true),
		WithAutoShutdown(200*time.Millisecond), // 200ms 后自动关闭
		WithAuditLogger(NewNoopAuditLogger()),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx := context.Background()
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	//nolint:errcheck // test cleanup
	defer func() { _ = srv.Stop() }()

	// 启用监听
	if err := srv.Enable(); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}

	if !srv.IsListening() {
		t.Fatal("server should be listening after Enable")
	}

	// 等待自动关闭
	time.Sleep(300 * time.Millisecond)

	if srv.IsListening() {
		t.Error("server should have auto-shutdown after timeout")
	}
}

func TestServer_ResetShutdownTimer(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	srv, err := New(
		WithSocketPath(socketPath),
		WithBackgroundMode(true),
		WithAutoShutdown(200*time.Millisecond),
		WithAuditLogger(NewNoopAuditLogger()),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx := context.Background()
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	//nolint:errcheck // test cleanup
	defer func() { _ = srv.Stop() }()

	// 启用监听
	if err := srv.Enable(); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}

	// 在定时器触发前重置
	time.Sleep(100 * time.Millisecond)
	srv.resetShutdownTimer()

	// 再等 150ms（总共 250ms），如果没有重置，应该已经关闭了
	time.Sleep(150 * time.Millisecond)

	// 由于重置了定时器，应该还在监听
	if !srv.IsListening() {
		t.Error("server should still be listening after timer reset")
	}

	// 再等 100ms，定时器应该触发
	time.Sleep(100 * time.Millisecond)

	if srv.IsListening() {
		t.Error("server should have auto-shutdown after reset timer expired")
	}
}

func TestServer_CommandSlots(t *testing.T) {
	srv, err := New(
		WithBackgroundMode(true),
		WithMaxConcurrentCommands(2),
		WithAuditLogger(NewNoopAuditLogger()),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// 获取两个槽位
	if !srv.acquireCommandSlot() {
		t.Error("should acquire first slot")
	}
	if !srv.acquireCommandSlot() {
		t.Error("should acquire second slot")
	}

	// 第三个应该失败
	if srv.acquireCommandSlot() {
		t.Error("should not acquire third slot")
	}

	// 释放一个
	srv.releaseCommandSlot()

	// 现在应该可以获取
	if !srv.acquireCommandSlot() {
		t.Error("should acquire slot after release")
	}
}

func TestServer_EnableBeforeStart(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	srv, err := New(
		WithSocketPath(socketPath),
		WithBackgroundMode(true),
		WithAuditLogger(NewNoopAuditLogger()),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// 在 Start 之前调用 Enable 应该不会崩溃
	// 但由于状态不是 Started，可能不会真正启动监听
	err = srv.Enable()
	// Enable 内部会检查状态，可能返回 nil 或不做任何事
	if err != nil {
		t.Logf("Enable before Start error = %v (expected)", err)
	}
}

func TestServer_DisableNotListening(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	srv, err := New(
		WithSocketPath(socketPath),
		WithBackgroundMode(true),
		WithAutoShutdown(0),
		WithAuditLogger(NewNoopAuditLogger()),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx := context.Background()
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	//nolint:errcheck // test cleanup
	defer func() { _ = srv.Stop() }()

	// 在未监听时调用 Disable 应该是安全的
	err = srv.Disable()
	if err != nil {
		t.Errorf("Disable() when not listening should not error, got: %v", err)
	}
}

func TestServer_AuditLogging(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	// 使用一个记录调用的审计日志器
	auditLogs := make([]AuditEvent, 0)
	customAudit := &testAuditLogger{
		logs: &auditLogs,
	}

	srv, err := New(
		WithSocketPath(socketPath),
		WithBackgroundMode(true),
		WithAutoShutdown(0),
		WithAuditLogger(customAudit),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx := context.Background()
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// 启用
	if err := srv.Enable(); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	// 检查是否记录了 ServerStart 事件
	hasStart := false
	for _, event := range auditLogs {
		if event == AuditEventServerStart {
			hasStart = true
			break
		}
	}
	if !hasStart {
		t.Error("should have logged ServerStart event")
	}

	// 停止
	//nolint:errcheck // test cleanup: 测试服务器停止失败不影响测试结果
	_ = srv.Stop()

	// 检查是否记录了 ServerStop 事件
	hasStop := false
	for _, event := range auditLogs {
		if event == AuditEventServerStop {
			hasStop = true
			break
		}
	}
	if !hasStop {
		t.Error("should have logged ServerStop event")
	}
}

// testAuditLogger 用于测试的审计日志器。
type testAuditLogger struct {
	logs *[]AuditEvent
	mu   sync.Mutex
}

func (l *testAuditLogger) Log(record *AuditRecord) {
	l.mu.Lock()
	defer l.mu.Unlock()
	*l.logs = append(*l.logs, record.Event)
}

func (l *testAuditLogger) Close() error {
	return nil
}
