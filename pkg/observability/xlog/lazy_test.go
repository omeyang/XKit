package xlog_test

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/omeyang/xkit/pkg/observability/xlog"
)

// =============================================================================
// Lazy 求值测试
// =============================================================================

// testError 实现 error 接口用于测试
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

// lazyTestCase Lazy 测试用例结构
type lazyTestCase struct {
	name         string
	lazyType     string
	level        xlog.Level
	lazyAttr     func(*bool) slog.Attr // 返回 slog.Attr
	wantCalled   bool
	wantContains string
}

// getLazyTestCases 返回所有 Lazy 测试用例
func getLazyTestCases() []lazyTestCase {
	return []lazyTestCase{
		// Lazy (any) 测试
		{
			name: "Lazy_enabled", lazyType: "Lazy", level: xlog.LevelDebug,
			lazyAttr: func(called *bool) slog.Attr {
				return xlog.Lazy("key", func() any { *called = true; return "computed value" })
			},
			wantCalled: true, wantContains: "computed value",
		},
		{
			name: "Lazy_disabled", lazyType: "Lazy", level: xlog.LevelError,
			lazyAttr: func(called *bool) slog.Attr {
				return xlog.Lazy("key", func() any { *called = true; return "computed value" })
			},
			wantCalled: false, wantContains: "",
		},
		// LazyString 测试
		{
			name: "LazyString_enabled", lazyType: "LazyString", level: xlog.LevelDebug,
			lazyAttr: func(called *bool) slog.Attr {
				return xlog.LazyString("msg", func() string { *called = true; return "lazy string" })
			},
			wantCalled: true, wantContains: "lazy string",
		},
		{
			name: "LazyString_disabled", lazyType: "LazyString", level: xlog.LevelError,
			lazyAttr: func(called *bool) slog.Attr {
				return xlog.LazyString("msg", func() string { *called = true; return "lazy string" })
			},
			wantCalled: false, wantContains: "",
		},
		// LazyInt 测试
		{
			name: "LazyInt_enabled", lazyType: "LazyInt", level: xlog.LevelDebug,
			lazyAttr: func(called *bool) slog.Attr {
				return xlog.LazyInt("count", func() int64 { *called = true; return 42 })
			},
			wantCalled: true, wantContains: "42",
		},
		{
			name: "LazyInt_disabled", lazyType: "LazyInt", level: xlog.LevelError,
			lazyAttr: func(called *bool) slog.Attr {
				return xlog.LazyInt("count", func() int64 { *called = true; return 42 })
			},
			wantCalled: false, wantContains: "",
		},
		// LazyError 测试
		{
			name: "LazyError_enabled", lazyType: "LazyError", level: xlog.LevelDebug,
			lazyAttr: func(called *bool) slog.Attr {
				return xlog.LazyError("err", func() error { *called = true; return &testError{"test error"} })
			},
			wantCalled: true, wantContains: "test error",
		},
		{
			name: "LazyError_disabled", lazyType: "LazyError", level: xlog.LevelError,
			lazyAttr: func(called *bool) slog.Attr {
				return xlog.LazyError("err", func() error { *called = true; return &testError{"test error"} })
			},
			wantCalled: false, wantContains: "",
		},
		{
			name: "LazyError_nil", lazyType: "LazyError", level: xlog.LevelDebug,
			lazyAttr: func(called *bool) slog.Attr {
				return xlog.LazyError("err", func() error { *called = true; return nil })
			},
			wantCalled: true, wantContains: "test", // 只验证消息存在（nil error 不输出值）
		},
	}
}

// TestLazy 测试所有 Lazy 系列函数的延迟求值特性
func TestLazy(t *testing.T) {
	for _, tt := range getLazyTestCases() {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger, cleanup, err := xlog.New().
				SetOutput(&buf).SetLevel(tt.level).SetFormat("json").Build()
			if err != nil {
				t.Fatalf("Build() error: %v", err)
			}
			testCleanup(t, cleanup)

			called := false
			attr := tt.lazyAttr(&called)

			logger.Debug(context.Background(), "test", attr)

			if called != tt.wantCalled {
				t.Errorf("%s: called=%v, want %v", tt.name, called, tt.wantCalled)
			}
			if tt.wantContains != "" && !strings.Contains(buf.String(), tt.wantContains) {
				t.Errorf("%s: output missing %q\noutput: %s", tt.name, tt.wantContains, buf.String())
			}
		})
	}
}

// =============================================================================
// 性能测试
// =============================================================================

func BenchmarkLazy(b *testing.B) {
	cases := []struct {
		name  string
		level xlog.Level
	}{
		{"enabled", xlog.LevelDebug},
		{"disabled", xlog.LevelError},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			// 使用 io.Discard 避免 I/O 污染基准测试结果
			logger, cleanup, err := xlog.New().
				SetOutput(io.Discard).
				SetLevel(tc.level).
				Build()
			if err != nil {
				b.Fatal(err)
			}
			b.Cleanup(func() {
				if err := cleanup(); err != nil {
					b.Errorf("cleanup error: %v", err)
				}
			})

			ctx := context.Background()
			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				logger.Debug(ctx, "test", xlog.Lazy("key", func() any { return "value" }))
			}
		})
	}
}

// BenchmarkWithoutLazy_Disabled 对比：不使用 Lazy 时，即使 level disabled 也会求值
//
// 这是一个公平的对比基准测试，展示：
//   - 使用 Lazy：当日志级别禁用时，expensive 计算不会执行
//   - 不使用 Lazy：即使日志级别禁用，expensive 计算仍会在参数传递时执行
func BenchmarkWithoutLazy_Disabled(b *testing.B) {
	// 使用 io.Discard 避免 I/O 污染基准测试结果
	logger, cleanup, err := xlog.New().
		SetOutput(io.Discard).
		SetLevel(xlog.LevelError).
		Build()
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		if err := cleanup(); err != nil {
			b.Errorf("cleanup error: %v", err)
		}
	})

	ctx := context.Background()
	expensive := func() string { return "expensive computation result" }
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		// 不使用 Lazy：expensive() 在参数传递时已经被求值
		// 即使日志级别禁用，计算开销也已经发生
		logger.Debug(ctx, "test", slog.String("key", expensive()))
	}
}

// =============================================================================
// 新增 Lazy 函数测试
// =============================================================================

// TestLazyErr 测试 LazyErr 函数（使用标准 "error" key）
func TestLazyErr(t *testing.T) {
	tests := []struct {
		name         string
		level        xlog.Level
		err          error
		wantCalled   bool
		wantContains string
	}{
		{
			name:         "enabled_with_error",
			level:        xlog.LevelDebug,
			err:          &testError{"lazy error"},
			wantCalled:   true,
			wantContains: "lazy error",
		},
		{
			name:       "disabled",
			level:      xlog.LevelError,
			err:        &testError{"lazy error"},
			wantCalled: false,
		},
		{
			name:       "enabled_nil_error",
			level:      xlog.LevelDebug,
			err:        nil,
			wantCalled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger, cleanup, err := xlog.New().
				SetOutput(&buf).
				SetLevel(tt.level).
				SetFormat("json").
				Build()
			if err != nil {
				t.Fatalf("Build() error: %v", err)
			}
			testCleanup(t, cleanup)

			called := false
			testErr := tt.err
			attr := xlog.LazyErr(func() error {
				called = true
				return testErr
			})

			logger.Debug(context.Background(), "test", attr)

			if called != tt.wantCalled {
				t.Errorf("LazyErr: called=%v, want %v", called, tt.wantCalled)
			}
			if tt.wantContains != "" && !strings.Contains(buf.String(), tt.wantContains) {
				t.Errorf("LazyErr: output missing %q\noutput: %s", tt.wantContains, buf.String())
			}
		})
	}
}

// TestLazyDuration 测试 LazyDuration 函数
func TestLazyDuration(t *testing.T) {
	tests := []struct {
		name         string
		level        xlog.Level
		duration     time.Duration
		wantCalled   bool
		wantContains string
	}{
		{
			name:         "enabled",
			level:        xlog.LevelDebug,
			duration:     5 * time.Second,
			wantCalled:   true,
			wantContains: "5s",
		},
		{
			name:       "disabled",
			level:      xlog.LevelError,
			duration:   5 * time.Second,
			wantCalled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger, cleanup, err := xlog.New().
				SetOutput(&buf).
				SetLevel(tt.level).
				SetFormat("json").
				Build()
			if err != nil {
				t.Fatalf("Build() error: %v", err)
			}
			testCleanup(t, cleanup)

			called := false
			d := tt.duration
			attr := xlog.LazyDuration("elapsed", func() time.Duration {
				called = true
				return d
			})

			logger.Debug(context.Background(), "test", attr)

			if called != tt.wantCalled {
				t.Errorf("LazyDuration: called=%v, want %v", called, tt.wantCalled)
			}
			if tt.wantContains != "" && !strings.Contains(buf.String(), tt.wantContains) {
				t.Errorf("LazyDuration: output missing %q\noutput: %s", tt.wantContains, buf.String())
			}
		})
	}
}

// TestLazyGroup 测试 LazyGroup 函数
func TestLazyGroup(t *testing.T) {
	tests := []struct {
		name       string
		level      xlog.Level
		wantCalled bool
	}{
		{
			name:       "enabled",
			level:      xlog.LevelDebug,
			wantCalled: true,
		},
		{
			name:       "disabled",
			level:      xlog.LevelError,
			wantCalled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger, cleanup, err := xlog.New().
				SetOutput(&buf).
				SetLevel(tt.level).
				SetFormat("json").
				Build()
			if err != nil {
				t.Fatalf("Build() error: %v", err)
			}
			testCleanup(t, cleanup)

			called := false
			attr := xlog.LazyGroup("metrics", func() []slog.Attr {
				called = true
				return []slog.Attr{
					slog.Int64("count", 42),
					slog.String("status", "ok"),
				}
			})

			logger.Debug(context.Background(), "test", attr)

			if called != tt.wantCalled {
				t.Errorf("LazyGroup: called=%v, want %v", called, tt.wantCalled)
			}
			if tt.wantCalled {
				// 验证分组内容
				output := buf.String()
				if !strings.Contains(output, "42") || !strings.Contains(output, "ok") {
					t.Errorf("LazyGroup: output missing group content\noutput: %s", output)
				}
			}
		})
	}
}

// BenchmarkLazyDuration 测试 LazyDuration 性能
func BenchmarkLazyDuration(b *testing.B) {
	logger, cleanup, err := xlog.New().
		SetOutput(io.Discard).
		SetLevel(xlog.LevelDebug).
		Build()
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = cleanup() })

	ctx := context.Background()
	start := time.Now()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		logger.Debug(ctx, "test", xlog.LazyDuration("elapsed", func() time.Duration {
			return time.Since(start)
		}))
	}
}

// TestLazy_NilFn 测试 nil 回调不会 panic（输出安全降级值）
func TestLazy_NilFn(t *testing.T) {
	var buf bytes.Buffer
	logger, cleanup, err := xlog.New().
		SetOutput(&buf).
		SetLevel(xlog.LevelDebug).
		SetFormat("json").
		Build()
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	defer func() { _ = cleanup() }()

	ctx := context.Background()

	// 所有 nil fn 都不应 panic
	logger.Debug(ctx, "nil test",
		xlog.Lazy("any", nil),
		xlog.LazyString("str", nil),
		xlog.LazyInt("num", nil),
		xlog.LazyError("err", nil),
		xlog.LazyErr(nil),
		xlog.LazyDuration("dur", nil),
		xlog.LazyGroup("grp", nil),
	)

	output := buf.String()
	if !strings.Contains(output, "nil test") {
		t.Errorf("expected output to contain 'nil test', got: %s", output)
	}
	// 确认没有 "LogValue panicked" 出现
	if strings.Contains(output, "panicked") {
		t.Errorf("nil fn should not cause panic, got: %s", output)
	}
}

// BenchmarkLazyGroup 测试 LazyGroup 性能
func BenchmarkLazyGroup(b *testing.B) {
	logger, cleanup, err := xlog.New().
		SetOutput(io.Discard).
		SetLevel(xlog.LevelDebug).
		Build()
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = cleanup() })

	ctx := context.Background()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		logger.Debug(ctx, "test", xlog.LazyGroup("metrics", func() []slog.Attr {
			return []slog.Attr{
				slog.Int64("count", 42),
				slog.String("status", "ok"),
			}
		}))
	}
}
