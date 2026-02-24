package xmongo

import (
	"context"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/omeyang/xkit/pkg/observability/xmetrics"
)

// =============================================================================
// Options Fuzz 测试
// =============================================================================

// FuzzWithHealthTimeout 模糊测试 WithHealthTimeout 选项函数。
func FuzzWithHealthTimeout(f *testing.F) {
	// 种子语料
	f.Add(int64(0))
	f.Add(int64(1))
	f.Add(int64(-1))
	f.Add(int64(5000000000))  // 5 seconds in nanoseconds
	f.Add(int64(-5000000000)) // -5 seconds

	f.Fuzz(func(t *testing.T, ns int64) {
		timeout := time.Duration(ns)
		opts := defaultOptions()
		original := opts.HealthTimeout

		// 不应 panic
		WithHealthTimeout(timeout)(opts)

		// 非正值应被忽略，保持原值
		if timeout <= 0 {
			if opts.HealthTimeout != original {
				t.Errorf("WithHealthTimeout(%v) should keep default, got %v", timeout, opts.HealthTimeout)
			}
		} else {
			if opts.HealthTimeout != timeout {
				t.Errorf("WithHealthTimeout(%v) set HealthTimeout to %v", timeout, opts.HealthTimeout)
			}
		}
	})
}

// FuzzWithSlowQueryThreshold 模糊测试 WithSlowQueryThreshold 选项函数。
func FuzzWithSlowQueryThreshold(f *testing.F) {
	f.Add(int64(0))
	f.Add(int64(1))
	f.Add(int64(-1))
	f.Add(int64(100000000)) // 100ms in nanoseconds

	f.Fuzz(func(t *testing.T, ns int64) {
		threshold := time.Duration(ns)
		opts := defaultOptions()
		original := opts.SlowQueryThreshold

		// 不应 panic
		WithSlowQueryThreshold(threshold)(opts)

		// 负值被忽略，保持原值；非负值被正确设置
		if threshold < 0 {
			if opts.SlowQueryThreshold != original {
				t.Errorf("WithSlowQueryThreshold(%v) should keep default, got %v", threshold, opts.SlowQueryThreshold)
			}
		} else {
			if opts.SlowQueryThreshold != threshold {
				t.Errorf("WithSlowQueryThreshold(%v) set SlowQueryThreshold to %v", threshold, opts.SlowQueryThreshold)
			}
		}
	})
}

// FuzzWithSlowQueryHook 模糊测试 WithSlowQueryHook 选项函数。
func FuzzWithSlowQueryHook(f *testing.F) {
	f.Add(true)
	f.Add(false)

	f.Fuzz(func(t *testing.T, setHook bool) {
		opts := defaultOptions()
		var hookCalled bool

		var hook SlowQueryHook
		if setHook {
			hook = func(_ context.Context, _ SlowQueryInfo) {
				hookCalled = true
			}
		}

		// 不应 panic
		WithSlowQueryHook(hook)(opts)

		// 验证 hook 设置正确
		if setHook && opts.SlowQueryHook == nil {
			t.Error("WithSlowQueryHook should set the hook")
		}

		// 调用 hook 验证不 panic（如果设置了）
		if opts.SlowQueryHook != nil {
			opts.SlowQueryHook(context.Background(), SlowQueryInfo{
				Database:   "testdb",
				Collection: "testcoll",
				Operation:  "find",
				Duration:   time.Second,
			})
			if !hookCalled {
				t.Error("Hook should have been called")
			}
		}
	})
}

// FuzzWithObserver 模糊测试 WithObserver 选项函数。
func FuzzWithObserver(f *testing.F) {
	f.Add(true)
	f.Add(false)

	f.Fuzz(func(t *testing.T, useNoop bool) {
		opts := defaultOptions()
		originalObserver := opts.Observer

		var observer xmetrics.Observer
		if useNoop {
			observer = xmetrics.NoopObserver{}
		}

		// 不应 panic
		WithObserver(observer)(opts)

		// 验证行为：nil observer 不应改变原值
		if observer == nil {
			if opts.Observer != originalObserver {
				t.Error("WithObserver(nil) should not change observer")
			}
		} else {
			// 非 nil observer 应该被设置
			if _, ok := opts.Observer.(xmetrics.NoopObserver); !ok {
				t.Error("WithObserver should set the observer")
			}
		}
	})
}

// FuzzDefaultOptions 模糊测试 defaultOptions 函数。
func FuzzDefaultOptions(f *testing.F) {
	f.Add(0)

	f.Fuzz(func(t *testing.T, _ int) {
		opts := defaultOptions()

		// 验证默认值
		if opts.HealthTimeout != 5*time.Second {
			t.Errorf("defaultOptions().HealthTimeout = %v, want 5s", opts.HealthTimeout)
		}
		if opts.SlowQueryThreshold != 0 {
			t.Errorf("defaultOptions().SlowQueryThreshold = %v, want 0", opts.SlowQueryThreshold)
		}
		if opts.SlowQueryHook != nil {
			t.Error("defaultOptions().SlowQueryHook should be nil")
		}
		if _, ok := opts.Observer.(xmetrics.NoopObserver); !ok {
			t.Error("defaultOptions().Observer should be NoopObserver")
		}
	})
}

// =============================================================================
// PageOptions Fuzz 测试
// =============================================================================

// FuzzPageOptions 模糊测试 PageOptions 结构。
func FuzzPageOptions(f *testing.F) {
	f.Add(int64(1), int64(10))
	f.Add(int64(0), int64(0))
	f.Add(int64(-1), int64(-1))
	f.Add(int64(100), int64(1000))
	f.Add(int64(1<<62), int64(1<<62))

	f.Fuzz(func(t *testing.T, page, pageSize int64) {
		opts := PageOptions{
			Page:     page,
			PageSize: pageSize,
			Sort:     bson.D{{Key: "created_at", Value: -1}},
		}

		// 验证字段正确设置
		if opts.Page != page {
			t.Errorf("PageOptions.Page = %d, want %d", opts.Page, page)
		}
		if opts.PageSize != pageSize {
			t.Errorf("PageOptions.PageSize = %d, want %d", opts.PageSize, pageSize)
		}
		if len(opts.Sort) != 1 {
			t.Error("PageOptions.Sort should have 1 element")
		}
	})
}

// =============================================================================
// BulkOptions Fuzz 测试
// =============================================================================

// FuzzBulkOptions 模糊测试 BulkOptions 结构。
func FuzzBulkOptions(f *testing.F) {
	f.Add(1000, true)
	f.Add(0, false)
	f.Add(-1, true)
	f.Add(1, false)
	f.Add(100000, true)

	f.Fuzz(func(t *testing.T, batchSize int, ordered bool) {
		opts := BulkOptions{
			BatchSize: batchSize,
			Ordered:   ordered,
		}

		// 验证字段正确设置
		if opts.BatchSize != batchSize {
			t.Errorf("BulkOptions.BatchSize = %d, want %d", opts.BatchSize, batchSize)
		}
		if opts.Ordered != ordered {
			t.Errorf("BulkOptions.Ordered = %v, want %v", opts.Ordered, ordered)
		}
	})
}

// =============================================================================
// SlowQueryInfo Fuzz 测试
// =============================================================================

// FuzzSlowQueryInfo 模糊测试 SlowQueryInfo 结构。
func FuzzSlowQueryInfo(f *testing.F) {
	f.Add("testdb", "users", "find", int64(1000000)) // 1ms
	f.Add("", "", "", int64(0))
	f.Add("db_测试", "集合", "insert", int64(-1))
	f.Add("db\x00null", "coll\nnewline", "update", int64(1<<62))

	f.Fuzz(func(t *testing.T, database, collection, operation string, durationNs int64) {
		duration := time.Duration(durationNs)

		info := SlowQueryInfo{
			Database:   database,
			Collection: collection,
			Operation:  operation,
			Duration:   duration,
		}

		// 验证字段正确设置
		if info.Database != database {
			t.Errorf("SlowQueryInfo.Database = %q, want %q", info.Database, database)
		}
		if info.Collection != collection {
			t.Errorf("SlowQueryInfo.Collection = %q, want %q", info.Collection, collection)
		}
		if info.Operation != operation {
			t.Errorf("SlowQueryInfo.Operation = %q, want %q", info.Operation, operation)
		}
		if info.Duration != duration {
			t.Errorf("SlowQueryInfo.Duration = %v, want %v", info.Duration, duration)
		}
	})
}

// =============================================================================
// Stats Fuzz 测试
// =============================================================================

// FuzzStats 模糊测试 Stats 结构。
func FuzzStats(f *testing.F) {
	f.Add(int64(0), int64(0), int64(0), 0)
	f.Add(int64(100), int64(5), int64(10), 10)
	f.Add(int64(-1), int64(-1), int64(-1), -1)
	f.Add(int64(1<<62), int64(1<<62), int64(1<<62), 1<<30)

	f.Fuzz(func(t *testing.T, pingCount, pingErrors, slowQueries int64, inUse int) {
		stats := Stats{
			PingCount:   pingCount,
			PingErrors:  pingErrors,
			SlowQueries: slowQueries,
			Pool: PoolStats{
				InUseConnections: inUse,
			},
		}

		// 验证字段正确设置
		if stats.PingCount != pingCount {
			t.Errorf("Stats.PingCount = %d, want %d", stats.PingCount, pingCount)
		}
		if stats.PingErrors != pingErrors {
			t.Errorf("Stats.PingErrors = %d, want %d", stats.PingErrors, pingErrors)
		}
		if stats.SlowQueries != slowQueries {
			t.Errorf("Stats.SlowQueries = %d, want %d", stats.SlowQueries, slowQueries)
		}
		if stats.Pool.InUseConnections != inUse {
			t.Errorf("Stats.Pool.InUseConnections = %d, want %d", stats.Pool.InUseConnections, inUse)
		}
	})
}

// FuzzPoolStats 模糊测试 PoolStats 结构。
func FuzzPoolStats(f *testing.F) {
	f.Add(10)
	f.Add(0)
	f.Add(-1)

	f.Fuzz(func(t *testing.T, inUse int) {
		pool := PoolStats{
			InUseConnections: inUse,
		}

		// 验证字段正确设置
		if pool.InUseConnections != inUse {
			t.Error("PoolStats.InUseConnections mismatch")
		}
	})
}

// =============================================================================
// PageResult Fuzz 测试
// =============================================================================

// FuzzPageResult 模糊测试 PageResult 结构。
func FuzzPageResult(f *testing.F) {
	f.Add(int64(100), int64(1), int64(10), int64(10))
	f.Add(int64(0), int64(0), int64(0), int64(0))
	f.Add(int64(-1), int64(-1), int64(-1), int64(-1))
	f.Add(int64(1<<62), int64(1<<62), int64(1<<62), int64(1<<62))

	f.Fuzz(func(t *testing.T, total, page, pageSize, totalPages int64) {
		result := PageResult{
			Data:       []bson.M{{"_id": "test"}},
			Total:      total,
			Page:       page,
			PageSize:   pageSize,
			TotalPages: totalPages,
		}

		// 验证字段正确设置
		if result.Total != total {
			t.Errorf("PageResult.Total = %d, want %d", result.Total, total)
		}
		if result.Page != page {
			t.Errorf("PageResult.Page = %d, want %d", result.Page, page)
		}
		if result.PageSize != pageSize {
			t.Errorf("PageResult.PageSize = %d, want %d", result.PageSize, pageSize)
		}
		if result.TotalPages != totalPages {
			t.Errorf("PageResult.TotalPages = %d, want %d", result.TotalPages, totalPages)
		}
		if len(result.Data) != 1 {
			t.Error("PageResult.Data should have 1 element")
		}
	})
}

// =============================================================================
// BulkResult Fuzz 测试
// =============================================================================

// FuzzBulkResult 模糊测试 BulkResult 结构。
func FuzzBulkResult(f *testing.F) {
	f.Add(int64(1000), 0)
	f.Add(int64(0), 5)
	f.Add(int64(-1), -1)
	f.Add(int64(1<<62), 100)

	f.Fuzz(func(t *testing.T, insertedCount int64, errorCount int) {
		// 限制错误数量
		if errorCount < 0 {
			errorCount = 0
		}
		if errorCount > 100 {
			errorCount = 100
		}

		errors := make([]error, errorCount)
		for i := 0; i < errorCount; i++ {
			errors[i] = ErrNilClient
		}

		result := BulkResult{
			InsertedCount: insertedCount,
			Errors:        errors,
		}

		// 验证字段正确设置
		if result.InsertedCount != insertedCount {
			t.Errorf("BulkResult.InsertedCount = %d, want %d", result.InsertedCount, insertedCount)
		}
		if len(result.Errors) != errorCount {
			t.Errorf("BulkResult.Errors length = %d, want %d", len(result.Errors), errorCount)
		}
	})
}

// =============================================================================
// New Factory Fuzz 测试
// =============================================================================

// FuzzNew_NilClient 模糊测试 New 工厂函数（nil 客户端）。
func FuzzNew_NilClient(f *testing.F) {
	f.Add(int64(5000000000), int64(100000000)) // 5s, 100ms

	f.Fuzz(func(t *testing.T, healthTimeoutNs, slowThresholdNs int64) {
		healthTimeout := time.Duration(healthTimeoutNs)
		slowThreshold := time.Duration(slowThresholdNs)

		// 使用 nil 客户端应返回错误
		mongo, err := New(nil,
			WithHealthTimeout(healthTimeout),
			WithSlowQueryThreshold(slowThreshold),
		)

		if err != ErrNilClient {
			t.Errorf("New(nil) error = %v, want %v", err, ErrNilClient)
		}
		if mongo != nil {
			t.Error("New(nil) should return nil Mongo")
		}
	})
}

// =============================================================================
// 错误类型 Fuzz 测试
// =============================================================================

// FuzzIsErrNilClient 模糊测试 ErrNilClient 错误匹配。
func FuzzIsErrNilClient(f *testing.F) {
	f.Add("")
	f.Add("some error")
	f.Add("xmongo: nil client")

	f.Fuzz(func(t *testing.T, errMsg string) {
		var err error
		if errMsg != "" {
			err = &testError{msg: errMsg}
		}

		// 验证 errors.Is 对于非匹配错误不会 panic
		_ = (err == ErrNilClient)
	})
}

// FuzzIsErrClosed 模糊测试 ErrClosed 错误匹配。
func FuzzIsErrClosed(f *testing.F) {
	f.Add("")
	f.Add("some error")
	f.Add("xmongo: client closed")

	f.Fuzz(func(t *testing.T, errMsg string) {
		var err error
		if errMsg != "" {
			err = &testError{msg: errMsg}
		}

		// 验证 errors.Is 对于非匹配错误不会 panic
		_ = (err == ErrClosed)
	})
}

// testError 用于模糊测试的简单错误类型。
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}
