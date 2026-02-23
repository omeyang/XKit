package mqcore

import (
	"context"
	"testing"

	"github.com/omeyang/xkit/pkg/context/xctx"
)

func BenchmarkRunConsumeLoop_SuccessPath(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var n int
	consume := func(_ context.Context) error {
		n++
		if n >= b.N {
			cancel()
		}
		return nil
	}

	if err := RunConsumeLoop(ctx, consume); err != nil && err != context.Canceled {
		b.Fatal(err)
	}
}

func BenchmarkMergeTraceContext(b *testing.B) {
	base := context.Background()
	extracted := context.Background()
	extracted, _ = xctx.WithTraceID(extracted, "0af7651916cd43dd8448eb211c80319c")
	extracted, _ = xctx.WithSpanID(extracted, "b7ad6b7169203331")
	extracted, _ = xctx.WithRequestID(extracted, "req-12345")
	extracted, _ = xctx.WithTraceFlags(extracted, "01")

	b.ResetTimer()
	for b.Loop() {
		MergeTraceContext(base, extracted)
	}
}

func BenchmarkEnsureSpanContext_FromXctx(b *testing.B) {
	ctx := context.Background()
	ctx, _ = xctx.WithTraceID(ctx, "0af7651916cd43dd8448eb211c80319c")
	ctx, _ = xctx.WithSpanID(ctx, "b7ad6b7169203331")

	b.ResetTimer()
	for b.Loop() {
		ensureSpanContext(ctx)
	}
}
