//go:build integration

package xclickhouse_test

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/omeyang/xkit/pkg/storage/xclickhouse"
)

// =============================================================================
// 测试设置
// =============================================================================

// setupClickHouse 启动 ClickHouse 容器或连接到已有 ClickHouse。
// 如果设置了 XKIT_CLICKHOUSE_ADDR 环境变量，直接使用外部 ClickHouse。
func setupClickHouse(t *testing.T) (clickhouse.Conn, func()) {
	t.Helper()

	addr := os.Getenv("XKIT_CLICKHOUSE_ADDR")
	if addr == "" {
		addr = startClickHouseContainer(t)
	}

	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{addr},
		Auth: clickhouse.Auth{
			Database: "default",
			Username: "default",
			Password: "",
		},
		DialTimeout: 10 * time.Second,
		Settings: clickhouse.Settings{
			"max_execution_time": 60,
		},
	})
	require.NoError(t, err, "打开 ClickHouse 连接失败")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	require.NoError(t, conn.Ping(ctx), "ClickHouse ping 失败")

	cleanup := func() {
		conn.Close()
	}

	return conn, cleanup
}

// startClickHouseContainer 使用 testcontainers 启动 ClickHouse 容器。
func startClickHouseContainer(t *testing.T) string {
	t.Helper()

	ctx := context.Background()
	req := testcontainers.ContainerRequest{
		Image:        "clickhouse/clickhouse-server:23.12",
		ExposedPorts: []string{"9000/tcp"},
		WaitingFor: wait.ForAll(
			wait.ForListeningPort("9000/tcp"),
			wait.ForLog("Ready for connections"),
		).WithDeadline(60 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Skipf("无法启动 ClickHouse 容器: %v", err)
	}

	t.Cleanup(func() {
		container.Terminate(ctx)
	})

	host, err := container.Host(ctx)
	require.NoError(t, err, "获取容器 host 失败")

	port, err := container.MappedPort(ctx, "9000/tcp")
	require.NoError(t, err, "获取容器端口失败")

	return fmt.Sprintf("%s:%s", host, port.Port())
}

// =============================================================================
// 测试数据结构
// =============================================================================

// simpleRow 简单行结构。
type simpleRow struct {
	ID   uint64 `ch:"id"`
	Name string `ch:"name"`
}

// eventRow 事件行结构，用于 MergeTree 测试。
type eventRow struct {
	ID        uint64    `ch:"id"`
	EventType string    `ch:"event_type"`
	UserID    uint64    `ch:"user_id"`
	Value     float64   `ch:"value"`
	CreatedAt time.Time `ch:"created_at"`
}

// complexRow 复杂类型行结构。
type complexRow struct {
	ID     uint64            `ch:"id"`
	Tags   []string          `ch:"tags"`
	Attrs  map[string]string `ch:"attrs"`
	Values []float64         `ch:"values"`
}

// =============================================================================
// 基本操作测试
// =============================================================================

func TestClickHouse_Health_Integration(t *testing.T) {
	conn, cleanup := setupClickHouse(t)
	defer cleanup()

	wrapper, err := xclickhouse.New(conn)
	require.NoError(t, err)

	ctx := context.Background()
	err = wrapper.Health(ctx)
	assert.NoError(t, err)
}

func TestClickHouse_Stats_Integration(t *testing.T) {
	conn, cleanup := setupClickHouse(t)
	defer cleanup()

	wrapper, err := xclickhouse.New(conn)
	require.NoError(t, err)

	ctx := context.Background()

	// 执行几次健康检查
	for i := 0; i < 3; i++ {
		_ = wrapper.Health(ctx)
	}

	stats := wrapper.Stats()
	assert.Equal(t, int64(3), stats.PingCount, "ping 计数应为 3")
	assert.Zero(t, stats.PingErrors, "ping 错误应为 0")
}

func TestClickHouse_Close_Integration(t *testing.T) {
	conn, cleanup := setupClickHouse(t)
	defer cleanup()

	wrapper, err := xclickhouse.New(conn)
	require.NoError(t, err)

	err = wrapper.Close()
	assert.NoError(t, err)
}

// =============================================================================
// BatchInsert 测试
// =============================================================================

func TestClickHouse_BatchInsert_Basic_Integration(t *testing.T) {
	conn, cleanup := setupClickHouse(t)
	defer cleanup()

	ctx := context.Background()
	tableName := fmt.Sprintf("test_batch_basic_%d", time.Now().UnixNano())

	// 创建表
	err := conn.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id UInt64,
			name String
		) ENGINE = Memory
	`, tableName))
	require.NoError(t, err)

	wrapper, err := xclickhouse.New(conn)
	require.NoError(t, err)

	// 准备数据
	rows := []any{
		&simpleRow{ID: 1, Name: "Alice"},
		&simpleRow{ID: 2, Name: "Bob"},
		&simpleRow{ID: 3, Name: "Charlie"},
	}

	// 批量插入
	result, err := wrapper.BatchInsert(ctx, tableName, rows, xclickhouse.BatchOptions{BatchSize: 10})
	require.NoError(t, err)
	assert.Equal(t, int64(3), result.InsertedCount)
	assert.Empty(t, result.Errors)

	// 验证数据
	var count uint64
	err = conn.QueryRow(ctx, fmt.Sprintf("SELECT count() FROM %s", tableName)).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, uint64(3), count)
}

func TestClickHouse_BatchInsert_LargeBatch_Integration(t *testing.T) {
	conn, cleanup := setupClickHouse(t)
	defer cleanup()

	ctx := context.Background()
	tableName := fmt.Sprintf("test_batch_large_%d", time.Now().UnixNano())

	// 创建表
	err := conn.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id UInt64,
			name String
		) ENGINE = Memory
	`, tableName))
	require.NoError(t, err)

	wrapper, err := xclickhouse.New(conn)
	require.NoError(t, err)

	// 准备大量数据
	const rowCount = 10000
	rows := make([]any, rowCount)
	for i := 0; i < rowCount; i++ {
		rows[i] = &simpleRow{ID: uint64(i + 1), Name: fmt.Sprintf("user_%d", i+1)}
	}

	// 使用较小的批次大小
	result, err := wrapper.BatchInsert(ctx, tableName, rows, xclickhouse.BatchOptions{BatchSize: 1000})
	require.NoError(t, err)
	assert.Equal(t, int64(rowCount), result.InsertedCount)
	assert.Empty(t, result.Errors)

	// 验证数据
	var count uint64
	err = conn.QueryRow(ctx, fmt.Sprintf("SELECT count() FROM %s", tableName)).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, uint64(rowCount), count)
}

func TestClickHouse_BatchInsert_DefaultBatchSize_Integration(t *testing.T) {
	conn, cleanup := setupClickHouse(t)
	defer cleanup()

	ctx := context.Background()
	tableName := fmt.Sprintf("test_batch_default_%d", time.Now().UnixNano())

	err := conn.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id UInt64,
			name String
		) ENGINE = Memory
	`, tableName))
	require.NoError(t, err)

	wrapper, err := xclickhouse.New(conn)
	require.NoError(t, err)

	rows := []any{
		&simpleRow{ID: 1, Name: "test1"},
		&simpleRow{ID: 2, Name: "test2"},
	}

	// 不指定 BatchSize，使用默认值
	result, err := wrapper.BatchInsert(ctx, tableName, rows, xclickhouse.BatchOptions{})
	require.NoError(t, err)
	assert.Equal(t, int64(2), result.InsertedCount)
}

// =============================================================================
// QueryPage 测试
// =============================================================================

func TestClickHouse_QueryPage_Basic_Integration(t *testing.T) {
	conn, cleanup := setupClickHouse(t)
	defer cleanup()

	ctx := context.Background()
	tableName := fmt.Sprintf("test_page_basic_%d", time.Now().UnixNano())

	// 创建表并插入数据
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
	rows := make([]any, 25)
	for i := 0; i < 25; i++ {
		rows[i] = &simpleRow{ID: uint64(i + 1), Name: fmt.Sprintf("user_%d", i+1)}
	}
	_, err = wrapper.BatchInsert(ctx, tableName, rows, xclickhouse.BatchOptions{})
	require.NoError(t, err)

	// 查询第一页
	result, err := wrapper.QueryPage(ctx,
		fmt.Sprintf("SELECT id, name FROM %s ORDER BY id", tableName),
		xclickhouse.PageOptions{Page: 1, PageSize: 10},
	)
	require.NoError(t, err)

	assert.Equal(t, int64(25), result.Total)
	assert.Equal(t, int64(1), result.Page)
	assert.Equal(t, int64(10), result.PageSize)
	assert.Equal(t, int64(3), result.TotalPages)
	assert.Len(t, result.Rows, 10)
	assert.Equal(t, []string{"id", "name"}, result.Columns)
}

func TestClickHouse_QueryPage_SecondPage_Integration(t *testing.T) {
	conn, cleanup := setupClickHouse(t)
	defer cleanup()

	ctx := context.Background()
	tableName := fmt.Sprintf("test_page_second_%d", time.Now().UnixNano())

	err := conn.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id UInt64,
			name String
		) ENGINE = Memory
	`, tableName))
	require.NoError(t, err)

	wrapper, err := xclickhouse.New(conn)
	require.NoError(t, err)

	// 插入 25 条数据
	rows := make([]any, 25)
	for i := 0; i < 25; i++ {
		rows[i] = &simpleRow{ID: uint64(i + 1), Name: fmt.Sprintf("user_%d", i+1)}
	}
	_, err = wrapper.BatchInsert(ctx, tableName, rows, xclickhouse.BatchOptions{})
	require.NoError(t, err)

	// 查询第二页
	result, err := wrapper.QueryPage(ctx,
		fmt.Sprintf("SELECT id, name FROM %s ORDER BY id", tableName),
		xclickhouse.PageOptions{Page: 2, PageSize: 10},
	)
	require.NoError(t, err)

	assert.Len(t, result.Rows, 10)
	// 第二页应该从 id=11 开始
	if len(result.Rows) > 0 {
		firstID := result.Rows[0][0].(uint64)
		assert.Equal(t, uint64(11), firstID)
	}
}

func TestClickHouse_QueryPage_LastPage_Integration(t *testing.T) {
	conn, cleanup := setupClickHouse(t)
	defer cleanup()

	ctx := context.Background()
	tableName := fmt.Sprintf("test_page_last_%d", time.Now().UnixNano())

	err := conn.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id UInt64,
			name String
		) ENGINE = Memory
	`, tableName))
	require.NoError(t, err)

	wrapper, err := xclickhouse.New(conn)
	require.NoError(t, err)

	// 插入 25 条数据
	rows := make([]any, 25)
	for i := 0; i < 25; i++ {
		rows[i] = &simpleRow{ID: uint64(i + 1), Name: fmt.Sprintf("user_%d", i+1)}
	}
	_, err = wrapper.BatchInsert(ctx, tableName, rows, xclickhouse.BatchOptions{})
	require.NoError(t, err)

	// 查询最后一页（第 3 页，只有 5 条）
	result, err := wrapper.QueryPage(ctx,
		fmt.Sprintf("SELECT id, name FROM %s ORDER BY id", tableName),
		xclickhouse.PageOptions{Page: 3, PageSize: 10},
	)
	require.NoError(t, err)

	assert.Len(t, result.Rows, 5, "最后一页应只有 5 条记录")
}

func TestClickHouse_QueryPage_WithWhere_Integration(t *testing.T) {
	conn, cleanup := setupClickHouse(t)
	defer cleanup()

	ctx := context.Background()
	tableName := fmt.Sprintf("test_page_where_%d", time.Now().UnixNano())

	err := conn.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id UInt64,
			category String,
			value UInt64
		) ENGINE = Memory
	`, tableName))
	require.NoError(t, err)

	wrapper, err := xclickhouse.New(conn)
	require.NoError(t, err)

	// 插入不同分类的数据
	type catRow struct {
		ID       uint64 `ch:"id"`
		Category string `ch:"category"`
		Value    uint64 `ch:"value"`
	}

	rows := []any{
		&catRow{ID: 1, Category: "A", Value: 100},
		&catRow{ID: 2, Category: "B", Value: 200},
		&catRow{ID: 3, Category: "A", Value: 150},
		&catRow{ID: 4, Category: "B", Value: 250},
		&catRow{ID: 5, Category: "A", Value: 120},
	}
	_, err = wrapper.BatchInsert(ctx, tableName, rows, xclickhouse.BatchOptions{})
	require.NoError(t, err)

	// 查询分类 A 的数据
	result, err := wrapper.QueryPage(ctx,
		fmt.Sprintf("SELECT id, category, value FROM %s WHERE category = 'A' ORDER BY id", tableName),
		xclickhouse.PageOptions{Page: 1, PageSize: 10},
	)
	require.NoError(t, err)

	assert.Equal(t, int64(3), result.Total, "分类 A 应有 3 条记录")
	assert.Len(t, result.Rows, 3)
}

func TestClickHouse_QueryPage_EmptyResult_Integration(t *testing.T) {
	conn, cleanup := setupClickHouse(t)
	defer cleanup()

	ctx := context.Background()
	tableName := fmt.Sprintf("test_page_empty_%d", time.Now().UnixNano())

	err := conn.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id UInt64,
			name String
		) ENGINE = Memory
	`, tableName))
	require.NoError(t, err)

	wrapper, err := xclickhouse.New(conn)
	require.NoError(t, err)

	// 空表查询
	result, err := wrapper.QueryPage(ctx,
		fmt.Sprintf("SELECT id, name FROM %s", tableName),
		xclickhouse.PageOptions{Page: 1, PageSize: 10},
	)
	require.NoError(t, err)

	assert.Equal(t, int64(0), result.Total)
	assert.Empty(t, result.Rows)
	assert.Equal(t, int64(0), result.TotalPages)
}

func TestClickHouse_QueryPage_WithArgs_Integration(t *testing.T) {
	conn, cleanup := setupClickHouse(t)
	defer cleanup()

	ctx := context.Background()
	tableName := fmt.Sprintf("test_page_args_%d", time.Now().UnixNano())

	err := conn.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id UInt64,
			value UInt64
		) ENGINE = Memory
	`, tableName))
	require.NoError(t, err)

	wrapper, err := xclickhouse.New(conn)
	require.NoError(t, err)

	type valRow struct {
		ID    uint64 `ch:"id"`
		Value uint64 `ch:"value"`
	}

	rows := make([]any, 20)
	for i := 0; i < 20; i++ {
		rows[i] = &valRow{ID: uint64(i + 1), Value: uint64((i + 1) * 10)}
	}
	_, err = wrapper.BatchInsert(ctx, tableName, rows, xclickhouse.BatchOptions{})
	require.NoError(t, err)

	// 使用参数化查询
	result, err := wrapper.QueryPage(ctx,
		fmt.Sprintf("SELECT id, value FROM %s WHERE value > {minVal:UInt64} ORDER BY id", tableName),
		xclickhouse.PageOptions{Page: 1, PageSize: 10},
		clickhouse.Named("minVal", uint64(100)),
	)
	require.NoError(t, err)

	// value > 100 意味着 id > 10
	assert.Equal(t, int64(10), result.Total)
}

// =============================================================================
// MergeTree 引擎测试（更接近生产环境）
// =============================================================================

func TestClickHouse_MergeTree_Integration(t *testing.T) {
	conn, cleanup := setupClickHouse(t)
	defer cleanup()

	ctx := context.Background()
	tableName := fmt.Sprintf("test_mergetree_%d", time.Now().UnixNano())

	// 创建 MergeTree 表
	err := conn.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id UInt64,
			event_type String,
			user_id UInt64,
			value Float64,
			created_at DateTime
		) ENGINE = MergeTree()
		ORDER BY (event_type, created_at)
		PARTITION BY toYYYYMM(created_at)
	`, tableName))
	require.NoError(t, err)

	wrapper, err := xclickhouse.New(conn)
	require.NoError(t, err)

	// 插入事件数据
	now := time.Now().Truncate(time.Second)
	rows := []any{
		&eventRow{ID: 1, EventType: "click", UserID: 100, Value: 1.5, CreatedAt: now.Add(-1 * time.Hour)},
		&eventRow{ID: 2, EventType: "view", UserID: 100, Value: 0.5, CreatedAt: now.Add(-2 * time.Hour)},
		&eventRow{ID: 3, EventType: "click", UserID: 101, Value: 2.0, CreatedAt: now.Add(-30 * time.Minute)},
		&eventRow{ID: 4, EventType: "purchase", UserID: 100, Value: 99.99, CreatedAt: now},
	}

	result, err := wrapper.BatchInsert(ctx, tableName, rows, xclickhouse.BatchOptions{})
	require.NoError(t, err)
	assert.Equal(t, int64(4), result.InsertedCount)

	// 分页查询
	pageResult, err := wrapper.QueryPage(ctx,
		fmt.Sprintf("SELECT id, event_type, user_id, value FROM %s ORDER BY id", tableName),
		xclickhouse.PageOptions{Page: 1, PageSize: 10},
	)
	require.NoError(t, err)
	assert.Equal(t, int64(4), pageResult.Total)
}

func TestClickHouse_MergeTree_Aggregation_Integration(t *testing.T) {
	conn, cleanup := setupClickHouse(t)
	defer cleanup()

	ctx := context.Background()
	tableName := fmt.Sprintf("test_agg_%d", time.Now().UnixNano())

	err := conn.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id UInt64,
			event_type String,
			user_id UInt64,
			value Float64,
			created_at DateTime
		) ENGINE = MergeTree()
		ORDER BY (event_type, created_at)
	`, tableName))
	require.NoError(t, err)

	wrapper, err := xclickhouse.New(conn)
	require.NoError(t, err)

	now := time.Now().Truncate(time.Second)
	rows := []any{
		&eventRow{ID: 1, EventType: "click", UserID: 100, Value: 1.0, CreatedAt: now},
		&eventRow{ID: 2, EventType: "click", UserID: 100, Value: 2.0, CreatedAt: now},
		&eventRow{ID: 3, EventType: "view", UserID: 101, Value: 0.5, CreatedAt: now},
		&eventRow{ID: 4, EventType: "click", UserID: 102, Value: 3.0, CreatedAt: now},
		&eventRow{ID: 5, EventType: "view", UserID: 100, Value: 0.5, CreatedAt: now},
	}
	_, err = wrapper.BatchInsert(ctx, tableName, rows, xclickhouse.BatchOptions{})
	require.NoError(t, err)

	// 聚合查询
	pageResult, err := wrapper.QueryPage(ctx,
		fmt.Sprintf("SELECT event_type, count() as cnt, sum(value) as total FROM %s GROUP BY event_type ORDER BY event_type", tableName),
		xclickhouse.PageOptions{Page: 1, PageSize: 10},
	)
	require.NoError(t, err)

	assert.Equal(t, int64(2), pageResult.Total, "应有 2 种事件类型")
	assert.Len(t, pageResult.Rows, 2)
}

// =============================================================================
// 复杂数据类型测试
// =============================================================================

func TestClickHouse_ComplexTypes_Array_Integration(t *testing.T) {
	conn, cleanup := setupClickHouse(t)
	defer cleanup()

	ctx := context.Background()
	tableName := fmt.Sprintf("test_array_%d", time.Now().UnixNano())

	// 创建带 Array 类型的表
	err := conn.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id UInt64,
			tags Array(String),
			values Array(Float64)
		) ENGINE = Memory
	`, tableName))
	require.NoError(t, err)

	wrapper, err := xclickhouse.New(conn)
	require.NoError(t, err)

	type arrayRow struct {
		ID     uint64    `ch:"id"`
		Tags   []string  `ch:"tags"`
		Values []float64 `ch:"values"`
	}

	rows := []any{
		&arrayRow{ID: 1, Tags: []string{"go", "rust"}, Values: []float64{1.1, 2.2}},
		&arrayRow{ID: 2, Tags: []string{"python"}, Values: []float64{3.3}},
		&arrayRow{ID: 3, Tags: []string{}, Values: []float64{}}, // 空数组
	}

	result, err := wrapper.BatchInsert(ctx, tableName, rows, xclickhouse.BatchOptions{})
	require.NoError(t, err)
	assert.Equal(t, int64(3), result.InsertedCount)

	// 验证数据
	pageResult, err := wrapper.QueryPage(ctx,
		fmt.Sprintf("SELECT id, tags, values FROM %s ORDER BY id", tableName),
		xclickhouse.PageOptions{Page: 1, PageSize: 10},
	)
	require.NoError(t, err)
	assert.Equal(t, int64(3), pageResult.Total)
}

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
		wg.Add(1)
		go func(workerID int) {
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
		}(w)
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
		wg.Add(1)
		go func(workerID int) {
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
		}(w)
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
