package xplatform_test

import (
	"runtime"
	"testing"

	"github.com/omeyang/xkit/pkg/context/xplatform"
)

// sink 变量消费 benchmark 返回值，防止编译器死代码消除（DCE）导致结果失真。
var (
	sinkString string
	sinkBool   bool
)

// =============================================================================
// Config 方法 Benchmark
// =============================================================================

func BenchmarkConfig_Validate_Valid(b *testing.B) {
	cfg := xplatform.Config{
		PlatformID:      "platform-001",
		HasParent:       true,
		UnclassRegionID: "region-001",
	}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := cfg.Validate(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkConfig_Validate_Invalid(b *testing.B) {
	cfg := xplatform.Config{
		PlatformID: "",
	}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := cfg.Validate(); err == nil {
			b.Fatal("expected validation error")
		}
	}
}

// =============================================================================
// 全局访问函数 Benchmark
// =============================================================================

func BenchmarkPlatformID(b *testing.B) {
	xplatform.Reset()
	b.Cleanup(xplatform.Reset)
	if err := xplatform.Init(xplatform.Config{
		PlatformID: "platform-benchmark",
	}); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		sinkString = xplatform.PlatformID()
	}
}

func BenchmarkHasParent(b *testing.B) {
	xplatform.Reset()
	b.Cleanup(xplatform.Reset)
	if err := xplatform.Init(xplatform.Config{
		PlatformID: "platform-benchmark",
		HasParent:  true,
	}); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		sinkBool = xplatform.HasParent()
	}
}

func BenchmarkUnclassRegionID(b *testing.B) {
	xplatform.Reset()
	b.Cleanup(xplatform.Reset)
	if err := xplatform.Init(xplatform.Config{
		PlatformID:      "platform-benchmark",
		UnclassRegionID: "region-benchmark",
	}); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		sinkString = xplatform.UnclassRegionID()
	}
}

func BenchmarkIsInitialized(b *testing.B) {
	xplatform.Reset()
	b.Cleanup(xplatform.Reset)
	if err := xplatform.Init(xplatform.Config{
		PlatformID: "platform-benchmark",
	}); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		sinkBool = xplatform.IsInitialized()
	}
}

func BenchmarkRequirePlatformID(b *testing.B) {
	xplatform.Reset()
	b.Cleanup(xplatform.Reset)
	if err := xplatform.Init(xplatform.Config{
		PlatformID: "platform-benchmark",
	}); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := xplatform.RequirePlatformID(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGetConfig(b *testing.B) {
	xplatform.Reset()
	b.Cleanup(xplatform.Reset)
	if err := xplatform.Init(xplatform.Config{
		PlatformID:      "platform-benchmark",
		HasParent:       true,
		UnclassRegionID: "region-benchmark",
	}); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := xplatform.GetConfig(); err != nil {
			b.Fatal(err)
		}
	}
}

// =============================================================================
// 并发访问 Benchmark
//
// 使用 runtime.KeepAlive 消费 goroutine 本地结果，避免并发写入共享 sink 变量
// 导致 go test -race 报告 DATA RACE。RunParallel 的多个 goroutine 共享
// 包级变量写入不安全，runtime.KeepAlive 既能防止 DCE 又无竞态风险。
// =============================================================================

func BenchmarkPlatformID_Parallel(b *testing.B) {
	xplatform.Reset()
	b.Cleanup(xplatform.Reset)
	if err := xplatform.Init(xplatform.Config{
		PlatformID: "platform-parallel",
	}); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		var local string
		for pb.Next() {
			local = xplatform.PlatformID()
		}
		runtime.KeepAlive(local)
	})
}

func BenchmarkHasParent_Parallel(b *testing.B) {
	xplatform.Reset()
	b.Cleanup(xplatform.Reset)
	if err := xplatform.Init(xplatform.Config{
		PlatformID: "platform-parallel",
		HasParent:  true,
	}); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		var local bool
		for pb.Next() {
			local = xplatform.HasParent()
		}
		runtime.KeepAlive(local)
	})
}

func BenchmarkIsInitialized_Parallel(b *testing.B) {
	xplatform.Reset()
	b.Cleanup(xplatform.Reset)
	if err := xplatform.Init(xplatform.Config{
		PlatformID: "platform-parallel",
	}); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		var local bool
		for pb.Next() {
			local = xplatform.IsInitialized()
		}
		runtime.KeepAlive(local)
	})
}

func BenchmarkRequirePlatformID_Parallel(b *testing.B) {
	xplatform.Reset()
	b.Cleanup(xplatform.Reset)
	if err := xplatform.Init(xplatform.Config{
		PlatformID: "platform-parallel",
	}); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		var local string
		for pb.Next() {
			v, err := xplatform.RequirePlatformID()
			if err != nil {
				b.Fatal(err)
			}
			local = v
		}
		runtime.KeepAlive(local)
	})
}

func BenchmarkUnclassRegionID_Parallel(b *testing.B) {
	xplatform.Reset()
	b.Cleanup(xplatform.Reset)
	if err := xplatform.Init(xplatform.Config{
		PlatformID:      "platform-parallel",
		UnclassRegionID: "region-parallel",
	}); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		var local string
		for pb.Next() {
			local = xplatform.UnclassRegionID()
		}
		runtime.KeepAlive(local)
	})
}

func BenchmarkGetConfig_Parallel(b *testing.B) {
	xplatform.Reset()
	b.Cleanup(xplatform.Reset)
	if err := xplatform.Init(xplatform.Config{
		PlatformID:      "platform-parallel",
		HasParent:       true,
		UnclassRegionID: "region-parallel",
	}); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := xplatform.GetConfig(); err != nil {
				b.Fatal(err)
			}
		}
	})
}
