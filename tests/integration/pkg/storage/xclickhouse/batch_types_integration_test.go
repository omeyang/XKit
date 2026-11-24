//go:build integration

package xclickhouse_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/omeyang/xkit/pkg/storage/xclickhouse"
)

func TestClickHouse_ComplexTypes_Map_Integration(t *testing.T) {
	conn, cleanup := setupClickHouse(t)
	defer cleanup()

	ctx := context.Background()
	tableName := fmt.Sprintf("test_map_%d", time.Now().UnixNano())

	// 创建带 Map 类型的表
	err := conn.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id UInt64,
			attrs Map(String, String)
		) ENGINE = Memory
	`, tableName))
	require.NoError(t, err)

	wrapper, err := xclickhouse.New(conn)
	require.NoError(t, err)

	type mapRow struct {
		ID    uint64            `ch:"id"`
		Attrs map[string]string `ch:"attrs"`
	}

	rows := []any{
		&mapRow{ID: 1, Attrs: map[string]string{"env": "prod", "region": "us-west"}},
		&mapRow{ID: 2, Attrs: map[string]string{"env": "dev"}},
		&mapRow{ID: 3, Attrs: map[string]string{}}, // 空 Map
	}

	result, err := wrapper.BatchInsert(ctx, tableName, rows, xclickhouse.BatchOptions{})
	require.NoError(t, err)
	assert.Equal(t, int64(3), result.InsertedCount)

	// 验证数据
	pageResult, err := wrapper.QueryPage(ctx,
		fmt.Sprintf("SELECT id, attrs FROM %s ORDER BY id", tableName),
		xclickhouse.PageOptions{Page: 1, PageSize: 10},
	)
	require.NoError(t, err)
	assert.Equal(t, int64(3), pageResult.Total)
}

func TestClickHouse_ComplexTypes_Nullable_Integration(t *testing.T) {
	conn, cleanup := setupClickHouse(t)
	defer cleanup()

	ctx := context.Background()
	tableName := fmt.Sprintf("test_nullable_%d", time.Now().UnixNano())

	// 创建带 Nullable 类型的表
	err := conn.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id UInt64,
			name Nullable(String),
			score Nullable(Float64)
		) ENGINE = Memory
	`, tableName))
	require.NoError(t, err)

	wrapper, err := xclickhouse.New(conn)
	require.NoError(t, err)

	type nullableRow struct {
		ID    uint64   `ch:"id"`
		Name  *string  `ch:"name"`
		Score *float64 `ch:"score"`
	}

	name := "Alice"
	score := 95.5

	rows := []any{
		&nullableRow{ID: 1, Name: &name, Score: &score},
		&nullableRow{ID: 2, Name: nil, Score: &score},
		&nullableRow{ID: 3, Name: &name, Score: nil},
		&nullableRow{ID: 4, Name: nil, Score: nil},
	}

	result, err := wrapper.BatchInsert(ctx, tableName, rows, xclickhouse.BatchOptions{})
	require.NoError(t, err)
	assert.Equal(t, int64(4), result.InsertedCount)

	// 查询侧断言：验证 Nullable 类型的读取和 nil 语义
	pageResult, err := wrapper.QueryPage(ctx,
		fmt.Sprintf("SELECT id, name, score FROM %s ORDER BY id", tableName),
		xclickhouse.PageOptions{Page: 1, PageSize: 10},
	)
	require.NoError(t, err)
	assert.Equal(t, int64(4), pageResult.Total)
	require.Len(t, pageResult.Rows, 4)

	// id=1: name="Alice", score=95.5（均非 nil）
	assert.NotNil(t, pageResult.Rows[0][1], "id=1 name 应为非 nil")
	assert.NotNil(t, pageResult.Rows[0][2], "id=1 score 应为非 nil")

	// id=2: name=nil, score=95.5
	assert.Nil(t, pageResult.Rows[1][1], "id=2 name 应为 nil")
	assert.NotNil(t, pageResult.Rows[1][2], "id=2 score 应为非 nil")

	// id=3: name="Alice", score=nil
	assert.NotNil(t, pageResult.Rows[2][1], "id=3 name 应为非 nil")
	assert.Nil(t, pageResult.Rows[2][2], "id=3 score 应为 nil")

	// id=4: name=nil, score=nil
	assert.Nil(t, pageResult.Rows[3][1], "id=4 name 应为 nil")
	assert.Nil(t, pageResult.Rows[3][2], "id=4 score 应为 nil")
}

func TestClickHouse_ComplexTypes_DateTime_Integration(t *testing.T) {
	conn, cleanup := setupClickHouse(t)
	defer cleanup()

	ctx := context.Background()
	tableName := fmt.Sprintf("test_datetime_%d", time.Now().UnixNano())

	// 创建带多种时间类型的表
	err := conn.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id UInt64,
			created_at DateTime,
			updated_at DateTime64(3)
		) ENGINE = Memory
	`, tableName))
	require.NoError(t, err)

	wrapper, err := xclickhouse.New(conn)
	require.NoError(t, err)

	type dtRow struct {
		ID        uint64    `ch:"id"`
		CreatedAt time.Time `ch:"created_at"`
		UpdatedAt time.Time `ch:"updated_at"`
	}

	now := time.Now().Truncate(time.Millisecond)
	rows := []any{
		&dtRow{ID: 1, CreatedAt: now, UpdatedAt: now},
		&dtRow{ID: 2, CreatedAt: now.Add(-24 * time.Hour), UpdatedAt: now.Add(-1 * time.Hour)},
	}

	result, err := wrapper.BatchInsert(ctx, tableName, rows, xclickhouse.BatchOptions{})
	require.NoError(t, err)
	assert.Equal(t, int64(2), result.InsertedCount)

	// 查询侧断言：验证 DateTime/DateTime64 类型的读取和精度
	pageResult, err := wrapper.QueryPage(ctx,
		fmt.Sprintf("SELECT id, created_at, updated_at FROM %s ORDER BY id", tableName),
		xclickhouse.PageOptions{Page: 1, PageSize: 10},
	)
	require.NoError(t, err)
	assert.Equal(t, int64(2), pageResult.Total)
	require.Len(t, pageResult.Rows, 2)

	// 验证返回类型为 time.Time
	for i, row := range pageResult.Rows {
		createdAt, ok := row[1].(time.Time)
		assert.True(t, ok, "id=%d created_at 应为 time.Time 类型", i+1)
		assert.False(t, createdAt.IsZero(), "id=%d created_at 不应为零值", i+1)

		updatedAt, ok := row[2].(time.Time)
		assert.True(t, ok, "id=%d updated_at 应为 time.Time 类型", i+1)
		assert.False(t, updatedAt.IsZero(), "id=%d updated_at 不应为零值", i+1)
	}
}

// =============================================================================
// 慢查询钩子测试
// =============================================================================

func TestClickHouse_SlowQueryHook_Integration(t *testing.T) {
	conn, cleanup := setupClickHouse(t)
	defer cleanup()

	ctx := context.Background()
	tableName := fmt.Sprintf("test_slow_%d", time.Now().UnixNano())

	err := conn.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id UInt64,
			name String
		) ENGINE = Memory
	`, tableName))
	require.NoError(t, err)

	// 记录慢查询
	var slowQueries []xclickhouse.SlowQueryInfo
	var mu sync.Mutex

	wrapper, err := xclickhouse.New(conn,
		xclickhouse.WithSlowQueryThreshold(1*time.Nanosecond), // 设置极低阈值以触发
		xclickhouse.WithSlowQueryHook(func(ctx context.Context, info xclickhouse.SlowQueryInfo) {
			mu.Lock()
			defer mu.Unlock()
			slowQueries = append(slowQueries, info)
		}),
	)
	require.NoError(t, err)

	// 插入数据
	rows := []any{&simpleRow{ID: 1, Name: "test"}}
	_, err = wrapper.BatchInsert(ctx, tableName, rows, xclickhouse.BatchOptions{})
	require.NoError(t, err)

	// 查询数据
	_, err = wrapper.QueryPage(ctx,
		fmt.Sprintf("SELECT id, name FROM %s", tableName),
		xclickhouse.PageOptions{Page: 1, PageSize: 10},
	)
	require.NoError(t, err)

	// 验证慢查询钩子被调用
	mu.Lock()
	defer mu.Unlock()
	assert.NotEmpty(t, slowQueries, "慢查询钩子应被调用")

	// 验证统计
	stats := wrapper.Stats()
	assert.Greater(t, stats.SlowQueries, int64(0), "慢查询计数应大于 0")
}

func TestClickHouse_SlowQueryHook_NotTriggered_Integration(t *testing.T) {
	conn, cleanup := setupClickHouse(t)
	defer cleanup()

	ctx := context.Background()
	tableName := fmt.Sprintf("test_fast_%d", time.Now().UnixNano())

	err := conn.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id UInt64,
			name String
		) ENGINE = Memory
	`, tableName))
	require.NoError(t, err)

	// 设置很高的阈值
	hookCalled := false
	wrapper, err := xclickhouse.New(conn,
		xclickhouse.WithSlowQueryThreshold(1*time.Hour), // 不可能达到
		xclickhouse.WithSlowQueryHook(func(ctx context.Context, info xclickhouse.SlowQueryInfo) {
			hookCalled = true
		}),
	)
	require.NoError(t, err)

	// 快速查询
	rows := []any{&simpleRow{ID: 1, Name: "test"}}
	_, err = wrapper.BatchInsert(ctx, tableName, rows, xclickhouse.BatchOptions{})
	require.NoError(t, err)

	_, err = wrapper.QueryPage(ctx,
		fmt.Sprintf("SELECT id, name FROM %s", tableName),
		xclickhouse.PageOptions{Page: 1, PageSize: 10},
	)
	require.NoError(t, err)

	assert.False(t, hookCalled, "慢查询钩子不应被调用")
}

// =============================================================================
// 错误处理测试
// =============================================================================

func TestClickHouse_QueryPage_InvalidPage_Integration(t *testing.T) {
	conn, cleanup := setupClickHouse(t)
	defer cleanup()

	wrapper, err := xclickhouse.New(conn)
	require.NoError(t, err)

	ctx := context.Background()

	// 无效页码
	_, err = wrapper.QueryPage(ctx, "SELECT 1", xclickhouse.PageOptions{Page: 0, PageSize: 10})
	assert.Error(t, err)

	_, err = wrapper.QueryPage(ctx, "SELECT 1", xclickhouse.PageOptions{Page: -1, PageSize: 10})
	assert.Error(t, err)
}

func TestClickHouse_QueryPage_InvalidPageSize_Integration(t *testing.T) {
	conn, cleanup := setupClickHouse(t)
	defer cleanup()

	wrapper, err := xclickhouse.New(conn)
	require.NoError(t, err)

	ctx := context.Background()

	// 无效页大小
	_, err = wrapper.QueryPage(ctx, "SELECT 1", xclickhouse.PageOptions{Page: 1, PageSize: 0})
	assert.Error(t, err)

	_, err = wrapper.QueryPage(ctx, "SELECT 1", xclickhouse.PageOptions{Page: 1, PageSize: -1})
	assert.Error(t, err)
}

func TestClickHouse_QueryPage_EmptyQuery_Integration(t *testing.T) {
	conn, cleanup := setupClickHouse(t)
	defer cleanup()

	wrapper, err := xclickhouse.New(conn)
	require.NoError(t, err)

	ctx := context.Background()

	_, err = wrapper.QueryPage(ctx, "", xclickhouse.PageOptions{Page: 1, PageSize: 10})
	assert.Error(t, err)
}

func TestClickHouse_BatchInsert_EmptyRows_Integration(t *testing.T) {
	conn, cleanup := setupClickHouse(t)
	defer cleanup()

	wrapper, err := xclickhouse.New(conn)
	require.NoError(t, err)

	ctx := context.Background()

	_, err = wrapper.BatchInsert(ctx, "test_table", []any{}, xclickhouse.BatchOptions{})
	assert.Error(t, err)
}

func TestClickHouse_BatchInsert_InvalidTableName_Integration(t *testing.T) {
	conn, cleanup := setupClickHouse(t)
	defer cleanup()

	wrapper, err := xclickhouse.New(conn)
	require.NoError(t, err)

	ctx := context.Background()

	// 空表名
	_, err = wrapper.BatchInsert(ctx, "", []any{&simpleRow{ID: 1, Name: "test"}}, xclickhouse.BatchOptions{})
	assert.Error(t, err)

	// 非法表名（SQL 注入尝试）
	_, err = wrapper.BatchInsert(ctx, "table; DROP TABLE users--", []any{&simpleRow{ID: 1, Name: "test"}}, xclickhouse.BatchOptions{})
	assert.Error(t, err)
}

// =============================================================================
// 统计计数测试
// =============================================================================

func TestClickHouse_Stats_QueryCount_Integration(t *testing.T) {
	conn, cleanup := setupClickHouse(t)
	defer cleanup()

	ctx := context.Background()
	tableName := fmt.Sprintf("test_stats_%d", time.Now().UnixNano())

	err := conn.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id UInt64,
			name String
		) ENGINE = Memory
	`, tableName))
	require.NoError(t, err)

	wrapper, err := xclickhouse.New(conn)
	require.NoError(t, err)

	// 插入数据
	rows := []any{
		&simpleRow{ID: 1, Name: "test1"},
		&simpleRow{ID: 2, Name: "test2"},
	}
	_, err = wrapper.BatchInsert(ctx, tableName, rows, xclickhouse.BatchOptions{})
	require.NoError(t, err)

	// 执行多次查询
	for i := 0; i < 5; i++ {
		_, err = wrapper.QueryPage(ctx,
			fmt.Sprintf("SELECT id, name FROM %s", tableName),
			xclickhouse.PageOptions{Page: 1, PageSize: 10},
		)
		require.NoError(t, err)
	}

	stats := wrapper.Stats()
	// 每次 QueryPage 执行 count 查询 + 分页查询 = 2 次内部查询
	// QueryCount 统计的是实际 SQL 执行次数，5 次 QueryPage = 10 次查询
	assert.Equal(t, int64(10), stats.QueryCount, "查询计数应为 10（5 次 QueryPage × 2）")
	assert.Zero(t, stats.QueryErrors, "查询错误应为 0")
}

// =============================================================================
// 并发测试
// =============================================================================

func TestClickHouse_Concurrent_Insert_Integration(t *testing.T) {
	conn, cleanup := setupClickHouse(t)
	defer cleanup()

	ctx := context.Background()
	tableName := fmt.Sprintf("test_concurrent_insert_%d", time.Now().UnixNano())

	err := conn.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id UInt64,
			worker_id UInt64,
			name String
		) ENGINE = MergeTree()
		ORDER BY (id)
	`, tableName))
	require.NoError(t, err)

	wrapper, err := xclickhouse.New(conn)
	require.NoError(t, err)

	const workers = 5
	const rowsPerWorker = 100

	var wg sync.WaitGroup
	var totalInserted int64

	for w := 0; w < workers; w++ {
		workerID := w
		wg.Add(1)
		go func() {
			defer wg.Done()
			type workerRow struct {
				ID       uint64 `ch:"id"`
				WorkerID uint64 `ch:"worker_id"`
				Name     string `ch:"name"`
			}

			rows := make([]any, rowsPerWorker)
			for i := 0; i < rowsPerWorker; i++ {
				rows[i] = &workerRow{
					ID:       uint64(workerID*rowsPerWorker + i),
					WorkerID: uint64(workerID),
					Name:     fmt.Sprintf("worker-%d-item-%d", workerID, i),
				}
			}

			result, err := wrapper.BatchInsert(ctx, tableName, rows, xclickhouse.BatchOptions{BatchSize: 50})
			if err != nil {
				t.Logf("Worker %d insert error: %v", workerID, err)
				return
			}

			atomic.AddInt64(&totalInserted, result.InsertedCount)
		}()
	}

	wg.Wait()

	assert.Equal(t, int64(workers*rowsPerWorker), totalInserted)

	// 验证总数
	var count uint64
	err = conn.QueryRow(ctx, fmt.Sprintf("SELECT count() FROM %s", tableName)).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, uint64(workers*rowsPerWorker), count)
}

func TestClickHouse_Concurrent_Query_Integration(t *testing.T) {
	conn, cleanup := setupClickHouse(t)
	defer cleanup()

	ctx := context.Background()
	tableName := fmt.Sprintf("test_concurrent_query_%d", time.Now().UnixNano())

	err := conn.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id UInt64,
			name String
		) ENGINE = Memory
	`, tableName))
	require.NoError(t, err)

	wrapper, err := xclickhouse.New(conn)
	require.NoError(t, err)

	// 插入测试数据
	rows := make([]any, 100)
	for i := 0; i < 100; i++ {
		rows[i] = &simpleRow{ID: uint64(i + 1), Name: fmt.Sprintf("item_%d", i+1)}
	}
	_, err = wrapper.BatchInsert(ctx, tableName, rows, xclickhouse.BatchOptions{})
	require.NoError(t, err)

	const workers = 10
	var wg sync.WaitGroup
	var successCount int64

	for w := 0; w < workers; w++ {
		workerID := w
		wg.Add(1)
		go func() {
			defer wg.Done()
			// 每个 worker 查询不同的页
			page := int64((workerID % 10) + 1)
			result, err := wrapper.QueryPage(ctx,
				fmt.Sprintf("SELECT id, name FROM %s ORDER BY id", tableName),
				xclickhouse.PageOptions{Page: page, PageSize: 10},
			)
			if err != nil {
				t.Logf("Worker %d query error: %v", workerID, err)
				return
			}

			if result.Total == 100 {
				atomic.AddInt64(&successCount, 1)
			}
		}()
	}

	wg.Wait()

	assert.Equal(t, int64(workers), successCount, "所有查询应成功")
}

// =============================================================================
// 健康检查超时测试
// =============================================================================

func TestClickHouse_HealthTimeout_Integration(t *testing.T) {
	conn, cleanup := setupClickHouse(t)
	defer cleanup()

	wrapper, err := xclickhouse.New(conn,
		xclickhouse.WithHealthTimeout(5*time.Second),
	)
	require.NoError(t, err)

	ctx := context.Background()
	err = wrapper.Health(ctx)
	assert.NoError(t, err)
}

// =============================================================================
// 真实场景测试
// =============================================================================

func TestClickHouse_RealWorldScenario_LogAnalytics_Integration(t *testing.T) {
	conn, cleanup := setupClickHouse(t)
	defer cleanup()

	ctx := context.Background()
	tableName := fmt.Sprintf("test_logs_%d", time.Now().UnixNano())

	// 创建日志表
	err := conn.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			timestamp DateTime64(3),
			level String,
			service String,
			message String,
			trace_id String,
			duration_ms UInt64
		) ENGINE = MergeTree()
		ORDER BY (service, timestamp)
		PARTITION BY toYYYYMMDD(timestamp)
	`, tableName))
	require.NoError(t, err)

	wrapper, err := xclickhouse.New(conn)
	require.NoError(t, err)

	type logEntry struct {
		Timestamp  time.Time `ch:"timestamp"`
		Level      string    `ch:"level"`
		Service    string    `ch:"service"`
		Message    string    `ch:"message"`
		TraceID    string    `ch:"trace_id"`
		DurationMs uint64    `ch:"duration_ms"`
	}

	// 模拟插入日志
	now := time.Now()
	logs := []any{
		&logEntry{Timestamp: now.Add(-5 * time.Minute), Level: "INFO", Service: "api-gateway", Message: "Request started", TraceID: "trace-001", DurationMs: 0},
		&logEntry{Timestamp: now.Add(-4 * time.Minute), Level: "DEBUG", Service: "user-service", Message: "Fetching user", TraceID: "trace-001", DurationMs: 15},
		&logEntry{Timestamp: now.Add(-3 * time.Minute), Level: "ERROR", Service: "payment-service", Message: "Payment failed", TraceID: "trace-001", DurationMs: 500},
		&logEntry{Timestamp: now.Add(-2 * time.Minute), Level: "WARN", Service: "api-gateway", Message: "Retry initiated", TraceID: "trace-001", DurationMs: 0},
		&logEntry{Timestamp: now.Add(-1 * time.Minute), Level: "INFO", Service: "api-gateway", Message: "Request completed", TraceID: "trace-001", DurationMs: 520},
	}

	result, err := wrapper.BatchInsert(ctx, tableName, logs, xclickhouse.BatchOptions{})
	require.NoError(t, err)
	assert.Equal(t, int64(5), result.InsertedCount)

	// 查询错误日志
	pageResult, err := wrapper.QueryPage(ctx,
		fmt.Sprintf("SELECT timestamp, level, service, message FROM %s WHERE level = 'ERROR' ORDER BY timestamp", tableName),
		xclickhouse.PageOptions{Page: 1, PageSize: 10},
	)
	require.NoError(t, err)
	assert.Equal(t, int64(1), pageResult.Total)

	// 按服务聚合
	aggResult, err := wrapper.QueryPage(ctx,
		fmt.Sprintf("SELECT service, count() as cnt, avg(duration_ms) as avg_duration FROM %s GROUP BY service ORDER BY cnt DESC", tableName),
		xclickhouse.PageOptions{Page: 1, PageSize: 10},
	)
	require.NoError(t, err)
	assert.Equal(t, int64(3), aggResult.Total, "应有 3 个服务")
}

func TestClickHouse_RealWorldScenario_TimeSeriesMetrics_Integration(t *testing.T) {
	conn, cleanup := setupClickHouse(t)
	defer cleanup()

	ctx := context.Background()
	tableName := fmt.Sprintf("test_metrics_%d", time.Now().UnixNano())

	// 创建指标表
	err := conn.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			timestamp DateTime,
			host String,
			metric_name String,
			value Float64,
			tags Map(String, String)
		) ENGINE = MergeTree()
		ORDER BY (metric_name, host, timestamp)
		PARTITION BY toYYYYMMDD(timestamp)
	`, tableName))
	require.NoError(t, err)

	wrapper, err := xclickhouse.New(conn)
	require.NoError(t, err)

	type metric struct {
		Timestamp  time.Time         `ch:"timestamp"`
		Host       string            `ch:"host"`
		MetricName string            `ch:"metric_name"`
		Value      float64           `ch:"value"`
		Tags       map[string]string `ch:"tags"`
	}

	// 模拟插入指标数据
	now := time.Now().Truncate(time.Second)
	metrics := make([]any, 60)
	for i := 0; i < 60; i++ {
		host := fmt.Sprintf("server-%d", (i%3)+1)
		metrics[i] = &metric{
			Timestamp:  now.Add(time.Duration(-i) * time.Minute),
			Host:       host,
			MetricName: "cpu_usage",
			Value:      50.0 + float64(i%20),
			Tags:       map[string]string{"env": "prod", "region": "us-west"},
		}
	}

	result, err := wrapper.BatchInsert(ctx, tableName, metrics, xclickhouse.BatchOptions{BatchSize: 100})
	require.NoError(t, err)
	assert.Equal(t, int64(60), result.InsertedCount)

	// 查询最近的指标
	pageResult, err := wrapper.QueryPage(ctx,
		fmt.Sprintf("SELECT timestamp, host, value FROM %s WHERE metric_name = 'cpu_usage' ORDER BY timestamp DESC", tableName),
		xclickhouse.PageOptions{Page: 1, PageSize: 10},
	)
	require.NoError(t, err)
	assert.Equal(t, int64(60), pageResult.Total)
	assert.Len(t, pageResult.Rows, 10)

	// 按主机聚合
	aggResult, err := wrapper.QueryPage(ctx,
		fmt.Sprintf("SELECT host, avg(value) as avg_cpu, max(value) as max_cpu FROM %s WHERE metric_name = 'cpu_usage' GROUP BY host ORDER BY host", tableName),
		xclickhouse.PageOptions{Page: 1, PageSize: 10},
	)
	require.NoError(t, err)
	assert.Equal(t, int64(3), aggResult.Total, "应有 3 台主机")
}
