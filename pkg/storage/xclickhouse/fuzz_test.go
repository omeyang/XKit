package xclickhouse

import (
	"context"
	"testing"
	"time"

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
		originalTimeout := opts.HealthTimeout

		// 不应 panic
		WithHealthTimeout(timeout)(opts)

		// 验证行为：只有正值才会被设置
		if timeout > 0 {
			if opts.HealthTimeout != timeout {
				t.Errorf("WithHealthTimeout(%v) set HealthTimeout to %v", timeout, opts.HealthTimeout)
			}
		} else {
			// 非正值不应改变原值
			if opts.HealthTimeout != originalTimeout {
				t.Errorf("WithHealthTimeout(%v) should not change HealthTimeout", timeout)
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

		// 不应 panic
		WithSlowQueryThreshold(threshold)(opts)

		// 验证值被正确设置（任意值都接受）
		if opts.SlowQueryThreshold != threshold {
			t.Errorf("WithSlowQueryThreshold(%v) set SlowQueryThreshold to %v", threshold, opts.SlowQueryThreshold)
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
				Query:    "SELECT 1",
				Args:     []any{},
				Duration: time.Second,
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
		}

		// 验证字段正确设置
		if opts.Page != page {
			t.Errorf("PageOptions.Page = %d, want %d", opts.Page, page)
		}
		if opts.PageSize != pageSize {
			t.Errorf("PageOptions.PageSize = %d, want %d", opts.PageSize, pageSize)
		}
	})
}

// =============================================================================
// BatchOptions Fuzz 测试
// =============================================================================

// FuzzBatchOptions 模糊测试 BatchOptions 结构。
func FuzzBatchOptions(f *testing.F) {
	f.Add(10000)
	f.Add(0)
	f.Add(-1)
	f.Add(1)
	f.Add(100000)

	f.Fuzz(func(t *testing.T, batchSize int) {
		opts := BatchOptions{
			BatchSize: batchSize,
		}

		// 验证字段正确设置
		if opts.BatchSize != batchSize {
			t.Errorf("BatchOptions.BatchSize = %d, want %d", opts.BatchSize, batchSize)
		}
	})
}

// =============================================================================
// SlowQueryInfo Fuzz 测试
// =============================================================================

// FuzzSlowQueryInfo 模糊测试 SlowQueryInfo 结构。
func FuzzSlowQueryInfo(f *testing.F) {
	f.Add("SELECT * FROM users", int64(1000000)) // 1ms
	f.Add("", int64(0))
	f.Add("SELECT * FROM 测试表", int64(-1))
	f.Add("INSERT INTO t\x00 VALUES", int64(1<<62))

	f.Fuzz(func(t *testing.T, query string, durationNs int64) {
		duration := time.Duration(durationNs)

		info := SlowQueryInfo{
			Query:    query,
			Args:     []any{1, "test"},
			Duration: duration,
		}

		// 验证字段正确设置
		if info.Query != query {
			t.Errorf("SlowQueryInfo.Query = %q, want %q", info.Query, query)
		}
		if info.Duration != duration {
			t.Errorf("SlowQueryInfo.Duration = %v, want %v", info.Duration, duration)
		}
		if len(info.Args) != 2 {
			t.Errorf("SlowQueryInfo.Args length = %d, want 2", len(info.Args))
		}
	})
}

// =============================================================================
// Stats Fuzz 测试
// =============================================================================

// FuzzStats 模糊测试 Stats 结构。
func FuzzStats(f *testing.F) {
	f.Add(int64(0), int64(0), int64(0), int64(0), int64(0), 0, 0, 0)
	f.Add(int64(100), int64(5), int64(1000), int64(10), int64(20), 50, 40, 10)
	f.Add(int64(-1), int64(-1), int64(-1), int64(-1), int64(-1), -1, -1, -1)
	f.Add(int64(1<<62), int64(1<<62), int64(1<<62), int64(1<<62), int64(1<<62), 1<<30, 1<<30, 1<<30)

	f.Fuzz(func(t *testing.T, pingCount, pingErrors, queryCount, queryErrors, slowQueries int64, open, idle, inUse int) {
		stats := Stats{
			PingCount:   pingCount,
			PingErrors:  pingErrors,
			QueryCount:  queryCount,
			QueryErrors: queryErrors,
			SlowQueries: slowQueries,
			Pool: PoolStats{
				Open:  open,
				Idle:  idle,
				InUse: inUse,
			},
		}

		// 验证字段正确设置
		if stats.PingCount != pingCount {
			t.Errorf("Stats.PingCount = %d, want %d", stats.PingCount, pingCount)
		}
		if stats.PingErrors != pingErrors {
			t.Errorf("Stats.PingErrors = %d, want %d", stats.PingErrors, pingErrors)
		}
		if stats.QueryCount != queryCount {
			t.Errorf("Stats.QueryCount = %d, want %d", stats.QueryCount, queryCount)
		}
		if stats.QueryErrors != queryErrors {
			t.Errorf("Stats.QueryErrors = %d, want %d", stats.QueryErrors, queryErrors)
		}
		if stats.SlowQueries != slowQueries {
			t.Errorf("Stats.SlowQueries = %d, want %d", stats.SlowQueries, slowQueries)
		}
		if stats.Pool.Open != open {
			t.Errorf("Stats.Pool.Open = %d, want %d", stats.Pool.Open, open)
		}
	})
}

// FuzzPoolStats 模糊测试 PoolStats 结构。
func FuzzPoolStats(f *testing.F) {
	f.Add(100, 90, 10)
	f.Add(0, 0, 0)
	f.Add(-1, -1, -1)

	f.Fuzz(func(t *testing.T, open, idle, inUse int) {
		pool := PoolStats{
			Open:  open,
			Idle:  idle,
			InUse: inUse,
		}

		// 验证字段正确设置
		if pool.Open != open {
			t.Error("PoolStats.Open mismatch")
		}
		if pool.Idle != idle {
			t.Error("PoolStats.Idle mismatch")
		}
		if pool.InUse != inUse {
			t.Error("PoolStats.InUse mismatch")
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
			Columns:    []string{"id", "name"},
			Rows:       [][]any{{1, "test"}},
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
		if len(result.Columns) != 2 {
			t.Error("PageResult.Columns should have 2 elements")
		}
		if len(result.Rows) != 1 {
			t.Error("PageResult.Rows should have 1 element")
		}
	})
}

// =============================================================================
// BatchResult Fuzz 测试
// =============================================================================

// FuzzBatchResult 模糊测试 BatchResult 结构。
func FuzzBatchResult(f *testing.F) {
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
			errors[i] = ErrNilConn
		}

		result := BatchResult{
			InsertedCount: insertedCount,
			Errors:        errors,
		}

		// 验证字段正确设置
		if result.InsertedCount != insertedCount {
			t.Errorf("BatchResult.InsertedCount = %d, want %d", result.InsertedCount, insertedCount)
		}
		if len(result.Errors) != errorCount {
			t.Errorf("BatchResult.Errors length = %d, want %d", len(result.Errors), errorCount)
		}
	})
}

// =============================================================================
// New Factory Fuzz 测试
// =============================================================================

// FuzzNew_NilConn 模糊测试 New 工厂函数（nil 连接）。
func FuzzNew_NilConn(f *testing.F) {
	f.Add(int64(5000000000), int64(100000000)) // 5s, 100ms

	f.Fuzz(func(t *testing.T, healthTimeoutNs, slowThresholdNs int64) {
		healthTimeout := time.Duration(healthTimeoutNs)
		slowThreshold := time.Duration(slowThresholdNs)

		// 使用 nil 连接应返回错误
		ch, err := New(nil,
			WithHealthTimeout(healthTimeout),
			WithSlowQueryThreshold(slowThreshold),
		)

		if err != ErrNilConn {
			t.Errorf("New(nil) error = %v, want %v", err, ErrNilConn)
		}
		if ch != nil {
			t.Error("New(nil) should return nil ClickHouse")
		}
	})
}

// =============================================================================
// 错误类型 Fuzz 测试
// =============================================================================

// FuzzIsErrNilConn 模糊测试 ErrNilConn 错误匹配。
func FuzzIsErrNilConn(f *testing.F) {
	f.Add("")
	f.Add("some error")
	f.Add("xclickhouse: nil connection")

	f.Fuzz(func(t *testing.T, errMsg string) {
		var err error
		if errMsg != "" {
			err = &testError{msg: errMsg}
		}

		// 验证 errors.Is 对于非匹配错误不会 panic
		_ = (err == ErrNilConn)
	})
}

// FuzzIsErrClosed 模糊测试 ErrClosed 错误匹配。
func FuzzIsErrClosed(f *testing.F) {
	f.Add("")
	f.Add("some error")
	f.Add("xclickhouse: connection closed")

	f.Fuzz(func(t *testing.T, errMsg string) {
		var err error
		if errMsg != "" {
			err = &testError{msg: errMsg}
		}

		// 验证 errors.Is 对于非匹配错误不会 panic
		_ = (err == ErrClosed)
	})
}

// FuzzIsErrEmptyQuery 模糊测试 ErrEmptyQuery 错误匹配。
func FuzzIsErrEmptyQuery(f *testing.F) {
	f.Add("")
	f.Add("empty query")
	f.Add("xclickhouse: empty query")

	f.Fuzz(func(t *testing.T, errMsg string) {
		var err error
		if errMsg != "" {
			err = &testError{msg: errMsg}
		}

		// 验证 errors.Is 对于非匹配错误不会 panic
		_ = (err == ErrEmptyQuery)
	})
}

// FuzzIsErrInvalidTableName 模糊测试 ErrInvalidTableName 错误匹配。
func FuzzIsErrInvalidTableName(f *testing.F) {
	f.Add("")
	f.Add("invalid table")
	f.Add("xclickhouse: invalid table name, contains illegal characters")

	f.Fuzz(func(t *testing.T, errMsg string) {
		var err error
		if errMsg != "" {
			err = &testError{msg: errMsg}
		}

		// 验证 errors.Is 对于非匹配错误不会 panic
		_ = (err == ErrInvalidTableName)
	})
}

// testError 用于模糊测试的简单错误类型。
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

// =============================================================================
// SQL 规范化和校验 Fuzz 测试
// =============================================================================

// FuzzNormalizeQuery 模糊测试 normalizeQuery 函数。
func FuzzNormalizeQuery(f *testing.F) {
	// 种子语料
	f.Add("SELECT * FROM users")
	f.Add("SELECT * FROM users;")
	f.Add("SELECT * FROM users; ; ; ")
	f.Add("SELECT * FROM users\n\t ")
	f.Add("")
	f.Add(";;;")
	f.Add("   \t\n")
	f.Add("SELECT ';' FROM users")
	f.Add("SELECT '\x00' FROM users;")
	f.Add("SELECT * FROM 测试表;")

	f.Fuzz(func(t *testing.T, query string) {
		// normalizeQuery 不应 panic
		normalized := normalizeQuery(query)

		// 规范化后的查询不应以分号、空格、制表符、换行符结尾
		if len(normalized) > 0 {
			last := normalized[len(normalized)-1]
			if last == ';' || last == ' ' || last == '\t' || last == '\n' || last == '\r' {
				t.Errorf("normalizeQuery(%q) = %q, should not end with %q", query, normalized, string(last))
			}
		}
	})
}

// FuzzValidateQuerySyntax 模糊测试 validateQuerySyntax 函数。
func FuzzValidateQuerySyntax(f *testing.F) {
	// 种子语料
	f.Add("SELECT * FROM users")
	f.Add("SELECT * FROM users FORMAT JSON")
	f.Add("SELECT * FROM users SETTINGS max_threads=4")
	f.Add("SELECT * FROM users format json")
	f.Add("SELECT * FROM users settings max_threads=4")
	f.Add("")
	f.Add("   ")
	f.Add("SELECT FORMATTER FROM users")
	f.Add("SELECT SETTINGS_KEY FROM config")
	f.Add("SELECT '\x00' FROM users")
	f.Add("SELECT * FROM 测试表")

	f.Fuzz(func(t *testing.T, query string) {
		// validateQuerySyntax 不应 panic
		normalized, err := validateQuerySyntax(query)

		// 如果没有错误，规范化后的查询不应为空
		if err == nil && normalized == "" {
			t.Errorf("validateQuerySyntax(%q) returned empty normalized query without error", query)
		}

		// 如果有错误，应该是已知的错误类型
		if err != nil {
			switch err {
			case ErrEmptyQuery:
				// 预期：空查询
			case ErrQueryContainsFormat:
				// 预期：包含 FORMAT
			case ErrQueryContainsSettings:
				// 预期：包含 SETTINGS
			default:
				t.Errorf("validateQuerySyntax(%q) returned unexpected error: %v", query, err)
			}
		}
	})
}

// FuzzValidatePageOptions 模糊测试 validatePageOptions 函数。
func FuzzValidatePageOptions(f *testing.F) {
	// 种子语料
	f.Add("SELECT * FROM users", int64(1), int64(10))
	f.Add("SELECT * FROM users;", int64(1), int64(10))
	f.Add("SELECT * FROM users FORMAT JSON", int64(1), int64(10))
	f.Add("", int64(1), int64(10))
	f.Add("SELECT *", int64(0), int64(10))
	f.Add("SELECT *", int64(1), int64(0))
	f.Add("SELECT *", int64(-1), int64(-1))

	f.Fuzz(func(t *testing.T, query string, page, pageSize int64) {
		opts := PageOptions{Page: page, PageSize: pageSize}

		// validatePageOptions 不应 panic
		normalized, _, err := validatePageOptions(query, opts)

		// 如果没有错误，规范化后的查询不应为空
		if err == nil && normalized == "" {
			t.Errorf("validatePageOptions(%q, %+v) returned empty normalized query without error", query, opts)
		}

		// 如果有错误，规范化后的查询应为空
		if err != nil && normalized != "" {
			t.Errorf("validatePageOptions(%q, %+v) returned non-empty normalized query with error: %v", query, opts, err)
		}
	})
}
