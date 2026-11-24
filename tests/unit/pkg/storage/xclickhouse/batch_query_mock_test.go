package xclickhouse

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBatchInsert_DefaultBatchSize(t *testing.T) {
	conn := newMockConn()
	batchCount := 0
	conn.batchFunc = func(_ context.Context, _ string) Batch {
		batchCount++
		return &mockBatch{}
	}

	w := &clickhouseWrapper{
		conn:    conn,
		options: defaultOptions(),
	}

	// 创建 5 条记录，不指定 BatchSize（默认 10000）
	rows := make([]any, 5)
	for i := range rows {
		rows[i] = struct{ ID int }{ID: i}
	}

	result, err := w.BatchInsert(context.Background(), "users", rows, BatchOptions{})

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 1, batchCount) // 5 条记录应该只需要 1 批
}

func TestBatchInsert_SlowQueryHook(t *testing.T) {
	conn := newMockConn()
	conn.batchFunc = func(_ context.Context, _ string) Batch {
		time.Sleep(20 * time.Millisecond)
		return &mockBatch{}
	}

	var captured SlowQueryInfo
	opts := defaultOptions()
	opts.SlowQueryThreshold = 10 * time.Millisecond
	opts.SlowQueryHook = func(_ context.Context, info SlowQueryInfo) {
		captured = info
	}

	detector, err := newSlowQueryDetector(opts)
	require.NoError(t, err)

	w := &clickhouseWrapper{
		conn:              conn,
		options:           opts,
		slowQueryDetector: detector,
	}

	rows := []any{struct{ ID int }{ID: 1}}

	_, err = w.BatchInsert(context.Background(), "users", rows, BatchOptions{})

	assert.NoError(t, err)
	assert.Contains(t, captured.Query, "INSERT INTO users")
	assert.True(t, captured.Duration >= 10*time.Millisecond)
}

func TestNew_Success(t *testing.T) {
	conn := newMockConn()

	ch, err := New(conn)

	assert.NoError(t, err)
	assert.NotNil(t, ch)
	assert.Equal(t, conn, ch.Client())
}

func TestNew_WithAllOptions(t *testing.T) {
	conn := newMockConn()
	var hookCalled bool

	ch, err := New(conn,
		WithHealthTimeout(10*time.Second),
		WithSlowQueryThreshold(100*time.Millisecond),
		WithSlowQueryHook(func(_ context.Context, _ SlowQueryInfo) {
			hookCalled = true
		}),
	)

	assert.NoError(t, err)
	assert.NotNil(t, ch)

	// 验证选项已应用 - 通过触发慢查询钩子（需要 Duration >= Threshold）
	wrapper, ok := ch.(*clickhouseWrapper)
	assert.True(t, ok)
	wrapper.maybeSlowQuery(context.Background(), SlowQueryInfo{Duration: 200 * time.Millisecond})
	assert.True(t, hookCalled)
}

func TestStats_AfterOperations(t *testing.T) {
	conn := newMockConn()
	conn.queryRowFunc = func(_ context.Context, _ string, _ ...any) Row {
		return &mockRow{
			scanFunc: func(dest ...any) error {
				if ptr, ok := dest[0].(*uint64); ok {
					*ptr = 10
				}
				return nil
			},
		}
	}
	conn.queryFunc = func(_ context.Context, _ string, _ ...any) (Rows, error) {
		return newMockRows([]string{"id"}, [][]any{}), nil
	}

	w := &clickhouseWrapper{
		conn:    conn,
		options: defaultOptions(),
	}

	// 执行一些操作来增加统计计数
	err := w.Health(context.Background())
	assert.NoError(t, err)
	err = w.Health(context.Background())
	assert.NoError(t, err)
	conn.pingErr = assert.AnError
	err = w.Health(context.Background())
	assert.Error(t, err)

	_, err = w.QueryPage(context.Background(), "SELECT * FROM t", PageOptions{Page: 1, PageSize: 10})
	assert.NoError(t, err)

	stats := w.Stats()

	assert.Equal(t, int64(3), stats.PingCount)
	assert.Equal(t, int64(1), stats.PingErrors)
	assert.Equal(t, int64(2), stats.QueryCount) // QueryPage 执行 2 次查询：count + page
	assert.Equal(t, int64(0), stats.QueryErrors)
}

func TestStats_Pool_WithConn(t *testing.T) {
	conn := newMockConn()
	conn.stats = driver.Stats{Open: 10, Idle: 3}
	w := &clickhouseWrapper{
		conn:    conn,
		options: defaultOptions(),
	}

	stats := w.Stats().Pool

	assert.Equal(t, 10, stats.Open)
	assert.Equal(t, 3, stats.Idle)
	assert.Equal(t, 7, stats.InUse) // Open - Idle
}

func TestBatchInsert_ContextCanceled(t *testing.T) {
	conn := newMockConn()
	batchCount := 0
	conn.batchFunc = func(_ context.Context, _ string) Batch {
		batchCount++
		return &mockBatch{}
	}

	w := &clickhouseWrapper{
		conn:    conn,
		options: defaultOptions(),
	}

	// 创建 10 条记录，每批 2 条
	rows := make([]any, 10)
	for i := range rows {
		rows[i] = struct{ ID int }{ID: i}
	}

	// 使用已取消的 context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := w.BatchInsert(ctx, "users", rows, BatchOptions{BatchSize: 2})

	assert.Error(t, err)
	assert.NotNil(t, result)
	assert.Contains(t, err.Error(), "context canceled")
	// 第一批就应该因 context 取消而中止
	assert.Equal(t, 0, batchCount)
}

func TestBatchInsert_AbortBatchError(t *testing.T) {
	conn := newMockConn()
	conn.batchFunc = func(_ context.Context, _ string) Batch {
		return &mockBatch{
			appendErr: assert.AnError,
			abortErr:  assert.AnError,
		}
	}

	w := &clickhouseWrapper{
		conn:    conn,
		options: defaultOptions(),
	}

	rows := []any{struct{ ID int }{ID: 1}}

	result, err := w.BatchInsert(context.Background(), "users", rows, BatchOptions{})

	assert.Error(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, int64(0), result.InsertedCount)
	// 应包含 append 错误和 abort 错误
	assert.GreaterOrEqual(t, len(result.Errors), 2)
}

func TestQueryPage_RowsCloseError(t *testing.T) {
	conn := newMockConn()
	conn.queryRowFunc = func(_ context.Context, _ string, _ ...any) Row {
		return &mockRow{
			scanFunc: func(dest ...any) error {
				if ptr, ok := dest[0].(*uint64); ok {
					*ptr = 10
				}
				return nil
			},
		}
	}
	conn.queryFunc = func(_ context.Context, _ string, _ ...any) (Rows, error) {
		rows := newMockRows([]string{"id"}, [][]any{})
		rows.closeErr = assert.AnError
		return rows, nil
	}

	w := &clickhouseWrapper{
		conn:    conn,
		options: defaultOptions(),
	}

	result, err := w.QueryPage(context.Background(), "SELECT * FROM users", PageOptions{
		Page:     1,
		PageSize: 10,
	})

	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "close rows failed")
}

func TestClose_WithSlowQueryDetector(t *testing.T) {
	conn := newMockConn()
	opts := defaultOptions()
	opts.SlowQueryThreshold = 100 * time.Millisecond
	opts.SlowQueryHook = func(_ context.Context, _ SlowQueryInfo) {}

	detector, err := newSlowQueryDetector(opts)
	require.NoError(t, err)

	w := &clickhouseWrapper{
		conn:              conn,
		options:           opts,
		slowQueryDetector: detector,
	}

	err = w.Close()

	assert.NoError(t, err)
	assert.True(t, conn.closed)
}

func TestQueryPage_PageOverflow(t *testing.T) {
	w := &clickhouseWrapper{
		conn:    nil,
		options: defaultOptions(),
	}

	result, err := w.QueryPage(context.Background(), "SELECT * FROM users", PageOptions{
		Page:     1<<62 + 1,
		PageSize: 1<<62 + 1,
	})

	assert.Nil(t, result)
	assert.ErrorIs(t, err, ErrPageOverflow)
}

func TestClose_Idempotent_WithConn(t *testing.T) {
	conn := newMockConn()
	w := &clickhouseWrapper{
		conn:    conn,
		options: defaultOptions(),
	}

	// 第一次关闭成功
	err := w.Close()
	assert.NoError(t, err)
	assert.True(t, conn.closed)

	// 第二次关闭返回 ErrClosed
	err = w.Close()
	assert.ErrorIs(t, err, ErrClosed)
}

func TestHealth_AfterClose(t *testing.T) {
	conn := newMockConn()
	w := &clickhouseWrapper{
		conn:    conn,
		options: defaultOptions(),
	}

	// 关闭后调用 Health 应返回 ErrClosed
	err := w.Close()
	assert.NoError(t, err)

	err = w.Health(context.Background())
	assert.ErrorIs(t, err, ErrClosed)
}

func TestQueryPage_AfterClose(t *testing.T) {
	conn := newMockConn()
	w := &clickhouseWrapper{
		conn:    conn,
		options: defaultOptions(),
	}

	err := w.Close()
	assert.NoError(t, err)

	result, err := w.QueryPage(context.Background(), "SELECT * FROM users", PageOptions{
		Page:     1,
		PageSize: 10,
	})
	assert.Nil(t, result)
	assert.ErrorIs(t, err, ErrClosed)
}

func TestBatchInsert_AfterClose(t *testing.T) {
	conn := newMockConn()
	w := &clickhouseWrapper{
		conn:    conn,
		options: defaultOptions(),
	}

	err := w.Close()
	assert.NoError(t, err)

	result, err := w.BatchInsert(context.Background(), "users", []any{struct{ ID int }{ID: 1}}, BatchOptions{})
	assert.Nil(t, result)
	assert.ErrorIs(t, err, ErrClosed)
}

func TestBatchInsert_BatchSizeTooLarge(t *testing.T) {
	conn := newMockConn()
	w := &clickhouseWrapper{
		conn:    conn,
		options: defaultOptions(),
	}

	rows := []any{struct{ ID int }{ID: 1}}

	result, err := w.BatchInsert(context.Background(), "users", rows, BatchOptions{
		BatchSize: MaxBatchSize + 1,
	})

	assert.Nil(t, result)
	assert.ErrorIs(t, err, ErrBatchSizeTooLarge)
}

func TestBatchInsert_BatchSizeAtMax(t *testing.T) {
	conn := newMockConn()
	conn.batchFunc = func(_ context.Context, _ string) Batch {
		return &mockBatch{}
	}

	w := &clickhouseWrapper{
		conn:    conn,
		options: defaultOptions(),
	}

	rows := []any{struct{ ID int }{ID: 1}}

	// MaxBatchSize 正好等于限制值时应该通过
	result, err := w.BatchInsert(context.Background(), "users", rows, BatchOptions{
		BatchSize: MaxBatchSize,
	})
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, int64(1), result.InsertedCount)
}

func TestBatchInsert_ContextCanceledDuringAppend(t *testing.T) {
	// 测试: context 取消后应该 abort 而非 send
	conn := newMockConn()
	ctx, cancel := context.WithCancel(context.Background())

	conn.batchFunc = func(_ context.Context, _ string) Batch {
		return &mockBatch{}
	}

	w := &clickhouseWrapper{
		conn:    conn,
		options: defaultOptions(),
	}

	// 创建 200 条记录（足够触发 ctx 检查）
	rows := make([]any, 200)
	for i := range rows {
		rows[i] = struct{ ID int }{ID: i}
	}

	cancel()

	result, err := w.BatchInsert(ctx, "users", rows, BatchOptions{BatchSize: 200})

	assert.Error(t, err)
	assert.NotNil(t, result)
	assert.Contains(t, err.Error(), "context canceled")
}

func TestBatchInsert_ContextCanceledBeforeSend(t *testing.T) {
	// 测试: append 成功后、send 前 context 取消，应该 abort 而非 send
	conn := newMockConn()
	ctx, cancel := context.WithCancel(context.Background())
	appendCallCount := 0

	conn.batchFunc = func(_ context.Context, _ string) Batch {
		return &mockBatch{
			// 自定义 appendStruct 来在第 101 次 append 后取消 context
		}
	}

	// 使用自定义 batch 来在 append 过程中取消 context
	conn.batchFunc = func(_ context.Context, _ string) Batch {
		return &cancelOnAppendBatch{
			cancel:          cancel,
			cancelAfterRows: 100,
			appendCount:     &appendCallCount,
		}
	}

	w := &clickhouseWrapper{
		conn:    conn,
		options: defaultOptions(),
	}

	// 创建 200 条记录
	rows := make([]any, 200)
	for i := range rows {
		rows[i] = struct{ ID int }{ID: i}
	}

	result, err := w.BatchInsert(ctx, "users", rows, BatchOptions{BatchSize: 200})

	assert.Error(t, err)
	assert.NotNil(t, result)
	// 应包含 "context canceled before send" 或 "context canceled during append"
	assert.Contains(t, err.Error(), "context canceled")
	// InsertedCount 应该为 0（因为 abort 了）
	assert.Equal(t, int64(0), result.InsertedCount)
}

func TestBatchInsert_AppendErrorAbortsBatch(t *testing.T) {
	// 批次原子性: 任一 AppendStruct 错误应立即中止整批，不允许部分写入。
	conn := newMockConn()
	conn.batchFunc = func(_ context.Context, _ string) Batch {
		return &mockBatch{appendErr: assert.AnError}
	}

	w := &clickhouseWrapper{
		conn:    conn,
		options: defaultOptions(),
	}

	rows := make([]any, 200)
	for i := range rows {
		rows[i] = struct{ ID int }{ID: i}
	}

	result, err := w.BatchInsert(context.Background(), "users", rows, BatchOptions{BatchSize: 200})

	assert.Error(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, int64(0), result.InsertedCount)
	// 仅应包含 1 条 append 错误（遇到第一个错误即 short-circuit）
	assert.Len(t, result.Errors, 1)
	found := false
	for _, e := range result.Errors {
		if e != nil && strings.Contains(e.Error(), "append struct failed") {
			found = true
			break
		}
	}
	assert.True(t, found, "应包含 'append struct failed' 错误")
}
