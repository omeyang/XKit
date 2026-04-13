package xelection

import (
	"context"
	"testing"
)

// BenchmarkLeader_CheckLeader 测量 Fencing 前置检查的热路径开销。
// 目标：纳秒级、零分配。生产中该调用出现在每次 etcd 写操作之前。
func BenchmarkLeader_CheckLeader(b *testing.B) {
	ms := NewMockSession()
	l := NewTestLeader(ms, "cand-1", nil)
	defer func() { _ = l.Resign(context.Background()) }()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := l.CheckLeader(); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkLeader_IsLeader 与 CheckLeader 等价路径（无错误分配）。
func BenchmarkLeader_IsLeader(b *testing.B) {
	ms := NewMockSession()
	l := NewTestLeader(ms, "cand-1", nil)
	defer func() { _ = l.Resign(context.Background()) }()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = l.IsLeader()
	}
}

// BenchmarkLeader_Lost 读取 lost channel 指针（应为零分配）。
func BenchmarkLeader_Lost(b *testing.B) {
	ms := NewMockSession()
	l := NewTestLeader(ms, "cand-1", nil)
	defer func() { _ = l.Resign(context.Background()) }()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = l.Lost()
	}
}

// BenchmarkLeader_CheckLeader_AfterLoss 失效状态下的热路径（短路返回错误）。
func BenchmarkLeader_CheckLeader_AfterLoss(b *testing.B) {
	ms := NewMockSession()
	l := NewTestLeader(ms, "cand-1", nil)
	l.TriggerLose("benchmark")
	defer func() { _ = l.Resign(context.Background()) }()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = l.CheckLeader()
	}
}
