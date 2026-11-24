package xctx_test

import (
	"context"
	"testing"

	"github.com/omeyang/xkit/pkg/context/xctx"
)

var traceFuzzSeeds = [][3]string{
	{"t1", "s1", "r1"},
	{"", "", ""},
	{"trace", "", "request"},
}

var traceFuzzConfig = fuzzThreeFieldsConfig{
	FieldNames: [3]string{"TraceID", "SpanID", "RequestID"},
	Setters:    [3]func(context.Context, string) (context.Context, error){xctx.WithTraceID, xctx.WithSpanID, xctx.WithRequestID},
	GetFields: func(ctx context.Context) [3]string {
		tr := xctx.GetTrace(ctx)
		return [3]string{tr.TraceID, tr.SpanID, tr.RequestID}
	},
}

func FuzzTraceFields(f *testing.F) {
	runThreeFieldsFuzz(f, traceFuzzSeeds, traceFuzzConfig)
}

// FuzzTraceFlags 验证 TraceFlags 的 round-trip 一致性。
// 单独测试是因为 TraceFlags 未包含在 runThreeFieldsFuzz 的三字段配置中。
func FuzzTraceFlags(f *testing.F) {
	f.Add("01")
	f.Add("00")
	f.Add("")
	f.Add("ff")
	f.Fuzz(func(t *testing.T, flags string) {
		if flags == "" {
			return
		}
		ctx, err := xctx.WithTraceFlags(context.Background(), flags)
		if err != nil {
			t.Fatalf("WithTraceFlags() error = %v", err)
		}
		if got := xctx.TraceFlags(ctx); got != flags {
			t.Errorf("TraceFlags() = %q, want %q", got, flags)
		}
	})
}
