//go:build !windows

package xdbg

import (
	"context"
	"strings"
	"testing"
)

// mockLeveler 测试用的 mock 日志级别控制器。
type mockLeveler struct {
	level string
}

func newMockLeveler(level string) *mockLeveler {
	return &mockLeveler{level: level}
}

func (l *mockLeveler) Level() string {
	return l.level
}

func (l *mockLeveler) SetLevel(level string) error {
	l.level = level
	return nil
}

func TestHelpCommand(t *testing.T) {
	srv, err := New(
		WithBackgroundMode(true),
		WithAuditLogger(NewNoopAuditLogger()),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	cmd := srv.registry.Get("help")
	if cmd == nil {
		t.Fatal("help command not registered")
	}

	// 测试显示所有命令
	output, err := cmd.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !strings.Contains(output, "可用命令") {
		t.Error("output should contain '可用命令'")
	}

	if !strings.Contains(output, "help") {
		t.Error("output should contain 'help' command")
	}

	if !strings.Contains(output, "exit") {
		t.Error("output should contain 'exit' command")
	}
}

func TestHelpCommand_SpecificCommand(t *testing.T) {
	srv, err := New(
		WithBackgroundMode(true),
		WithAuditLogger(NewNoopAuditLogger()),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	cmd := srv.registry.Get("help")

	// 测试显示特定命令
	output, err := cmd.Execute(context.Background(), []string{"setlog"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !strings.Contains(output, "setlog") {
		t.Error("output should contain 'setlog'")
	}
}

func TestHelpCommand_UnknownCommand(t *testing.T) {
	srv, err := New(
		WithBackgroundMode(true),
		WithAuditLogger(NewNoopAuditLogger()),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	cmd := srv.registry.Get("help")

	// 测试未知命令
	_, err = cmd.Execute(context.Background(), []string{"unknown"})
	if err == nil {
		t.Error("expected error for unknown command")
	}
}

func TestExitCommand(t *testing.T) {
	srv, err := New(
		WithBackgroundMode(true),
		WithAuditLogger(NewNoopAuditLogger()),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	cmd := srv.registry.Get("exit")
	if cmd == nil {
		t.Fatal("exit command not registered")
	}

	output, err := cmd.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !strings.Contains(output, "关闭") {
		t.Error("output should mention closing")
	}
}

func TestSetlogCommand_NoLeveler(t *testing.T) {
	srv, err := New(
		WithBackgroundMode(true),
		WithAuditLogger(NewNoopAuditLogger()),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	cmd := srv.registry.Get("setlog")
	if cmd == nil {
		t.Fatal("setlog command not registered")
	}

	// 没有配置 Leveler
	_, err = cmd.Execute(context.Background(), []string{"debug"})
	if err == nil {
		t.Error("expected error when Leveler is not configured")
	}
}

func TestSetlogCommand_ShowLevel(t *testing.T) {
	leveler := newMockLeveler("info")

	srv, err := New(
		WithBackgroundMode(true),
		WithAuditLogger(NewNoopAuditLogger()),
		WithLeveler(leveler),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	cmd := srv.registry.Get("setlog")

	// 不带参数，显示当前级别
	output, err := cmd.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !strings.Contains(output, "info") {
		t.Error("output should contain current level 'info'")
	}
}

func TestSetlogCommand_SetLevel(t *testing.T) {
	leveler := newMockLeveler("info")

	srv, err := New(
		WithBackgroundMode(true),
		WithAuditLogger(NewNoopAuditLogger()),
		WithLeveler(leveler),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	cmd := srv.registry.Get("setlog")

	// 设置新级别
	output, err := cmd.Execute(context.Background(), []string{"debug"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !strings.Contains(output, "debug") {
		t.Error("output should confirm new level 'debug'")
	}

	if leveler.Level() != "debug" {
		t.Errorf("level = %q, want %q", leveler.Level(), "debug")
	}
}

func TestSetlogCommand_InvalidLevel(t *testing.T) {
	leveler := newMockLeveler("info")

	srv, err := New(
		WithBackgroundMode(true),
		WithAuditLogger(NewNoopAuditLogger()),
		WithLeveler(leveler),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	cmd := srv.registry.Get("setlog")

	// 无效级别
	_, err = cmd.Execute(context.Background(), []string{"invalid"})
	if err == nil {
		t.Error("expected error for invalid level")
	}
}

func TestStackCommand(t *testing.T) {
	cmd := newStackCommand()

	if cmd.Name() != "stack" {
		t.Errorf("Name() = %q, want %q", cmd.Name(), "stack")
	}

	output, err := cmd.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	// 输出应该包含 goroutine 信息
	if !strings.Contains(output, "goroutine") {
		t.Error("output should contain 'goroutine'")
	}
}

func TestFreememCommand(t *testing.T) {
	cmd := newFreememCommand()

	if cmd.Name() != "freemem" {
		t.Errorf("Name() = %q, want %q", cmd.Name(), "freemem")
	}

	output, err := cmd.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !strings.Contains(output, "HeapInuse") {
		t.Error("output should contain memory info")
	}
}

func TestPprofCommand_Usage(t *testing.T) {
	srv, err := New(
		WithBackgroundMode(true),
		WithAuditLogger(NewNoopAuditLogger()),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	cmd := srv.registry.Get("pprof")
	if cmd == nil {
		t.Fatal("pprof command not registered")
	}

	// 不带参数显示用法
	output, err := cmd.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !strings.Contains(output, "cpu start") {
		t.Error("output should show usage")
	}
}

func TestPprofCommand_Heap(t *testing.T) {
	srv, err := New(
		WithBackgroundMode(true),
		WithAuditLogger(NewNoopAuditLogger()),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	cmd := srv.registry.Get("pprof")

	output, err := cmd.Execute(context.Background(), []string{"heap"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !strings.Contains(output, "内存统计") {
		t.Error("output should contain memory stats")
	}
}

func TestPprofCommand_Goroutine(t *testing.T) {
	srv, err := New(
		WithBackgroundMode(true),
		WithAuditLogger(NewNoopAuditLogger()),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	cmd := srv.registry.Get("pprof")

	output, err := cmd.Execute(context.Background(), []string{"goroutine"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !strings.Contains(output, "Goroutine") {
		t.Error("output should contain goroutine info")
	}
}

func TestPprofCommand_CpuStartStop(t *testing.T) {
	srv, err := New(
		WithBackgroundMode(true),
		WithAuditLogger(NewNoopAuditLogger()),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	cmd := srv.registry.Get("pprof")

	// Start CPU profile
	output, err := cmd.Execute(context.Background(), []string{"cpu", "start"})
	if err != nil {
		t.Fatalf("cpu start error = %v", err)
	}

	if !strings.Contains(output, "已开始") {
		t.Error("output should confirm CPU profile started")
	}

	// Stop CPU profile
	output, err = cmd.Execute(context.Background(), []string{"cpu", "stop"})
	if err != nil {
		t.Fatalf("cpu stop error = %v", err)
	}

	if !strings.Contains(output, "已停止") {
		t.Error("output should confirm CPU profile stopped")
	}
}

func TestBuiltinCommandsRegistered(t *testing.T) {
	srv, err := New(
		WithBackgroundMode(true),
		WithAuditLogger(NewNoopAuditLogger()),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	expectedCmds := []string{"help", "exit", "setlog", "stack", "freemem", "pprof"}

	for _, name := range expectedCmds {
		if !srv.registry.Has(name) {
			t.Errorf("expected builtin command %q to be registered", name)
		}
	}
}
