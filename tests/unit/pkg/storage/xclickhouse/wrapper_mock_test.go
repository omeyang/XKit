package xclickhouse

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Mock 测试 - 使用 mock 测试核心逻辑
// =============================================================================

func TestHealth_Success(t *testing.T) {
	conn := newMockConn()
	w := &clickhouseWrapper{
		conn:    conn,
		options: defaultOptions(),
	}

	err := w.Health(context.Background())

	assert.NoError(t, err)
	assert.Equal(t, 1, conn.pingCount)
	assert.Equal(t, int64(1), w.healthCounter.PingCount())
	assert.Equal(t, int64(0), w.healthCounter.PingErrors())
}

func TestHealth_Error(t *testing.T) {
	conn := newMockConn()
	conn.pingErr = assert.AnError
	w := &clickhouseWrapper{
		conn:    conn,
		options: defaultOptions(),
	}

	err := w.Health(context.Background())

	assert.Error(t, err)
	assert.Equal(t, 1, conn.pingCount)
	assert.Equal(t, int64(1), w.healthCounter.PingCount())
	assert.Equal(t, int64(1), w.healthCounter.PingErrors())
}

func TestHealth_WithTimeout(t *testing.T) {
	conn := newMockConn()
	opts := defaultOptions()
	opts.HealthTimeout = 5 * time.Second
	w := &clickhouseWrapper{
		conn:    conn,
		options: opts,
	}

	err := w.Health(context.Background())

	assert.NoError(t, err)
	assert.Equal(t, 1, conn.pingCount)
}

func TestClose_Success(t *testing.T) {
	conn := newMockConn()
	w := &clickhouseWrapper{
		conn:    conn,
		options: defaultOptions(),
	}

	err := w.Close()

	assert.NoError(t, err)
	assert.True(t, conn.closed)
}

func TestClose_Error(t *testing.T) {
	conn := newMockConn()
	conn.closeErr = assert.AnError
	w := &clickhouseWrapper{
		conn:    conn,
		options: defaultOptions(),
	}

	err := w.Close()

	assert.Error(t, err)
	assert.True(t, conn.closed)
}

func TestClient_ReturnsMockConn(t *testing.T) {
	conn := newMockConn()
	w := &clickhouseWrapper{
		conn:    conn,
		options: defaultOptions(),
	}

	result := w.Client()

	assert.Equal(t, conn, result)
}

func TestQueryPage_Success(t *testing.T) {
	conn := newMockConn()

	// 设置 count 查询返回值
	conn.queryRowFunc = func(_ context.Context, _ string, _ ...any) Row {
		return &mockRow{
			scanFunc: func(dest ...any) error {
				if len(dest) > 0 {
					if ptr, ok := dest[0].(*uint64); ok {
						*ptr = 100 // 总共 100 条记录
					}
				}
				return nil
			},
		}
	}

	// 设置数据查询返回值
	conn.queryFunc = func(_ context.Context, _ string, _ ...any) (Rows, error) {
		return newMockRows(
			[]string{"id", "name"},
			[][]any{
				{1, "Alice"},
				{2, "Bob"},
			},
		), nil
	}

	w := &clickhouseWrapper{
		conn:    conn,
		options: defaultOptions(),
	}

	result, err := w.QueryPage(context.Background(), "SELECT id, name FROM users", PageOptions{
		Page:     1,
		PageSize: 10,
	})

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, int64(100), result.Total)
	assert.Equal(t, int64(1), result.Page)
	assert.Equal(t, int64(10), result.PageSize)
	assert.Equal(t, int64(10), result.TotalPages)
	assert.Equal(t, []string{"id", "name"}, result.Columns)
	assert.Len(t, result.Rows, 2)
}

func TestQueryPage_CountOverflow(t *testing.T) {
	// COUNT(*) 返回 UInt64 超过 int64 上限应返回 ErrCountOverflow。
	conn := newMockConn()
	conn.queryRowFunc = func(_ context.Context, _ string, _ ...any) Row {
		return &mockRow{
			scanFunc: func(dest ...any) error {
				if len(dest) > 0 {
					if ptr, ok := dest[0].(*uint64); ok {
						*ptr = uint64(1) << 63 // > MaxInt64
					}
				}
				return nil
			},
		}
	}

	w := &clickhouseWrapper{
		conn:    conn,
		options: defaultOptions(),
	}

	result, err := w.QueryPage(context.Background(), "SELECT id FROM t", PageOptions{Page: 1, PageSize: 10})
	assert.Nil(t, result)
	assert.ErrorIs(t, err, ErrCountOverflow)
}

func TestQueryPage_CountQueryError(t *testing.T) {
	conn := newMockConn()
	conn.queryRowFunc = func(_ context.Context, _ string, _ ...any) Row {
		return &mockRow{err: assert.AnError}
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
	assert.Contains(t, err.Error(), "count query failed")
	assert.Equal(t, int64(1), w.queryCounter.QueryErrors())
}

func TestQueryPage_DataQueryError(t *testing.T) {
	conn := newMockConn()
	conn.queryRowFunc = func(_ context.Context, _ string, _ ...any) Row {
		return &mockRow{
			scanFunc: func(dest ...any) error {
				if ptr, ok := dest[0].(*uint64); ok {
					*ptr = 100
				}
				return nil
			},
		}
	}
	conn.queryFunc = func(_ context.Context, _ string, _ ...any) (Rows, error) {
		return nil, assert.AnError
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
	assert.Contains(t, err.Error(), "page query failed")
}

func TestQueryPage_ScanError(t *testing.T) {
	conn := newMockConn()
	conn.queryRowFunc = func(_ context.Context, _ string, _ ...any) Row {
		return &mockRow{
			scanFunc: func(dest ...any) error {
				if ptr, ok := dest[0].(*uint64); ok {
					*ptr = 100
				}
				return nil
			},
		}
	}
	conn.queryFunc = func(_ context.Context, _ string, _ ...any) (Rows, error) {
		rows := newMockRows([]string{"id"}, [][]any{{1}})
		rows.scanErr = assert.AnError
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
	assert.Contains(t, err.Error(), "scan failed")
}

func TestQueryPage_RowsError(t *testing.T) {
	conn := newMockConn()
	conn.queryRowFunc = func(_ context.Context, _ string, _ ...any) Row {
		return &mockRow{
			scanFunc: func(dest ...any) error {
				if ptr, ok := dest[0].(*uint64); ok {
					*ptr = 100
				}
				return nil
			},
		}
	}
	conn.queryFunc = func(_ context.Context, _ string, _ ...any) (Rows, error) {
		rows := newMockRows([]string{"id"}, [][]any{}) // 空数据
		rows.err = assert.AnError
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
	assert.Contains(t, err.Error(), "rows error")
	assert.Equal(t, int64(1), w.queryCounter.QueryErrors(), "rows.Err() 应只计一次 QueryError")
}

func TestQueryPage_SlowQueryHook(t *testing.T) {
	conn := newMockConn()
	conn.queryRowFunc = func(_ context.Context, _ string, _ ...any) Row {
		return &mockRow{
			scanFunc: func(dest ...any) error {
				if ptr, ok := dest[0].(*uint64); ok {
					*ptr = 10
				}
				// 模拟慢查询
				time.Sleep(50 * time.Millisecond)
				return nil
			},
		}
	}
	conn.queryFunc = func(_ context.Context, _ string, _ ...any) (Rows, error) {
		return newMockRows([]string{"id"}, [][]any{}), nil
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

	_, err = w.QueryPage(context.Background(), "SELECT * FROM users", PageOptions{
		Page:     1,
		PageSize: 10,
	})

	assert.NoError(t, err)
	assert.Equal(t, "SELECT * FROM users", captured.Query)
	assert.True(t, captured.Duration >= 10*time.Millisecond)
	assert.Equal(t, int64(1), w.slowQueryCounter.Count())
}

func TestBatchInsert_Success(t *testing.T) {
	conn := newMockConn()
	batchSendCount := 0
	conn.batchFunc = func(_ context.Context, _ string) Batch {
		return &mockBatch{
			sendErr: nil,
		}
	}
	// 重新设置以跟踪发送次数
	conn.batchFunc = func(_ context.Context, _ string) Batch {
		batchSendCount++
		return &mockBatch{}
	}

	w := &clickhouseWrapper{
		conn:    conn,
		options: defaultOptions(),
	}

	type testRow struct {
		ID   int
		Name string
	}

	rows := []any{
		&testRow{ID: 1, Name: "Alice"},
		&testRow{ID: 2, Name: "Bob"},
		&testRow{ID: 3, Name: "Charlie"},
	}

	result, err := w.BatchInsert(context.Background(), "users", rows, BatchOptions{
		BatchSize: 2, // 每批 2 条，总共 3 条需要 2 批
	})

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, int64(3), result.InsertedCount)
	assert.Empty(t, result.Errors)
	assert.Equal(t, 2, batchSendCount) // 应该发送 2 批
}

func TestBatchInsert_PrepareBatchError(t *testing.T) {
	conn := newMockConn()
	conn.prepareBatchErr = assert.AnError

	w := &clickhouseWrapper{
		conn:    conn,
		options: defaultOptions(),
	}

	rows := []any{struct{ ID int }{ID: 1}}

	result, err := w.BatchInsert(context.Background(), "users", rows, BatchOptions{})

	assert.Error(t, err) // BatchInsert 收集错误并返回合并的错误
	assert.NotNil(t, result)
	assert.Equal(t, int64(0), result.InsertedCount)
	assert.Len(t, result.Errors, 1)
	assert.Contains(t, result.Errors[0].Error(), "prepare batch failed")
}

func TestBatchInsert_AppendStructError(t *testing.T) {
	conn := newMockConn()
	conn.batchFunc = func(_ context.Context, _ string) Batch {
		return &mockBatch{appendErr: assert.AnError}
	}

	w := &clickhouseWrapper{
		conn:    conn,
		options: defaultOptions(),
	}

	rows := []any{struct{ ID int }{ID: 1}}

	result, err := w.BatchInsert(context.Background(), "users", rows, BatchOptions{})

	assert.Error(t, err) // BatchInsert 收集错误并返回合并的错误
	assert.NotNil(t, result)
	// 注意：appendErr 不阻止 send，所以 insertedCount 可能不准确
	assert.Len(t, result.Errors, 1)
	assert.Contains(t, result.Errors[0].Error(), "append struct failed")
}

func TestBatchInsert_SendError(t *testing.T) {
	conn := newMockConn()
	conn.batchFunc = func(_ context.Context, _ string) Batch {
		return &mockBatch{sendErr: assert.AnError}
	}

	w := &clickhouseWrapper{
		conn:    conn,
		options: defaultOptions(),
	}

	rows := []any{struct{ ID int }{ID: 1}}

	result, err := w.BatchInsert(context.Background(), "users", rows, BatchOptions{})

	assert.Error(t, err) // BatchInsert 收集错误并返回合并的错误
	assert.NotNil(t, result)
	assert.Equal(t, int64(0), result.InsertedCount)
	assert.Len(t, result.Errors, 1)
	assert.Contains(t, result.Errors[0].Error(), "send batch failed")
}

func TestBatchInsert_SendErrorAbortsBatch(t *testing.T) {
	aborted := false
	conn := newMockConn()
	conn.batchFunc = func(_ context.Context, _ string) Batch {
		return &abortTrackingBatch{
			mockBatch: &mockBatch{sendErr: assert.AnError},
			aborted:   &aborted,
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
	assert.True(t, aborted, "Send 失败后应调用 Abort")
}

func TestQueryPage_ScanErrorCountsOnce(t *testing.T) {
	conn := newMockConn()
	conn.queryRowFunc = func(_ context.Context, _ string, _ ...any) Row {
		return &mockRow{
			scanFunc: func(dest ...any) error {
				if ptr, ok := dest[0].(*uint64); ok {
					*ptr = 100
				}
				return nil
			},
		}
	}
	conn.queryFunc = func(_ context.Context, _ string, _ ...any) (Rows, error) {
		rows := newMockRows([]string{"id"}, [][]any{{1}})
		rows.scanErr = assert.AnError
		return rows, nil
	}

	w := &clickhouseWrapper{
		conn:    conn,
		options: defaultOptions(),
	}

	_, err := w.QueryPage(context.Background(), "SELECT * FROM users", PageOptions{
		Page:     1,
		PageSize: 10,
	})

	assert.Error(t, err)
	assert.Equal(t, int64(1), w.queryCounter.QueryErrors(), "Scan 失败应只计一次 QueryError")
}
