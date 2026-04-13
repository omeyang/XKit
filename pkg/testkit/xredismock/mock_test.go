package xredismock

import (
	"context"
	"strings"
	"testing"
)

func TestNew_PingOK(t *testing.T) {
	t.Parallel()
	m, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer m.Close()

	if got, err := m.Client().Ping(context.Background()).Result(); err != nil || got != "PONG" {
		t.Fatalf("ping = %q err=%v", got, err)
	}
}

func TestNew_AddrNonEmpty(t *testing.T) {
	t.Parallel()
	m, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer m.Close()

	addr := m.Addr()
	if !strings.Contains(addr, ":") {
		t.Errorf("Addr %q should be host:port", addr)
	}
}

func TestNew_ServerAccessible(t *testing.T) {
	t.Parallel()
	m, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer m.Close()

	if m.Server() == nil {
		t.Fatal("Server should be non-nil")
	}
	// 借由底层 miniredis 设置键，再通过 client 读回，验证是同一个实例。
	if err := m.Server().Set("k", "v"); err != nil {
		t.Fatalf("miniredis set: %v", err)
	}
	got, err := m.Client().Get(context.Background(), "k").Result()
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got != "v" {
		t.Errorf("want v, got %q", got)
	}
}

func TestNewClient_CleanupCloses(t *testing.T) {
	t.Parallel()
	cli, cleanup, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if cli == nil {
		t.Fatal("client nil")
	}
	if cleanup == nil {
		t.Fatal("cleanup nil")
	}
	cleanup()

	// cleanup 后 ping 应失败。
	if _, err := cli.Ping(context.Background()).Result(); err == nil {
		t.Error("ping after cleanup should fail")
	}
}

func TestClose_IsIdempotent(t *testing.T) {
	t.Parallel()
	m, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	m.Close()
	m.Close() // 第二次 Close 不应 panic
}

func BenchmarkNew(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		m, err := New()
		if err != nil {
			b.Fatal(err)
		}
		m.Close()
	}
}

func FuzzClientSetGet(f *testing.F) {
	for _, seed := range []string{"", "k", "中文", "\x00\xff", strings.Repeat("x", 128)} {
		f.Add(seed, "v")
	}
	m, err := New()
	if err != nil {
		f.Skipf("miniredis init: %v", err)
	}
	defer m.Close()

	ctx := context.Background()
	f.Fuzz(func(t *testing.T, key, val string) {
		if key == "" {
			return // Redis 不允许空 key
		}
		if err := m.Client().Set(ctx, key, val, 0).Err(); err != nil {
			t.Fatalf("set: %v", err)
		}
		got, err := m.Client().Get(ctx, key).Result()
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if got != val {
			t.Errorf("key=%q got=%q want=%q", key, got, val)
		}
	})
}
