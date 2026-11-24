package xelection

import (
	"context"
	"errors"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// FuzzNewEtcdElection_Prefix 模糊测试 prefix 输入：任意字节串下不 panic，
// 空字符串返回 ErrEmptyPrefix，非空串返回非 nil Election。
func FuzzNewEtcdElection_Prefix(f *testing.F) {
	for _, seed := range []string{"", "/", "/p/", "a", "\x00\x01", " ", "//", "大写"} {
		f.Add(seed)
	}
	client := &clientv3.Client{}
	f.Fuzz(func(t *testing.T, prefix string) {
		e, err := NewEtcdElection(client, prefix)
		if prefix == "" {
			if !errors.Is(err, ErrEmptyPrefix) {
				t.Fatalf("empty prefix want ErrEmptyPrefix, got %v", err)
			}
			return
		}
		if err != nil {
			t.Fatalf("non-empty prefix %q should succeed, got %v", prefix, err)
		}
		if e == nil {
			t.Fatal("election should not be nil")
		}
		_ = e.Close(context.Background())
	})
}

// FuzzCampaign_CandidateID 模糊测试 candidateID：任意输入下 Campaign 参数校验不 panic，
// 空串返回 ErrEmptyCandidateID，非空串进入下一阶段（这里会因 sessionFactory 返回
// 非 *concurrency.Session 而被 defensive 分支拒绝，属于预期路径）。
func FuzzCampaign_CandidateID(f *testing.F) {
	for _, seed := range []string{"", "node-1", "\x00", "中文", " "} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, candidateID string) {
		e := NewTestElection("/p/", func() (sessionProvider, error) {
			return NewMockSession(), nil
		})
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		_, err := e.Campaign(ctx, candidateID)
		if candidateID == "" {
			if !errors.Is(err, ErrEmptyCandidateID) {
				t.Fatalf("empty id want ErrEmptyCandidateID, got %v", err)
			}
			return
		}
		// 非空 id：应进入 sessionFactory 路径并被 defensive 分支拒绝（非 etcd session）。
		if err == nil {
			t.Fatal("want defensive rejection for mock session, got nil")
		}
	})
}

// FuzzWithTTL 任意整数输入下 WithTTL 不 panic，正数被采纳，非正数保留默认。
func FuzzWithTTL(f *testing.F) {
	for _, seed := range []int{-1, 0, 1, 15, 1_000_000} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, n int) {
		o := defaultOptions()
		WithTTL(n)(o)
		if n > 0 && o.ttlSeconds != n {
			t.Fatalf("positive ttl %d not applied (got %d)", n, o.ttlSeconds)
		}
		if n <= 0 && o.ttlSeconds != defaultTTLSeconds {
			t.Fatalf("non-positive ttl %d should keep default", n)
		}
	})
}
