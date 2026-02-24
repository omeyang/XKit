package xlog_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/omeyang/xkit/pkg/context/xctx"
	"github.com/omeyang/xkit/pkg/observability/xlog"
)

// enrichTestCase 定义 EnrichHandler 测试用例
type enrichTestCase struct {
	name       string
	setupCtx   func(context.Context) context.Context
	wantKeys   []string // 期望输出包含的 key
	wantValues []string // 期望输出包含的 value
	notWant    []string // 期望输出不包含的内容
}

func TestEnrichHandler(t *testing.T) {
	tests := []enrichTestCase{
		{
			name: "with_trace_info",
			setupCtx: func(ctx context.Context) context.Context {
				ctx, _ = xctx.WithTraceID(ctx, "trace-123")
				ctx, _ = xctx.WithSpanID(ctx, "span-456")
				return ctx
			},
			wantKeys:   []string{"trace_id", "span_id"},
			wantValues: []string{"trace-123", "span-456"},
		},
		{
			name: "with_identity_info",
			setupCtx: func(ctx context.Context) context.Context {
				ctx, _ = xctx.WithPlatformID(ctx, "platform-abc")
				ctx, _ = xctx.WithTenantID(ctx, "tenant-xyz")
				return ctx
			},
			wantKeys:   []string{"platform_id", "tenant_id"},
			wantValues: []string{"platform-abc", "tenant-xyz"},
		},
		{
			name: "with_both_trace_and_identity",
			setupCtx: func(ctx context.Context) context.Context {
				ctx, _ = xctx.WithTraceID(ctx, "trace-999")
				ctx, _ = xctx.WithTenantID(ctx, "tenant-888")
				return ctx
			},
			wantValues: []string{"trace-999", "tenant-888"},
		},
		{
			name: "empty_context",
			setupCtx: func(ctx context.Context) context.Context {
				return ctx // 不添加任何信息
			},
			wantValues: []string{"test message"},
			notWant:    []string{"trace_id", "tenant_id"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			base := slog.NewJSONHandler(&buf, nil)
			handler, err := xlog.NewEnrichHandler(base)
			if err != nil {
				t.Fatalf("NewEnrichHandler() error: %v", err)
			}
			logger := slog.New(handler)

			ctx := tt.setupCtx(context.Background())
			logger.InfoContext(ctx, "test message")

			output := buf.String()

			// 检查期望的 key
			for _, key := range tt.wantKeys {
				if !strings.Contains(output, key) {
					t.Errorf("output missing key %q\noutput: %s", key, output)
				}
			}

			// 检查期望的 value
			for _, val := range tt.wantValues {
				if !strings.Contains(output, val) {
					t.Errorf("output missing value %q\noutput: %s", val, output)
				}
			}

			// 检查不期望的内容
			for _, notWant := range tt.notWant {
				if strings.Contains(output, notWant) {
					t.Errorf("output should not contain %q\noutput: %s", notWant, output)
				}
			}
		})
	}
}

func TestEnrichHandler_WithAttrs(t *testing.T) {
	var buf bytes.Buffer
	base := slog.NewJSONHandler(&buf, nil)
	handler, err := xlog.NewEnrichHandler(base)
	if err != nil {
		t.Fatalf("NewEnrichHandler() error: %v", err)
	}

	enriched := handler.WithAttrs([]slog.Attr{slog.String("extra", "value")})
	logger := slog.New(enriched)

	ctx, _ := xctx.WithTraceID(context.Background(), "trace-111")
	logger.InfoContext(ctx, "test message")

	output := buf.String()
	for _, want := range []string{"extra", "value", "trace-111"} {
		if !strings.Contains(output, want) {
			t.Errorf("output missing %q\noutput: %s", want, output)
		}
	}
}

func TestEnrichHandler_WithGroup(t *testing.T) {
	var buf bytes.Buffer
	base := slog.NewJSONHandler(&buf, nil)
	handler, err := xlog.NewEnrichHandler(base)
	if err != nil {
		t.Fatalf("NewEnrichHandler() error: %v", err)
	}

	grouped := handler.WithGroup("request")
	logger := slog.New(grouped)

	ctx, _ := xctx.WithTraceID(context.Background(), "trace-222")
	logger.InfoContext(ctx, "test message", slog.String("method", "GET"))

	output := buf.String()
	for _, want := range []string{"trace-222", "request"} {
		if !strings.Contains(output, want) {
			t.Errorf("output missing %q\noutput: %s", want, output)
		}
	}
}

func TestEnrichHandler_Enabled(t *testing.T) {
	base := slog.NewJSONHandler(&bytes.Buffer{}, &slog.HandlerOptions{Level: slog.LevelWarn})
	handler, err := xlog.NewEnrichHandler(base)
	if err != nil {
		t.Fatalf("NewEnrichHandler() error: %v", err)
	}

	ctx := context.Background()
	if handler.Enabled(ctx, slog.LevelInfo) {
		t.Error("Info should not be enabled when base level is Warn")
	}
	if !handler.Enabled(ctx, slog.LevelWarn) {
		t.Error("Warn should be enabled when base level is Warn")
	}
}

func TestNewEnrichHandler_NilBase_Error(t *testing.T) {
	handler, err := xlog.NewEnrichHandler(nil)
	if err == nil {
		t.Fatal("NewEnrichHandler(nil) should return error")
	}
	if handler != nil {
		t.Error("NewEnrichHandler(nil) should return nil handler")
	}
	if !errors.Is(err, xlog.ErrNilHandler) {
		t.Errorf("error should be ErrNilHandler, got: %v", err)
	}
}

// =============================================================================
// 性能测试
// =============================================================================

func BenchmarkEnrichHandler(b *testing.B) {
	cases := []struct {
		name     string
		setupCtx func(context.Context) context.Context
	}{
		{
			name: "with_context",
			setupCtx: func(ctx context.Context) context.Context {
				ctx, _ = xctx.WithTraceID(ctx, "trace-bench")
				ctx, _ = xctx.WithTenantID(ctx, "tenant-bench")
				return ctx
			},
		},
		{
			name:     "empty_context",
			setupCtx: func(ctx context.Context) context.Context { return ctx },
		},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			base := slog.NewJSONHandler(&bytes.Buffer{}, nil)
			handler, err := xlog.NewEnrichHandler(base)
			if err != nil {
				b.Fatalf("NewEnrichHandler() error: %v", err)
			}
			logger := slog.New(handler)

			ctx := tc.setupCtx(context.Background())
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				logger.InfoContext(ctx, "benchmark message")
			}
		})
	}
}
