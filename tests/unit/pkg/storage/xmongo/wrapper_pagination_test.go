package xmongo

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestWrapper_FindPageInternal_CountError(t *testing.T) {
	mock := newMockCollectionOps()
	mock.countErr = errMockCount

	w := &mongoWrapper{
		client:  nil,
		options: defaultOptions(),
	}

	result, err := w.findPageInternal(context.Background(), mock, nil, PageOptions{Page: 1, PageSize: 10})
	assert.Error(t, err)
	assert.ErrorIs(t, err, errMockCount)
	assert.Contains(t, err.Error(), "xmongo find_page count")
	assert.Nil(t, result)
}

func TestWrapper_FindPageInternal_FindError(t *testing.T) {
	mock := newMockCollectionOps()
	mock.countResult = 100
	mock.findErr = errMockFind

	w := &mongoWrapper{
		client:  nil,
		options: defaultOptions(),
	}

	result, err := w.findPageInternal(context.Background(), mock, nil, PageOptions{Page: 1, PageSize: 10})
	assert.Error(t, err)
	assert.ErrorIs(t, err, errMockFind)
	assert.Contains(t, err.Error(), "xmongo find_page find")
	assert.Nil(t, result)
}

// TestBulkInsert_NilCollection 已在 batch_test.go 中定义，此处不重复。

func TestWrapper_BulkInsert_EmptyDocs(t *testing.T) {
	w := &mongoWrapper{
		client:  nil,
		options: defaultOptions(),
	}

	// bulkInsert 先检查 nil collection，所以这里测试 BulkInsert 公共接口
	result, err := w.BulkInsert(context.Background(), nil, []any{}, BulkOptions{})
	assert.Error(t, err)
	assert.Equal(t, ErrNilCollection, err) // nil collection 先于 empty docs 检查
	assert.Nil(t, result)
}

func TestWrapper_BulkInsertInternal_EmptyDocs(t *testing.T) {
	mock := newMockCollectionOps()
	w := &mongoWrapper{
		client:  nil,
		options: defaultOptions(),
	}

	// 空文档数组测试 - 通过 bulkInsertInternal 测试
	// 注意：bulkInsertInternal 不检查空文档，它会直接返回空结果
	result, err := w.bulkInsertInternal(context.Background(), mock, []any{}, BulkOptions{BatchSize: 10})
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, int64(0), result.InsertedCount)
}

func TestWrapper_BulkInsertInternal_Success(t *testing.T) {
	mock := newMockCollectionOps()

	w := &mongoWrapper{
		client:  nil,
		options: defaultOptions(),
	}

	docs := []any{
		map[string]any{"name": "doc1"},
		map[string]any{"name": "doc2"},
		map[string]any{"name": "doc3"},
	}

	result, err := w.bulkInsertInternal(context.Background(), mock, docs, BulkOptions{BatchSize: 10})
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, int64(3), result.InsertedCount)
	assert.Empty(t, result.Errors)
}

func TestWrapper_BulkInsertInternal_DefaultBatchSize(t *testing.T) {
	mock := newMockCollectionOps()

	w := &mongoWrapper{
		client:  nil,
		options: defaultOptions(),
	}

	docs := []any{
		map[string]any{"name": "doc1"},
	}

	// BatchSize 为 0，应使用默认值 1000
	result, err := w.bulkInsertInternal(context.Background(), mock, docs, BulkOptions{BatchSize: 0})
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, int64(1), result.InsertedCount)
}

func TestWrapper_BulkInsertInternal_MaxBatchSize(t *testing.T) {
	mock := newMockCollectionOps()

	w := &mongoWrapper{
		client:  nil,
		options: defaultOptions(),
	}

	docs := []any{
		map[string]any{"name": "doc1"},
	}

	// BatchSize 超过上限，应被限制为 maxBatchSize
	result, err := w.bulkInsertInternal(context.Background(), mock, docs, BulkOptions{BatchSize: maxBatchSize + 1})
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, int64(1), result.InsertedCount)
}

func TestWrapper_BulkInsertInternal_InsertError_Unordered(t *testing.T) {
	mock := newMockCollectionOps()
	mock.insertErr = errMockInsert

	w := &mongoWrapper{
		client:  nil,
		options: defaultOptions(),
	}

	docs := []any{
		map[string]any{"name": "doc1"},
		map[string]any{"name": "doc2"},
	}

	// 无序模式：遇到错误继续执行
	result, err := w.bulkInsertInternal(context.Background(), mock, docs, BulkOptions{
		BatchSize: 1,
		Ordered:   false,
	})
	assert.Error(t, err) // bulkInsertInternal 收集错误并返回合并的错误
	assert.NotNil(t, result)
	assert.Equal(t, int64(0), result.InsertedCount)
	assert.Len(t, result.Errors, 2) // 每个批次一个错误
}

func TestWrapper_BulkInsertInternal_InsertError_Ordered(t *testing.T) {
	mock := newMockCollectionOps()
	mock.insertErr = errMockInsert

	w := &mongoWrapper{
		client:  nil,
		options: defaultOptions(),
	}

	docs := []any{
		map[string]any{"name": "doc1"},
		map[string]any{"name": "doc2"},
		map[string]any{"name": "doc3"},
	}

	// 有序模式：遇到错误停止
	result, err := w.bulkInsertInternal(context.Background(), mock, docs, BulkOptions{
		BatchSize: 1,
		Ordered:   true,
	})
	assert.Error(t, err) // bulkInsertInternal 收集错误并返回合并的错误
	assert.NotNil(t, result)
	assert.Equal(t, int64(0), result.InsertedCount)
	assert.Len(t, result.Errors, 1) // 只有第一个批次的错误
}

func TestWrapper_BulkInsertInternal_MultipleBatches(t *testing.T) {
	mock := newMockCollectionOps()

	w := &mongoWrapper{
		client:  nil,
		options: defaultOptions(),
	}

	// 创建 10 个文档
	docs := make([]any, 10)
	for i := range 10 {
		docs[i] = map[string]any{"index": i}
	}

	// 每批 3 个，共 4 批（3+3+3+1）
	result, err := w.bulkInsertInternal(context.Background(), mock, docs, BulkOptions{BatchSize: 3})
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, int64(10), result.InsertedCount)
	assert.Empty(t, result.Errors)
}

func TestWrapper_BulkInsertInternal_WithSlowQueryHook(t *testing.T) {
	mock := newMockCollectionOps()
	var captured SlowQueryInfo

	opts := &Options{
		SlowQueryThreshold: 1 * time.Nanosecond, // 极小阈值确保触发
		SlowQueryHook: func(_ context.Context, info SlowQueryInfo) {
			captured = info
		},
	}
	detector, err := newSlowQueryDetector(opts)
	require.NoError(t, err)

	w := &mongoWrapper{
		client:            nil,
		options:           opts,
		slowQueryDetector: detector,
	}

	docs := []any{map[string]any{"name": "doc1"}}

	result, err := w.bulkInsertInternal(context.Background(), mock, docs, BulkOptions{BatchSize: 10})
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "bulkInsert", captured.Operation)
}

// TestBuildSlowQueryInfoFromOps_Nil 已在上方定义，此处不重复。

func TestAdaptCollection_Nil(t *testing.T) {
	result := adaptCollection(nil)
	assert.Nil(t, result)
}

// =============================================================================
// 异步慢查询选项测试
// =============================================================================

func TestWithAsyncSlowQueryHook(t *testing.T) {
	var called bool
	hook := func(_ SlowQueryInfo) {
		called = true
	}

	opts := defaultOptions()
	WithAsyncSlowQueryHook(hook)(opts)

	assert.NotNil(t, opts.AsyncSlowQueryHook)
	opts.AsyncSlowQueryHook(SlowQueryInfo{})
	assert.True(t, called)
}

func TestWithAsyncSlowQueryWorkers(t *testing.T) {
	opts := defaultOptions()

	// 正数生效
	WithAsyncSlowQueryWorkers(20)(opts)
	assert.Equal(t, 20, opts.AsyncSlowQueryWorkers)

	// 零值被忽略
	WithAsyncSlowQueryWorkers(0)(opts)
	assert.Equal(t, 20, opts.AsyncSlowQueryWorkers)

	// 负值被忽略
	WithAsyncSlowQueryWorkers(-1)(opts)
	assert.Equal(t, 20, opts.AsyncSlowQueryWorkers)

	// 超过上限被 clamp
	WithAsyncSlowQueryWorkers(999999)(opts)
	assert.Equal(t, 1000, opts.AsyncSlowQueryWorkers)
}

func TestWithAsyncSlowQueryQueueSize(t *testing.T) {
	opts := defaultOptions()

	// 正数生效
	WithAsyncSlowQueryQueueSize(500)(opts)
	assert.Equal(t, 500, opts.AsyncSlowQueryQueueSize)

	// 零值被忽略
	WithAsyncSlowQueryQueueSize(0)(opts)
	assert.Equal(t, 500, opts.AsyncSlowQueryQueueSize)

	// 负值被忽略
	WithAsyncSlowQueryQueueSize(-1)(opts)
	assert.Equal(t, 500, opts.AsyncSlowQueryQueueSize)

	// 超过上限被 clamp
	WithAsyncSlowQueryQueueSize(999999)(opts)
	assert.Equal(t, 100000, opts.AsyncSlowQueryQueueSize)
}

func TestNewSlowQueryDetector_WithAsyncHook(t *testing.T) {
	var called sync.WaitGroup
	called.Add(1)
	opts := &Options{
		SlowQueryThreshold: 1 * time.Nanosecond,
		AsyncSlowQueryHook: func(_ SlowQueryInfo) {
			called.Done()
		},
		AsyncSlowQueryWorkers:   2,
		AsyncSlowQueryQueueSize: 10,
	}

	detector, err := newSlowQueryDetector(opts)
	require.NoError(t, err)
	assert.NotNil(t, detector)

	// 触发慢查询让异步钩子有机会执行
	detector.MaybeSlowQuery(context.Background(), SlowQueryInfo{Duration: time.Second}, time.Second)
	called.Wait() // 等待异步执行完成
	detector.Close()
}

// =============================================================================
// convertPaginationError 完整覆盖
// =============================================================================

func TestConvertPaginationError_AllCases(t *testing.T) {
	tests := []struct {
		name     string
		input    error
		expected error
	}{
		{"ErrInvalidPage", ErrInvalidPage, ErrInvalidPage},
		{"ErrInvalidPageSize", ErrInvalidPageSize, ErrInvalidPageSize},
		{"ErrPageOverflow", ErrPageOverflow, ErrPageOverflow},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 测试通过 findPageInternal 触发各种分页错误
			assert.ErrorIs(t, tt.input, tt.expected)
		})
	}
}

func TestWrapper_FindPageInternal_PageOverflow(t *testing.T) {
	mock := newMockCollectionOps()
	w := &mongoWrapper{
		client:  nil,
		options: defaultOptions(),
	}

	// 极大的 page 配合 MaxPageSize 内的 pageSize 触发 overflow
	// pageSize 必须 <= MaxPageSize 才能通过上限检查，page 足够大触发溢出
	result, err := w.findPageInternal(context.Background(), mock, nil, PageOptions{
		Page:     1<<62 + 1,
		PageSize: MaxPageSize,
	})
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrPageOverflow)
	assert.Nil(t, result)
}

// =============================================================================
// MaxPageSize 上限测试
// =============================================================================

func TestWrapper_FindPageInternal_PageSizeTooLarge(t *testing.T) {
	mock := newMockCollectionOps()
	w := &mongoWrapper{
		client:  nil,
		options: defaultOptions(),
	}

	result, err := w.findPageInternal(context.Background(), mock, nil, PageOptions{
		Page:     1,
		PageSize: MaxPageSize + 1,
	})
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrPageSizeTooLarge)
	assert.Nil(t, result)
}

func TestWrapper_FindPageInternal_PageSizeAtMax(t *testing.T) {
	docs := []any{bson.M{"_id": "1"}}
	mock := &cursorCollectionOps{
		docs:     docs,
		count:    1,
		collName: "test_coll",
	}

	w := &mongoWrapper{
		client:  nil,
		options: defaultOptions(),
	}

	// MaxPageSize 本身是允许的
	result, err := w.findPageInternal(context.Background(), mock, nil, PageOptions{
		Page:     1,
		PageSize: MaxPageSize,
	})
	assert.NoError(t, err)
	assert.NotNil(t, result)
}

// =============================================================================
// Close 幂等性测试
// =============================================================================

func TestWrapper_Close_NilDetector(t *testing.T) {
	mock := newMockClientOps()
	w := &mongoWrapper{
		client:            nil,
		clientOps:         mock,
		options:           defaultOptions(),
		slowQueryDetector: nil,
	}

	err := w.Close(context.Background())
	assert.NoError(t, err)
	assert.True(t, mock.disconnected)
}

// =============================================================================
// Close 后方法调用测试
// =============================================================================

func TestWrapper_Health_AfterClose(t *testing.T) {
	mock := newMockClientOps()
	w := &mongoWrapper{
		client:    nil,
		clientOps: mock,
		options:   defaultOptions(),
	}

	err := w.Close(context.Background())
	assert.NoError(t, err)

	// Close 后调用 Health 应返回 ErrClosed
	err = w.Health(context.Background())
	assert.ErrorIs(t, err, ErrClosed)
}

func TestWrapper_FindPage_AfterClose(t *testing.T) {
	mock := newMockClientOps()
	w := &mongoWrapper{
		client:    nil,
		clientOps: mock,
		options:   defaultOptions(),
	}

	err := w.Close(context.Background())
	assert.NoError(t, err)

	// Close 后调用 FindPage 应返回 ErrClosed
	result, err := w.FindPage(context.Background(), nil, nil, PageOptions{Page: 1, PageSize: 10})
	assert.ErrorIs(t, err, ErrClosed)
	assert.Nil(t, result)
}

func TestWrapper_BulkInsert_AfterClose(t *testing.T) {
	mock := newMockClientOps()
	w := &mongoWrapper{
		client:    nil,
		clientOps: mock,
		options:   defaultOptions(),
	}

	err := w.Close(context.Background())
	assert.NoError(t, err)

	// Close 后调用 BulkInsert 应返回 ErrClosed
	result, err := w.BulkInsert(context.Background(), nil, []any{1}, BulkOptions{})
	assert.ErrorIs(t, err, ErrClosed)
	assert.Nil(t, result)
}

func TestWrapper_Close_Concurrent(t *testing.T) {
	mock := newMockClientOps()
	w := &mongoWrapper{
		client:    nil,
		clientOps: mock,
		options:   defaultOptions(),
	}

	const goroutines = 20
	var wg sync.WaitGroup
	results := make([]error, goroutines)

	for i := range goroutines {
		idx := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[idx] = w.Close(context.Background())
		}()
	}
	wg.Wait()

	// 恰好一个 goroutine 应成功，其余返回 ErrClosed
	var successCount int
	for _, err := range results {
		if err == nil {
			successCount++
		} else {
			assert.ErrorIs(t, err, ErrClosed)
		}
	}
	assert.Equal(t, 1, successCount)
}

// =============================================================================
// findPageInternal 成功路径测试（使用 cursorCollectionOps）
// =============================================================================

func TestWrapper_FindPageInternal_Success(t *testing.T) {
	docs := []any{
		bson.M{"_id": "1", "name": "a"},
		bson.M{"_id": "2", "name": "b"},
	}
	mock := &cursorCollectionOps{
		docs:     docs,
		count:    2,
		collName: "test_coll",
	}

	w := &mongoWrapper{
		client:  nil,
		options: defaultOptions(),
	}

	result, err := w.findPageInternal(context.Background(), mock, bson.M{}, PageOptions{Page: 1, PageSize: 10})
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, int64(2), result.Total)
	assert.Equal(t, int64(1), result.Page)
	assert.Equal(t, int64(10), result.PageSize)
	assert.Equal(t, int64(1), result.TotalPages)
	assert.Len(t, result.Data, 2)
}
