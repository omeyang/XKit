package xclickhouse

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// =============================================================================
// 集成测试 - 需要真实 ClickHouse
// =============================================================================

func TestWrapper_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// 集成测试需要真实的 ClickHouse 连接
}

func TestWrapper_Health_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
}

func TestWrapper_Stats_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
}

// =============================================================================
// 单元测试 - 不需要真实 ClickHouse
// =============================================================================

func TestWrapper_Stats_Initial(t *testing.T) {
	// 验证初始统计值
	w := &clickhouseWrapper{
		conn:    nil,
		options: defaultOptions(),
	}

	stats := w.Stats()
	assert.Equal(t, int64(0), stats.PingCount)
	assert.Equal(t, int64(0), stats.PingErrors)
	assert.Equal(t, int64(0), stats.QueryCount)
	assert.Equal(t, int64(0), stats.QueryErrors)
	assert.Equal(t, int64(0), stats.SlowQueries)
}

func TestWrapper_SlowQueryHook(t *testing.T) {
	var captured SlowQueryInfo
	hook := func(_ context.Context, info SlowQueryInfo) {
		captured = info
	}

	w := &clickhouseWrapper{
		conn: nil,
		options: &Options{
			HealthTimeout:      5 * time.Second,
			SlowQueryThreshold: 100 * time.Millisecond,
			SlowQueryHook:      hook,
		},
	}

	// 模拟慢查询触发
	info := SlowQueryInfo{
		Query:    "SELECT * FROM users",
		Args:     []any{1, "test"},
		Duration: 200 * time.Millisecond,
	}

	w.triggerSlowQueryHook(context.Background(), info)

	assert.Equal(t, "SELECT * FROM users", captured.Query)
	assert.Len(t, captured.Args, 2)
	assert.Equal(t, 200*time.Millisecond, captured.Duration)
}

func TestWrapper_SlowQueryHook_NilHook(t *testing.T) {
	w := &clickhouseWrapper{
		conn: nil,
		options: &Options{
			HealthTimeout:      5 * time.Second,
			SlowQueryThreshold: 100 * time.Millisecond,
			SlowQueryHook:      nil, // 无钩子
		},
	}

	info := SlowQueryInfo{
		Query:    "SELECT * FROM users",
		Duration: 200 * time.Millisecond,
	}

	// 不应该 panic
	w.triggerSlowQueryHook(context.Background(), info)
}

func TestWrapper_SlowQueryHook_BelowThreshold(t *testing.T) {
	var called bool
	hook := func(_ context.Context, _ SlowQueryInfo) {
		called = true
	}

	w := &clickhouseWrapper{
		conn: nil,
		options: &Options{
			HealthTimeout:      5 * time.Second,
			SlowQueryThreshold: 100 * time.Millisecond,
			SlowQueryHook:      hook,
		},
	}

	// 耗时低于阈值
	info := SlowQueryInfo{
		Duration: 50 * time.Millisecond,
	}

	triggered := w.maybeSlowQuery(context.Background(), info)
	assert.False(t, called)
	assert.False(t, triggered)
}

func TestWrapper_SlowQueryHook_AboveThreshold(t *testing.T) {
	var called bool
	hook := func(_ context.Context, _ SlowQueryInfo) {
		called = true
	}

	w := &clickhouseWrapper{
		conn: nil,
		options: &Options{
			HealthTimeout:      5 * time.Second,
			SlowQueryThreshold: 100 * time.Millisecond,
			SlowQueryHook:      hook,
		},
	}

	// 耗时高于阈值
	info := SlowQueryInfo{
		Duration: 150 * time.Millisecond,
	}

	triggered := w.maybeSlowQuery(context.Background(), info)
	assert.True(t, called)
	assert.True(t, triggered)
}

func TestWrapper_SlowQueryHook_ThresholdDisabled(t *testing.T) {
	var called bool
	hook := func(_ context.Context, _ SlowQueryInfo) {
		called = true
	}

	w := &clickhouseWrapper{
		conn: nil,
		options: &Options{
			HealthTimeout:      5 * time.Second,
			SlowQueryThreshold: 0, // 禁用
			SlowQueryHook:      hook,
		},
	}

	info := SlowQueryInfo{
		Duration: 1000 * time.Millisecond,
	}

	triggered := w.maybeSlowQuery(context.Background(), info)
	assert.False(t, called)
	assert.False(t, triggered)
}

func TestWrapper_Close_NilConn(t *testing.T) {
	w := &clickhouseWrapper{
		conn:    nil,
		options: defaultOptions(),
	}

	// 关闭 nil conn 不应该出错
	err := w.Close()
	assert.NoError(t, err)
}

func TestWrapper_Conn_NilConn(t *testing.T) {
	w := &clickhouseWrapper{
		conn:    nil,
		options: defaultOptions(),
	}

	// 返回 nil conn
	assert.Nil(t, w.Conn())
}

func TestWrapper_Stats_Pool_NilConn(t *testing.T) {
	w := &clickhouseWrapper{
		conn:    nil,
		options: defaultOptions(),
	}

	stats := w.Stats().Pool
	assert.Equal(t, 0, stats.Open)
	assert.Equal(t, 0, stats.Idle)
	assert.Equal(t, 0, stats.InUse)
}

func TestMeasureOperation(t *testing.T) {
	start := time.Now()
	time.Sleep(10 * time.Millisecond)
	duration := measureOperation(start)

	assert.True(t, duration >= 10*time.Millisecond)
	assert.True(t, duration < 100*time.Millisecond)
}

func TestBuildCountQuery(t *testing.T) {
	// 使用子查询包装方式，避免复杂 SQL 解析问题
	tests := []struct {
		name     string
		query    string
		expected string
	}{
		{
			"标准查询",
			"SELECT id, name FROM users WHERE status = 1",
			"SELECT COUNT(*) FROM (SELECT id, name FROM users WHERE status = 1) AS _count_subquery",
		},
		{
			"带 JOIN 的查询",
			"SELECT u.id, o.amount FROM users u JOIN orders o ON u.id = o.user_id",
			"SELECT COUNT(*) FROM (SELECT u.id, o.amount FROM users u JOIN orders o ON u.id = o.user_id) AS _count_subquery",
		},
		{
			"小写查询",
			"select * from users",
			"SELECT COUNT(*) FROM (select * from users) AS _count_subquery",
		},
		{
			"带子查询的复杂查询",
			"SELECT * FROM (SELECT id FROM users) AS t WHERE t.id > 0",
			"SELECT COUNT(*) FROM (SELECT * FROM (SELECT id FROM users) AS t WHERE t.id > 0) AS _count_subquery",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildCountQuery(tt.query)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestWrapper_SlowQueryCounter(t *testing.T) {
	var callCount int
	hook := func(_ context.Context, _ SlowQueryInfo) {
		callCount++
	}

	w := &clickhouseWrapper{
		conn: nil,
		options: &Options{
			SlowQueryThreshold: 100 * time.Millisecond,
			SlowQueryHook:      hook,
		},
	}

	// 触发多次慢查询
	info := SlowQueryInfo{Duration: 200 * time.Millisecond}
	w.triggerSlowQueryHook(context.Background(), info)
	w.triggerSlowQueryHook(context.Background(), info)
	w.triggerSlowQueryHook(context.Background(), info)

	stats := w.Stats()
	assert.Equal(t, int64(3), stats.SlowQueries)
	assert.Equal(t, 3, callCount)
}

func TestWrapper_SlowQueryHook_ExactThreshold(t *testing.T) {
	var called bool
	hook := func(_ context.Context, _ SlowQueryInfo) {
		called = true
	}

	w := &clickhouseWrapper{
		conn: nil,
		options: &Options{
			SlowQueryThreshold: 100 * time.Millisecond,
			SlowQueryHook:      hook,
		},
	}

	// 耗时等于阈值也应该触发
	info := SlowQueryInfo{
		Duration: 100 * time.Millisecond,
	}

	_ = w.maybeSlowQuery(context.Background(), info)
	assert.True(t, called)
}

// =============================================================================
// QueryPage 测试
// =============================================================================

func TestQueryPage_EmptyQuery(t *testing.T) {
	w := &clickhouseWrapper{
		conn:    nil,
		options: defaultOptions(),
	}

	result, err := w.QueryPage(context.Background(), "", PageOptions{Page: 1, PageSize: 10})

	assert.Nil(t, result)
	assert.ErrorIs(t, err, ErrEmptyQuery)
}

func TestQueryPage_InvalidPage(t *testing.T) {
	w := &clickhouseWrapper{
		conn:    nil,
		options: defaultOptions(),
	}

	result, err := w.QueryPage(context.Background(), "SELECT * FROM users", PageOptions{Page: 0, PageSize: 10})

	assert.Nil(t, result)
	assert.ErrorIs(t, err, ErrInvalidPage)
}

func TestQueryPage_InvalidPageSize(t *testing.T) {
	w := &clickhouseWrapper{
		conn:    nil,
		options: defaultOptions(),
	}

	result, err := w.QueryPage(context.Background(), "SELECT * FROM users", PageOptions{Page: 1, PageSize: 0})

	assert.Nil(t, result)
	assert.ErrorIs(t, err, ErrInvalidPageSize)
}

// =============================================================================
// BatchInsert 测试
// =============================================================================

func TestBatchInsert_EmptyTable(t *testing.T) {
	w := &clickhouseWrapper{
		conn:    nil,
		options: defaultOptions(),
	}

	result, err := w.BatchInsert(context.Background(), "", []any{1, 2, 3}, BatchOptions{})

	assert.Nil(t, result)
	assert.ErrorIs(t, err, ErrEmptyTable)
}

func TestBatchInsert_EmptyRows(t *testing.T) {
	w := &clickhouseWrapper{
		conn:    nil,
		options: defaultOptions(),
	}

	result, err := w.BatchInsert(context.Background(), "users", []any{}, BatchOptions{})

	assert.Nil(t, result)
	assert.ErrorIs(t, err, ErrEmptyRows)
}

// =============================================================================
// 辅助函数测试
// =============================================================================

func TestValidatePageOptions(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		opts    PageOptions
		wantErr error
	}{
		{"空查询", "", PageOptions{Page: 1, PageSize: 10}, ErrEmptyQuery},
		{"无效页码", "SELECT *", PageOptions{Page: 0, PageSize: 10}, ErrInvalidPage},
		{"负数页码", "SELECT *", PageOptions{Page: -1, PageSize: 10}, ErrInvalidPage},
		{"无效页大小", "SELECT *", PageOptions{Page: 1, PageSize: 0}, ErrInvalidPageSize},
		{"负数页大小", "SELECT *", PageOptions{Page: 1, PageSize: -1}, ErrInvalidPageSize},
		{"有效参数", "SELECT *", PageOptions{Page: 1, PageSize: 10}, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePageOptions(tt.query, tt.opts)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCalculateTotalPages(t *testing.T) {
	tests := []struct {
		name     string
		total    int64
		pageSize int64
		expected int64
	}{
		{"整除", 100, 10, 10},
		{"有余数", 101, 10, 11},
		{"总数小于页大小", 5, 10, 1},
		{"总数为零", 0, 10, 0},
		{"单条记录", 1, 10, 1},
		{"页大小等于总数", 10, 10, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculateTotalPages(tt.total, tt.pageSize)
			assert.Equal(t, tt.expected, result)
		})
	}
}
