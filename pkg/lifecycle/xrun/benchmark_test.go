package xrun

import (
	"context"
	"strconv"
	"testing"
	"time"
)

func BenchmarkNewGroup(b *testing.B) {
	ctx := context.Background()
	b.ResetTimer()
	for b.Loop() {
		g, _ := NewGroup(ctx)
		_ = g
	}
}

func BenchmarkNewGroupWithOptions(b *testing.B) {
	ctx := context.Background()
	opts := []Option{WithName("bench-group")}
	b.ResetTimer()
	for b.Loop() {
		g, _ := NewGroup(ctx, opts...)
		_ = g
	}
}

func BenchmarkGroup_Go(b *testing.B) {
	ctx := context.Background()
	g, _ := NewGroup(ctx)
	fn := func(ctx context.Context) error {
		return nil
	}
	b.ResetTimer()
	for b.Loop() {
		g.Go(fn)
	}
	if err := g.Wait(); err != nil {
		b.Errorf("unexpected error: %v", err)
	}
}

func BenchmarkGroup_GoWithName(b *testing.B) {
	ctx := context.Background()
	g, _ := NewGroup(ctx)
	fn := func(ctx context.Context) error {
		return nil
	}
	b.ResetTimer()
	for b.Loop() {
		g.GoWithName("bench-service", fn)
	}
	if err := g.Wait(); err != nil {
		b.Errorf("unexpected error: %v", err)
	}
}

func BenchmarkGroup_Wait(b *testing.B) {
	ctx := context.Background()
	for b.Loop() {
		g, _ := NewGroup(ctx)
		g.Go(func(ctx context.Context) error {
			return nil
		})
		if err := g.Wait(); err != nil {
			b.Errorf("unexpected error: %v", err)
		}
	}
}

func BenchmarkGroup_MultipleServices(b *testing.B) {
	for _, n := range []int{1, 10, 100} {
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			ctx := context.Background()
			for b.Loop() {
				g, _ := NewGroup(ctx)
				for range n {
					g.Go(func(ctx context.Context) error {
						return nil
					})
				}
				if err := g.Wait(); err != nil {
					b.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func BenchmarkTicker(b *testing.B) {
	for b.Loop() {
		ctx, cancel := context.WithCancel(context.Background())
		g, _ := NewGroup(ctx)
		count := 0
		g.Go(Ticker(time.Microsecond, true, func(ctx context.Context) error {
			count++
			if count >= 10 {
				cancel()
			}
			return nil
		}))
		if err := g.Wait(); err != nil {
			b.Errorf("unexpected error: %v", err)
		}
	}
}

func BenchmarkTimer(b *testing.B) {
	for b.Loop() {
		g, _ := NewGroup(context.Background())
		g.Go(Timer(time.Nanosecond, func(ctx context.Context) error {
			return nil
		}))
		if err := g.Wait(); err != nil {
			b.Errorf("unexpected error: %v", err)
		}
	}
}

func BenchmarkServiceFunc(b *testing.B) {
	ctx := context.Background()
	for b.Loop() {
		svc := ServiceFunc(func(ctx context.Context) error {
			return nil
		})
		if err := svc.Run(ctx); err != nil {
			b.Errorf("unexpected error: %v", err)
		}
	}
}

func BenchmarkGroup_Cancel(b *testing.B) {
	for b.Loop() {
		g, _ := NewGroup(context.Background())
		g.Go(func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		})
		g.Cancel(nil)
		_ = g.Wait() // context.Canceled is filtered
	}
}

func BenchmarkHTTPServer_Shutdown(b *testing.B) {
	for b.Loop() {
		ctx, cancel := context.WithCancel(context.Background())
		server := newMockHTTPServer()

		g, _ := NewGroup(ctx)
		g.Go(HTTPServer(server, time.Second))

		// 立即取消
		cancel()

		if err := g.Wait(); err != nil {
			b.Errorf("unexpected error: %v", err)
		}
	}
}

func BenchmarkWaitForDone(b *testing.B) {
	for b.Loop() {
		ctx, cancel := context.WithCancel(context.Background())
		g, _ := NewGroup(ctx)
		g.Go(WaitForDone())
		cancel()
		if err := g.Wait(); err != nil {
			b.Errorf("unexpected error: %v", err)
		}
	}
}
