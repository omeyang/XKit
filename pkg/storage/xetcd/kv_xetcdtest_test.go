package xetcd_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/omeyang/xkit/pkg/storage/xetcd"
	"github.com/omeyang/xkit/pkg/testkit/xetcdtest"
)

// newXetcdClient 基于嵌入式 etcd 构造真实的 *xetcd.Client。
// 相比 gomock 版本（kv_mock_test.go），此方式同时验证 Client 包装层
// 与 etcd 真实行为，代价是每用例多 ~200ms 启动时间。
func newXetcdClient(t *testing.T) (*xetcd.Client, func()) {
	t.Helper()
	srv, err := xetcdtest.New()
	if err != nil {
		t.Fatalf("xetcdtest.New: %v", err)
	}
	c, err := xetcd.NewClient(&xetcd.Config{
		Endpoints:   srv.Endpoints(),
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		srv.Close()
		t.Fatalf("xetcd.NewClient: %v", err)
	}
	return c, func() {
		if err := c.Close(context.Background()); err != nil {
			t.Logf("xetcd.Client.Close: %v", err)
		}
		srv.Close()
	}
}

func TestKV_Integration_PutGet(t *testing.T) {
	c, cleanup := newXetcdClient(t)
	defer cleanup()

	ctx := context.Background()
	if err := c.Put(ctx, "k", []byte("v")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, err := c.Get(ctx, "k")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != "v" {
		t.Errorf("Get = %q, want %q", got, "v")
	}
}

func TestKV_Integration_GetNotFound(t *testing.T) {
	c, cleanup := newXetcdClient(t)
	defer cleanup()

	_, err := c.Get(context.Background(), "missing")
	if !errors.Is(err, xetcd.ErrKeyNotFound) {
		t.Errorf("Get missing: err = %v, want ErrKeyNotFound", err)
	}
}

func TestKV_Integration_Delete(t *testing.T) {
	c, cleanup := newXetcdClient(t)
	defer cleanup()

	ctx := context.Background()
	if err := c.Put(ctx, "d", []byte("x")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := c.Delete(ctx, "d"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := c.Get(ctx, "d"); !errors.Is(err, xetcd.ErrKeyNotFound) {
		t.Errorf("Get after Delete: err = %v, want ErrKeyNotFound", err)
	}
}
