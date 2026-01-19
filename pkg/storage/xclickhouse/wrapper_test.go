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

	opts := &Options{
		HealthTimeout:      5 * time.Second,
		SlowQueryThreshold: 100 * time.Millisecond,
		SlowQueryHook:      hook,
	}

	w := &clickhouseWrapper{
		conn:              nil,
		options:           opts,
		slowQueryDetector: newSlowQueryDetector(opts),
	}

	// 模拟慢查询触发
	info := SlowQueryInfo{
		Query:    "SELECT * FROM users",
		Args:     []any{1, "test"},
		Duration: 200 * time.Millisecond,
	}

	w.maybeSlowQuery(context.Background(), info)

	assert.Equal(t, "SELECT * FROM users", captured.Query)
	assert.Len(t, captured.Args, 2)
	assert.Equal(t, 200*time.Millisecond, captured.Duration)
}

func TestWrapper_SlowQueryHook_NilHook(t *testing.T) {
	opts := &Options{
		HealthTimeout:      5 * time.Second,
		SlowQueryThreshold: 100 * time.Millisecond,
		SlowQueryHook:      nil, // 无钩子
	}

	w := &clickhouseWrapper{
		conn:              nil,
		options:           opts,
		slowQueryDetector: newSlowQueryDetector(opts),
	}

	info := SlowQueryInfo{
		Query:    "SELECT * FROM users",
		Duration: 200 * time.Millisecond,
	}

	// 不应该 panic
	w.maybeSlowQuery(context.Background(), info)
}

func TestWrapper_SlowQueryHook_BelowThreshold(t *testing.T) {
	var called bool
	hook := func(_ context.Context, _ SlowQueryInfo) {
		called = true
	}

	opts := &Options{
		HealthTimeout:      5 * time.Second,
		SlowQueryThreshold: 100 * time.Millisecond,
		SlowQueryHook:      hook,
	}

	w := &clickhouseWrapper{
		conn:              nil,
		options:           opts,
		slowQueryDetector: newSlowQueryDetector(opts),
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

	opts := &Options{
		HealthTimeout:      5 * time.Second,
		SlowQueryThreshold: 100 * time.Millisecond,
		SlowQueryHook:      hook,
	}
	w := &clickhouseWrapper{
		conn:              nil,
		options:           opts,
		slowQueryDetector: newSlowQueryDetector(opts),
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

	opts := &Options{
		HealthTimeout:      5 * time.Second,
		SlowQueryThreshold: 0, // 禁用
		SlowQueryHook:      hook,
	}
	w := &clickhouseWrapper{
		conn:              nil,
		options:           opts,
		slowQueryDetector: newSlowQueryDetector(opts),
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

	opts := &Options{
		SlowQueryThreshold: 100 * time.Millisecond,
		SlowQueryHook:      hook,
	}

	w := &clickhouseWrapper{
		conn:              nil,
		options:           opts,
		slowQueryDetector: newSlowQueryDetector(opts),
	}

	// 触发多次慢查询
	info := SlowQueryInfo{Duration: 200 * time.Millisecond}
	w.maybeSlowQuery(context.Background(), info)
	w.maybeSlowQuery(context.Background(), info)
	w.maybeSlowQuery(context.Background(), info)

	stats := w.Stats()
	assert.Equal(t, int64(3), stats.SlowQueries)
	assert.Equal(t, 3, callCount)
}

func TestWrapper_SlowQueryHook_ExactThreshold(t *testing.T) {
	var called bool
	hook := func(_ context.Context, _ SlowQueryInfo) {
		called = true
	}

	opts := &Options{
		SlowQueryThreshold: 100 * time.Millisecond,
		SlowQueryHook:      hook,
	}

	w := &clickhouseWrapper{
		conn:              nil,
		options:           opts,
		slowQueryDetector: newSlowQueryDetector(opts),
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
		name           string
		query          string
		opts           PageOptions
		wantErr        error
		wantNormalized string
	}{
		{"空查询", "", PageOptions{Page: 1, PageSize: 10}, ErrEmptyQuery, ""},
		{"无效页码", "SELECT *", PageOptions{Page: 0, PageSize: 10}, ErrInvalidPage, ""},
		{"负数页码", "SELECT *", PageOptions{Page: -1, PageSize: 10}, ErrInvalidPage, ""},
		{"无效页大小", "SELECT *", PageOptions{Page: 1, PageSize: 0}, ErrInvalidPageSize, ""},
		{"负数页大小", "SELECT *", PageOptions{Page: 1, PageSize: -1}, ErrInvalidPageSize, ""},
		{"有效参数", "SELECT *", PageOptions{Page: 1, PageSize: 10}, nil, "SELECT *"},
		{"末尾分号", "SELECT * FROM users;", PageOptions{Page: 1, PageSize: 10}, nil, "SELECT * FROM users"},
		{"末尾多个分号和空白", "SELECT * ; ; ", PageOptions{Page: 1, PageSize: 10}, nil, "SELECT *"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			normalized, _, err := validatePageOptions(tt.query, tt.opts)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				assert.Empty(t, normalized)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantNormalized, normalized)
			}
		})
	}
}

// =============================================================================
// SQL 规范化和校验测试
// =============================================================================

func TestNormalizeQuery(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		expected string
	}{
		{"无变化", "SELECT * FROM users", "SELECT * FROM users"},
		{"去除末尾分号", "SELECT * FROM users;", "SELECT * FROM users"},
		{"去除末尾空格", "SELECT * FROM users   ", "SELECT * FROM users"},
		{"去除末尾换行", "SELECT * FROM users\n", "SELECT * FROM users"},
		{"去除多个末尾字符", "SELECT * ; ; \n \t ", "SELECT *"},
		{"空字符串", "", ""},
		{"只有分号", ";;;", ""},
		{"只有空白", "   \t\n", ""},
		{"保留中间分号", "SELECT ';' FROM users", "SELECT ';' FROM users"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeQuery(tt.query)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidateQuerySyntax(t *testing.T) {
	tests := []struct {
		name           string
		query          string
		wantErr        error
		wantNormalized string
	}{
		{"正常查询", "SELECT * FROM users", nil, "SELECT * FROM users"},
		{"带分号的正常查询", "SELECT * FROM users;", nil, "SELECT * FROM users"},
		{"空查询", "", ErrEmptyQuery, ""},
		{"只有空白", "   ", ErrEmptyQuery, ""},
		{"包含 FORMAT", "SELECT * FROM users FORMAT JSON", ErrQueryContainsFormat, ""},
		{"包含小写 format", "SELECT * FROM users format TabSeparated", ErrQueryContainsFormat, ""},
		{"包含 SETTINGS", "SELECT * FROM users SETTINGS max_threads=4", ErrQueryContainsSettings, ""},
		{"包含小写 settings", "SELECT * FROM users settings enable_optimize=1", ErrQueryContainsSettings, ""},
		// 已知限制：正则使用 \b 单词边界，无法区分 SQL 关键字和字符串常量中的同名词
		// 实际使用中很少在字符串常量中使用 FORMAT/SETTINGS 作为关键字
		{"FORMAT 在字符串中", "SELECT * FROM users WHERE name LIKE '%FORMAT%'", ErrQueryContainsFormat, ""},
		{"FORMATTER 不匹配", "SELECT * FROM users WHERE type = 'FORMATTER'", nil, "SELECT * FROM users WHERE type = 'FORMATTER'"},
		{"SETTINGS_KEY 不匹配", "SELECT SETTINGS_KEY FROM config", nil, "SELECT SETTINGS_KEY FROM config"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			normalized, err := validateQuerySyntax(tt.query)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				assert.Empty(t, normalized)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantNormalized, normalized)
			}
		})
	}
}

func TestQueryPage_ContainsFormat(t *testing.T) {
	w := &clickhouseWrapper{
		conn:    nil,
		options: defaultOptions(),
	}

	result, err := w.QueryPage(context.Background(),
		"SELECT * FROM users FORMAT JSON",
		PageOptions{Page: 1, PageSize: 10})

	assert.Nil(t, result)
	assert.ErrorIs(t, err, ErrQueryContainsFormat)
}

func TestQueryPage_ContainsSettings(t *testing.T) {
	w := &clickhouseWrapper{
		conn:    nil,
		options: defaultOptions(),
	}

	result, err := w.QueryPage(context.Background(),
		"SELECT * FROM users SETTINGS max_threads=4",
		PageOptions{Page: 1, PageSize: 10})

	assert.Nil(t, result)
	assert.ErrorIs(t, err, ErrQueryContainsSettings)
}

func TestQueryPage_NormalizesTrailingSemicolon(t *testing.T) {
	// 此测试验证末尾分号被正确去除
	// 通过直接调用 validatePageOptions 验证

	// 带分号的查询应该通过校验
	normalized, _, err := validatePageOptions("SELECT * FROM users;", PageOptions{Page: 1, PageSize: 10})

	// 校验应该通过
	assert.NoError(t, err)
	// 规范化后应该去除末尾分号
	assert.Equal(t, "SELECT * FROM users", normalized)
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
