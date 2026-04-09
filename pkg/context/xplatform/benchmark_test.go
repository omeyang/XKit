package xplatform_test

import (
	"runtime"
	"testing"

	"github.com/omeyang/xkit/pkg/context/xplatform"
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
	b.ReportAllocs()
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
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := cfg.Validate(); err == nil {
			b.Fatal("expected validation error")
		}
	}
}

// =============================================================================
// 全局访问函数 Benchmark
//
// 设计决策: 串行与并行基准统一使用 runtime.KeepAlive 消费返回值防止 DCE，
// 避免包级 sink 变量在未来误改为并行基准时引入 DATA RACE。
// =============================================================================

func BenchmarkPlatformID(b *testing.B) {
	xplatform.Reset()
	b.Cleanup(xplatform.Reset)
	if err := xplatform.Init(xplatform.Config{
		PlatformID: "platform-benchmark",
	}); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()

	var v string
	for i := 0; i < b.N; i++ {
		v = xplatform.PlatformID()
	}
	runtime.KeepAlive(v)
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
	b.ReportAllocs()
	b.ResetTimer()

	var v bool
	for i := 0; i < b.N; i++ {
		v = xplatform.HasParent()
	}
	runtime.KeepAlive(v)
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
	b.ReportAllocs()
	b.ResetTimer()

	var v string
	for i := 0; i < b.N; i++ {
		v = xplatform.UnclassRegionID()
	}
	runtime.KeepAlive(v)
}

func BenchmarkIsInitialized(b *testing.B) {
	xplatform.Reset()
	b.Cleanup(xplatform.Reset)
	if err := xplatform.Init(xplatform.Config{
		PlatformID: "platform-benchmark",
	}); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()

	var v bool
	for i := 0; i < b.N; i++ {
		v = xplatform.IsInitialized()
	}
	runtime.KeepAlive(v)
}

func BenchmarkRequirePlatformID(b *testing.B) {
	xplatform.Reset()
	b.Cleanup(xplatform.Reset)
	if err := xplatform.Init(xplatform.Config{
		PlatformID: "platform-benchmark",
	}); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()

	var v string
	for i := 0; i < b.N; i++ {
		var err error
		v, err = xplatform.RequirePlatformID()
		if err != nil {
			b.Fatal(err)
		}
	}
	runtime.KeepAlive(v)
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
	b.ReportAllocs()
	b.ResetTimer()

	var v xplatform.Config
	for i := 0; i < b.N; i++ {
		var err error
		v, err = xplatform.GetConfig()
		if err != nil {
			b.Fatal(err)
		}
	}
	runtime.KeepAlive(v)
}

// =============================================================================
// 并发访问 Benchmark
// =============================================================================

func BenchmarkPlatformID_Parallel(b *testing.B) {
	xplatform.Reset()
	b.Cleanup(xplatform.Reset)
	if err := xplatform.Init(xplatform.Config{
		PlatformID: "platform-parallel",
	}); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
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
	b.ReportAllocs()
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
	b.ReportAllocs()
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
	b.ReportAllocs()
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
	b.ReportAllocs()
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
	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		var local xplatform.Config
		for pb.Next() {
			v, err := xplatform.GetConfig()
			if err != nil {
				b.Fatal(err)
			}
			local = v
		}
		runtime.KeepAlive(local)
	})
}
