//go:build integration

package xclickhouse_test

import (
	"context"
	"fmt"
	"os"
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
