package xmongo

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func TestWrapper_FindPageInternal_WithSlowQuery(t *testing.T) {
	docs := []any{bson.M{"_id": "1"}}
	mock := &cursorCollectionOps{
		docs:     docs,
		count:    1,
		collName: "test_coll",
	}

	var captured SlowQueryInfo
	opts := &Options{
		SlowQueryThreshold: 1 * time.Nanosecond,
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

	result, err := w.findPageInternal(context.Background(), mock, bson.M{}, PageOptions{Page: 1, PageSize: 10})
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "findPage", captured.Operation)
}

// =============================================================================
// buildFindOptions 测试
// =============================================================================

// =============================================================================
// validatePageOptions 测试
// =============================================================================

func TestValidatePageOptions(t *testing.T) {
	tests := []struct {
		name    string
		opts    PageOptions
		wantErr error
	}{
		{"valid", PageOptions{Page: 1, PageSize: 10}, nil},
		{"max page size", PageOptions{Page: 1, PageSize: MaxPageSize}, nil},
		{"page size too large", PageOptions{Page: 1, PageSize: MaxPageSize + 1}, ErrPageSizeTooLarge},
		{"invalid page", PageOptions{Page: 0, PageSize: 10}, ErrInvalidPage},
		{"invalid page size", PageOptions{Page: 1, PageSize: 0}, ErrInvalidPageSize},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validatePageOptions(tt.opts)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestBuildFindOptions_Basic(t *testing.T) {
	opts := PageOptions{
		Page:     1,
		PageSize: 10,
	}
	findOpts := buildFindOptions(0, opts)
	assert.NotNil(t, findOpts)
}

func TestBuildFindOptions_WithProjection(t *testing.T) {
	opts := PageOptions{
		Page:       1,
		PageSize:   10,
		Sort:       bson.D{{Key: "name", Value: 1}},
		Projection: bson.D{{Key: "name", Value: 1}, {Key: "age", Value: 1}},
	}
	findOpts := buildFindOptions(0, opts)
	assert.NotNil(t, findOpts)
}

func TestWrapper_FindPageInternal_WithProjection(t *testing.T) {
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

	result, err := w.findPageInternal(context.Background(), mock, bson.M{}, PageOptions{
		Page:       1,
		PageSize:   10,
		Projection: bson.D{{Key: "name", Value: 1}},
	})
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result.Data, 2)
}

// =============================================================================
// convertPaginationError 完整覆盖
// =============================================================================

func TestConvertPaginationError_UnknownError(t *testing.T) {
	unknownErr := errors.New("some unknown error")
	result := convertPaginationError(unknownErr)
	assert.Equal(t, unknownErr, result)
}

// =============================================================================
// executeSingleBatch 无序模式 context 取消
// =============================================================================

func TestWrapper_ExecuteSingleBatch_UnorderedContextCancel(t *testing.T) {
	mock := newMockCollectionOps()
	mock.insertErr = errMockInsert

	w := &mongoWrapper{
		client:  nil,
		options: defaultOptions(),
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	insertOpts := options.InsertMany().SetOrdered(false)
	count, err, shouldStop := w.executeSingleBatch(ctx, mock, []any{bson.M{"a": 1}}, insertOpts, false, 0, 0)
	assert.Equal(t, int64(0), count)
	assert.Error(t, err)
	assert.True(t, shouldStop)

	// 验证原始 MongoDB 错误被保留（而非被 context 错误替换）
	var batchErr *BulkBatchError
	require.ErrorAs(t, err, &batchErr)
	assert.ErrorIs(t, batchErr.Err, errMockInsert)
}

// =============================================================================
// executeBatches context 取消测试
// =============================================================================

func TestWrapper_ExecuteBatches_ContextCanceled(t *testing.T) {
	mock := newMockCollectionOps()
	w := &mongoWrapper{
		client:  nil,
		options: defaultOptions(),
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	docs := []any{
		map[string]any{"name": "doc1"},
		map[string]any{"name": "doc2"},
	}

	count, errs := w.executeBatches(ctx, mock, docs, 1, false)
	assert.Equal(t, int64(0), count)
	assert.NotEmpty(t, errs)
}

// =============================================================================
// 使用真实 Collection (延迟连接) 测试验证逻辑
// =============================================================================

func TestWrapper_FindPage_InvalidPage_WithRealCollection(t *testing.T) {
	// 创建一个真实的 wrapper 和 collection（使用延迟连接）
	client, err := mongo.Connect(options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer func() {
		if err := client.Disconnect(context.Background()); err != nil {
			t.Logf("cleanup disconnect: %v", err)
		}
	}()

	w := &mongoWrapper{
		client:  client,
		options: defaultOptions(),
	}
	coll := client.Database("testdb").Collection("testcoll")

	// When: findPage is called with invalid page (< 1)
	result, err := w.findPage(context.Background(), coll, map[string]any{}, PageOptions{Page: 0, PageSize: 10})

	// Then: should return ErrInvalidPage
	assert.Nil(t, result)
	assert.ErrorIs(t, err, ErrInvalidPage)
}

func TestWrapper_FindPage_InvalidPageSize_WithRealCollection(t *testing.T) {
	// 创建一个真实的 wrapper 和 collection（使用延迟连接）
	client, err := mongo.Connect(options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer func() {
		if err := client.Disconnect(context.Background()); err != nil {
			t.Logf("cleanup disconnect: %v", err)
		}
	}()

	w := &mongoWrapper{
		client:  client,
		options: defaultOptions(),
	}
	coll := client.Database("testdb").Collection("testcoll")

	// When: findPage is called with invalid page size (< 1)
	result, err := w.findPage(context.Background(), coll, map[string]any{}, PageOptions{Page: 1, PageSize: 0})

	// Then: should return ErrInvalidPageSize
	assert.Nil(t, result)
	assert.ErrorIs(t, err, ErrInvalidPageSize)
}

func TestWrapper_FindPage_ValidParams_WithRealCollection(t *testing.T) {
	// 创建一个真实的 wrapper 和 collection（使用延迟连接）
	client, err := mongo.Connect(options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer func() {
		if err := client.Disconnect(context.Background()); err != nil {
			t.Logf("cleanup disconnect: %v", err)
		}
	}()

	w := &mongoWrapper{
		client:  client,
		options: defaultOptions(),
	}
	coll := client.Database("testdb").Collection("testcoll")

	// When: findPage is called with valid params
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	result, err := w.findPage(ctx, coll, map[string]any{}, PageOptions{Page: 1, PageSize: 10})

	// Then: 可能成功（有 MongoDB）或失败（无 MongoDB），两者都是有效的代码路径
	// 此测试主要验证代码路径可达，而非特定结果
	if err != nil {
		// 无 MongoDB 或连接失败
		assert.Nil(t, result)
	} else {
		// 有 MongoDB 运行时，应返回有效结果
		assert.NotNil(t, result)
		assert.GreaterOrEqual(t, result.Page, int64(1))
		assert.GreaterOrEqual(t, result.PageSize, int64(1))
	}
}

func TestWrapper_BulkInsert_EmptyDocs_WithRealCollection(t *testing.T) {
	// 创建一个真实的 wrapper 和 collection（使用延迟连接）
	client, err := mongo.Connect(options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer func() {
		if err := client.Disconnect(context.Background()); err != nil {
			t.Logf("cleanup disconnect: %v", err)
		}
	}()

	w := &mongoWrapper{
		client:  client,
		options: defaultOptions(),
	}
	coll := client.Database("testdb").Collection("testcoll")

	// When: bulkInsert is called with empty docs
	result, err := w.bulkInsert(context.Background(), coll, []any{}, BulkOptions{})

	// Then: should return ErrEmptyDocs
	assert.Nil(t, result)
	assert.ErrorIs(t, err, ErrEmptyDocs)
}

func TestWrapper_BulkInsert_ValidParams_WithRealCollection(t *testing.T) {
	// 创建一个真实的 wrapper 和 collection（使用延迟连接）
	client, err := mongo.Connect(options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer func() {
		if err := client.Disconnect(context.Background()); err != nil {
			t.Logf("cleanup disconnect: %v", err)
		}
	}()

	w := &mongoWrapper{
		client:  client,
		options: defaultOptions(),
	}
	coll := client.Database("testdb").Collection("testcoll")

	// When: bulkInsert is called with valid params
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	docs := []any{map[string]any{"name": "test"}}
	result, err := w.bulkInsert(ctx, coll, docs, BulkOptions{BatchSize: 10})

	// Then: 可能成功（有 MongoDB）或失败（无 MongoDB），两者都是有效的代码路径
	// 此测试主要验证代码路径可达，而非特定结果
	assert.NotNil(t, result)
	if err != nil {
		// 无 MongoDB 或连接失败 - 错误被收集在 result.Errors 中
		assert.NotEmpty(t, result.Errors)
	} else {
		// 有 MongoDB 运行时，应返回成功结果
		assert.Empty(t, result.Errors)
		assert.Equal(t, int64(1), result.InsertedCount)
	}
}

// =============================================================================
// applyTimeout 测试
// =============================================================================

func TestApplyTimeout_NoTimeout(t *testing.T) {
	ctx := context.Background()
	newCtx, cancel := applyTimeout(ctx, 0)
	defer cancel()

	// timeout 为 0 时不应添加 deadline
	_, hasDeadline := newCtx.Deadline()
	assert.False(t, hasDeadline)
}

func TestApplyTimeout_WithTimeout_NoDeadline(t *testing.T) {
	ctx := context.Background()
	newCtx, cancel := applyTimeout(ctx, 5*time.Second)
	defer cancel()

	// 应添加 deadline
	deadline, hasDeadline := newCtx.Deadline()
	assert.True(t, hasDeadline)
	assert.WithinDuration(t, time.Now().Add(5*time.Second), deadline, 1*time.Second)
}

func TestApplyTimeout_WithTimeout_ExistingDeadline(t *testing.T) {
	existingDeadline := time.Now().Add(10 * time.Second)
	ctx, ctxCancel := context.WithDeadline(context.Background(), existingDeadline)
	defer ctxCancel()

	newCtx, cancel := applyTimeout(ctx, 5*time.Second)
	defer cancel()

	// 已有 deadline 时不应覆盖
	deadline, hasDeadline := newCtx.Deadline()
	assert.True(t, hasDeadline)
	assert.WithinDuration(t, existingDeadline, deadline, 1*time.Second)
}

func TestApplyTimeout_NegativeTimeout(t *testing.T) {
	ctx := context.Background()
	newCtx, cancel := applyTimeout(ctx, -1*time.Second)
	defer cancel()

	// 负值不应添加 deadline
	_, hasDeadline := newCtx.Deadline()
	assert.False(t, hasDeadline)
}

// =============================================================================
// QueryTimeout / WriteTimeout 选项测试
// =============================================================================

func TestWithQueryTimeout(t *testing.T) {
	opts := defaultOptions()
	assert.Equal(t, DefaultQueryTimeout, opts.QueryTimeout, "默认值应为 DefaultQueryTimeout")

	// 正数覆盖默认值
	WithQueryTimeout(10 * time.Second)(opts)
	assert.Equal(t, 10*time.Second, opts.QueryTimeout)

	// 零值显式禁用兜底超时
	WithQueryTimeout(0)(opts)
	assert.Equal(t, time.Duration(0), opts.QueryTimeout)

	// 负值被忽略（保持当前值）
	WithQueryTimeout(-1 * time.Second)(opts)
	assert.Equal(t, time.Duration(0), opts.QueryTimeout)
}

func TestWithWriteTimeout(t *testing.T) {
	opts := defaultOptions()
	assert.Equal(t, DefaultWriteTimeout, opts.WriteTimeout, "默认值应为 DefaultWriteTimeout")

	// 正数覆盖默认值
	WithWriteTimeout(120 * time.Second)(opts)
	assert.Equal(t, 120*time.Second, opts.WriteTimeout)

	// 零值显式禁用兜底超时
	WithWriteTimeout(0)(opts)
	assert.Equal(t, time.Duration(0), opts.WriteTimeout)

	// 负值被忽略（保持当前值）
	WithWriteTimeout(-1 * time.Second)(opts)
	assert.Equal(t, time.Duration(0), opts.WriteTimeout)
}

// =============================================================================
// FindPage 空结果返回空切片测试
// =============================================================================

func TestWrapper_FindPageInternal_EmptyResult_ReturnsEmptySlice(t *testing.T) {
	mock := &cursorCollectionOps{
		docs:     nil, // 无文档
		count:    0,
		collName: "test_coll",
	}

	w := &mongoWrapper{
		client:  nil,
		options: defaultOptions(),
	}

	result, err := w.findPageInternal(context.Background(), mock, bson.M{}, PageOptions{Page: 1, PageSize: 10})
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotNil(t, result.Data, "空结果应返回空切片而非 nil")
	assert.Empty(t, result.Data)
	assert.Equal(t, int64(0), result.Total)
}

// =============================================================================
// QueryTimeout 在 findPageInternal 中生效测试
// =============================================================================

func TestWrapper_FindPageInternal_WithQueryTimeout(t *testing.T) {
	docs := []any{bson.M{"_id": "1"}}
	mock := &cursorCollectionOps{
		docs:     docs,
		count:    1,
		collName: "test_coll",
	}

	opts := defaultOptions()
	opts.QueryTimeout = 30 * time.Second

	w := &mongoWrapper{
		client:  nil,
		options: opts,
	}

	// 使用无 deadline 的 context，应被 QueryTimeout 兜底
	result, err := w.findPageInternal(context.Background(), mock, bson.M{}, PageOptions{Page: 1, PageSize: 10})
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result.Data, 1)
}

// =============================================================================
// WriteTimeout 在 bulkInsertInternal 中生效测试
// =============================================================================

func TestWrapper_BulkInsertInternal_WithWriteTimeout(t *testing.T) {
	mock := newMockCollectionOps()

	opts := defaultOptions()
	opts.WriteTimeout = 60 * time.Second

	w := &mongoWrapper{
		client:  nil,
		options: opts,
	}

	docs := []any{map[string]any{"name": "doc1"}}
	result, err := w.bulkInsertInternal(context.Background(), mock, docs, BulkOptions{BatchSize: 10})
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, int64(1), result.InsertedCount)
}

// =============================================================================
// Nil context 测试
// =============================================================================

func TestWrapper_Health_NilContext(t *testing.T) {
	mock := newMockClientOps()
	w := &mongoWrapper{
		client:    nil,
		clientOps: mock,
		options:   defaultOptions(),
	}

	err := w.Health(nil) //nolint:staticcheck // 测试 nil ctx 防御
	assert.ErrorIs(t, err, ErrNilContext)
	assert.Equal(t, 0, mock.pingCount, "nil ctx 应在 Ping 之前返回")
}

func TestWrapper_FindPage_NilContext(t *testing.T) {
	w := &mongoWrapper{
		client:  nil,
		options: defaultOptions(),
	}

	result, err := w.FindPage(nil, nil, nil, PageOptions{Page: 1, PageSize: 10}) //nolint:staticcheck // 测试 nil ctx 防御
	assert.ErrorIs(t, err, ErrNilContext)
	assert.Nil(t, result)
}

func TestWrapper_BulkInsert_NilContext(t *testing.T) {
	w := &mongoWrapper{
		client:  nil,
		options: defaultOptions(),
	}

	result, err := w.BulkInsert(nil, nil, []any{1}, BulkOptions{}) //nolint:staticcheck // 测试 nil ctx 防御
	assert.ErrorIs(t, err, ErrNilContext)
	assert.Nil(t, result)
}

func TestWrapper_Close_NilContext(t *testing.T) {
	mock := newMockClientOps()
	w := &mongoWrapper{
		client:    nil,
		clientOps: mock,
		options:   defaultOptions(),
	}

	// Close 对 nil ctx 使用 context.Background() 替代，不返回错误
	err := w.Close(nil) //nolint:staticcheck // 测试 nil ctx 防御
	assert.NoError(t, err)
	assert.True(t, mock.disconnected, "Close 应使用 context.Background() 替代 nil ctx")
}
