package xmongo

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// =============================================================================
// 集成测试 - 需要真实 MongoDB
// =============================================================================

// =============================================================================
// 单元测试 - 不需要真实 MongoDB
// =============================================================================

func TestWrapper_Stats_Initial(t *testing.T) {
	// 验证初始统计值
	w := &mongoWrapper{
		client:  nil,
		options: defaultOptions(),
	}

	stats := w.Stats()
	assert.Equal(t, int64(0), stats.PingCount)
	assert.Equal(t, int64(0), stats.PingErrors)
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

	detector, err := newSlowQueryDetector(opts)
	require.NoError(t, err)

	w := &mongoWrapper{
		client:            nil,
		options:           opts,
		slowQueryDetector: detector,
	}

	// 模拟慢查询触发
	info := SlowQueryInfo{
		Database:   "testdb",
		Collection: "users",
		Operation:  "find",
		Filter:     map[string]any{"name": "test"},
		Duration:   200 * time.Millisecond,
	}

	w.maybeSlowQuery(context.Background(), info)

	assert.Equal(t, "testdb", captured.Database)
	assert.Equal(t, "users", captured.Collection)
	assert.Equal(t, "find", captured.Operation)
	assert.Equal(t, 200*time.Millisecond, captured.Duration)
}

func TestWrapper_SlowQueryHook_NilHook(t *testing.T) {
	opts := &Options{
		HealthTimeout:      5 * time.Second,
		SlowQueryThreshold: 100 * time.Millisecond,
		SlowQueryHook:      nil, // 无钩子
	}

	detector, err := newSlowQueryDetector(opts)
	require.NoError(t, err)

	w := &mongoWrapper{
		client:            nil,
		options:           opts,
		slowQueryDetector: detector,
	}

	info := SlowQueryInfo{
		Database:   "testdb",
		Collection: "users",
		Operation:  "find",
		Duration:   200 * time.Millisecond,
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

	detector, err := newSlowQueryDetector(opts)
	require.NoError(t, err)

	w := &mongoWrapper{
		client:            nil,
		options:           opts,
		slowQueryDetector: detector,
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

	detector, err := newSlowQueryDetector(opts)
	require.NoError(t, err)

	w := &mongoWrapper{
		client:            nil,
		options:           opts,
		slowQueryDetector: detector,
	}

	// 耗时高于阈值
	info := SlowQueryInfo{
		Duration: 150 * time.Millisecond,
	}

	triggered := w.maybeSlowQuery(context.Background(), info)
	assert.True(t, called)
	assert.True(t, triggered)
}

func TestWrapper_SlowQueryHook_FilterSnapshot(t *testing.T) {
	var capturedFilter any
	hook := func(_ context.Context, info SlowQueryInfo) {
		capturedFilter = info.Filter
	}

	opts := &Options{
		HealthTimeout:      5 * time.Second,
		SlowQueryThreshold: 100 * time.Millisecond,
		SlowQueryHook:      hook,
	}
	detector, err := newSlowQueryDetector(opts)
	require.NoError(t, err)

	w := &mongoWrapper{
		options:           opts,
		slowQueryDetector: detector,
	}

	filter := bson.M{"name": "test"}
	info := SlowQueryInfo{
		Filter:   filter,
		Duration: 200 * time.Millisecond,
	}

	w.maybeSlowQuery(context.Background(), info)

	// 钩子收到的 Filter 应为字符串快照，而非原始 bson.M 引用
	filterStr, ok := capturedFilter.(string)
	assert.True(t, ok, "Filter should be a string snapshot")
	assert.Contains(t, filterStr, "name")
}

func TestWrapper_SlowQueryHook_FilterSnapshot_Nil(t *testing.T) {
	var capturedFilter any
	hook := func(_ context.Context, info SlowQueryInfo) {
		capturedFilter = info.Filter
	}

	opts := &Options{
		HealthTimeout:      5 * time.Second,
		SlowQueryThreshold: 100 * time.Millisecond,
		SlowQueryHook:      hook,
	}
	detector, err := newSlowQueryDetector(opts)
	require.NoError(t, err)

	w := &mongoWrapper{
		options:           opts,
		slowQueryDetector: detector,
	}

	info := SlowQueryInfo{
		Filter:   nil,
		Duration: 200 * time.Millisecond,
	}

	w.maybeSlowQuery(context.Background(), info)

	// nil Filter 保持 nil
	assert.Nil(t, capturedFilter)
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

	detector, err := newSlowQueryDetector(opts)
	require.NoError(t, err)

	w := &mongoWrapper{
		client:            nil,
		options:           opts,
		slowQueryDetector: detector,
	}

	info := SlowQueryInfo{
		Duration: 1000 * time.Millisecond,
	}

	triggered := w.maybeSlowQuery(context.Background(), info)
	assert.False(t, called)
	assert.False(t, triggered)
}

func TestWrapper_Close_NilClient(t *testing.T) {
	w := &mongoWrapper{
		client:  nil,
		options: defaultOptions(),
	}

	// 关闭 nil client 不应该出错
	err := w.Close(context.Background())
	assert.NoError(t, err)
}

func TestWrapper_Close_DoubleClose(t *testing.T) {
	mock := newMockClientOps()

	w := &mongoWrapper{
		client:    nil,
		clientOps: mock,
		options:   defaultOptions(),
	}

	// 第一次关闭成功
	err := w.Close(context.Background())
	assert.NoError(t, err)
	assert.True(t, mock.disconnected)

	// 第二次关闭返回 ErrClosed
	err = w.Close(context.Background())
	assert.ErrorIs(t, err, ErrClosed)
}

func TestWrapper_Client_NilClient(t *testing.T) {
	w := &mongoWrapper{
		client:  nil,
		options: defaultOptions(),
	}

	// 返回 nil client
	assert.Nil(t, w.Client())
}

func TestWrapper_GetPoolStats_NilClient(t *testing.T) {
	w := &mongoWrapper{
		client:  nil,
		options: defaultOptions(),
	}

	stats := w.getPoolStats()
	assert.Equal(t, 0, stats.InUseConnections)
}

func TestBuildSlowQueryInfoFromOps_Nil(t *testing.T) {
	// 测试 nil collection（通过 buildSlowQueryInfoFromOps）
	info := buildSlowQueryInfoFromOps(nil, "find", map[string]any{"name": "test"})
	assert.Equal(t, "", info.Database)
	assert.Equal(t, "", info.Collection)
	assert.Equal(t, "find", info.Operation)
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
	detector, err := newSlowQueryDetector(opts)
	require.NoError(t, err)

	w := &mongoWrapper{
		client:            nil,
		options:           opts,
		slowQueryDetector: detector,
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
	detector, err := newSlowQueryDetector(opts)
	require.NoError(t, err)

	w := &mongoWrapper{
		client:            nil,
		options:           opts,
		slowQueryDetector: detector,
	}

	// 耗时等于阈值也应该触发
	info := SlowQueryInfo{
		Duration: 100 * time.Millisecond,
	}

	_ = w.maybeSlowQuery(context.Background(), info)
	assert.True(t, called)
}

// =============================================================================
// Mock 测试 - 使用 mock 实现测试核心逻辑
// =============================================================================

func TestWrapper_Health_Success(t *testing.T) {
	mock := newMockClientOps()

	w := &mongoWrapper{
		client:    nil,
		clientOps: mock,
		options:   defaultOptions(),
	}

	err := w.Health(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, 1, mock.pingCount)

	stats := w.Stats()
	assert.Equal(t, int64(1), stats.PingCount)
	assert.Equal(t, int64(0), stats.PingErrors)
}

func TestWrapper_Health_Error(t *testing.T) {
	mock := newMockClientOps()
	mock.pingErr = errMockPing

	w := &mongoWrapper{
		client:    nil,
		clientOps: mock,
		options:   defaultOptions(),
	}

	err := w.Health(context.Background())
	assert.Error(t, err)
	assert.ErrorIs(t, err, errMockPing)
	assert.Contains(t, err.Error(), "xmongo health")
	assert.Equal(t, 1, mock.pingCount)

	stats := w.Stats()
	assert.Equal(t, int64(1), stats.PingCount)
	assert.Equal(t, int64(1), stats.PingErrors)
}

func TestWrapper_Health_WithTimeout(t *testing.T) {
	mock := newMockClientOps()

	w := &mongoWrapper{
		client:    nil,
		clientOps: mock,
		options: &Options{
			HealthTimeout: 5 * time.Second,
		},
	}

	err := w.Health(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, 1, mock.pingCount)
}

func TestWrapper_Health_MultipleCall(t *testing.T) {
	mock := newMockClientOps()

	w := &mongoWrapper{
		client:    nil,
		clientOps: mock,
		options:   defaultOptions(),
	}

	// 多次调用
	for range 5 {
		err := w.Health(context.Background())
		assert.NoError(t, err)
	}

	assert.Equal(t, 5, mock.pingCount)
	stats := w.Stats()
	assert.Equal(t, int64(5), stats.PingCount)
}

func TestWrapper_Close_Success(t *testing.T) {
	mock := newMockClientOps()

	w := &mongoWrapper{
		client:    nil,
		clientOps: mock,
		options:   defaultOptions(),
	}

	err := w.Close(context.Background())
	assert.NoError(t, err)
	assert.True(t, mock.disconnected)
}

func TestWrapper_Close_Error(t *testing.T) {
	mock := newMockClientOps()
	mock.disconnectErr = errMockDisconnect

	w := &mongoWrapper{
		client:    nil,
		clientOps: mock,
		options:   defaultOptions(),
	}

	err := w.Close(context.Background())
	assert.Error(t, err)
	assert.ErrorIs(t, err, errMockDisconnect)
	assert.Contains(t, err.Error(), "xmongo close")
	assert.True(t, mock.disconnected)
}

func TestWrapper_GetPoolStats_WithMock(t *testing.T) {
	mock := newMockClientOps()
	mock.sessionsInProgress = 10

	w := &mongoWrapper{
		client:    nil,
		clientOps: mock,
		options:   defaultOptions(),
	}

	stats := w.getPoolStats()
	assert.Equal(t, 10, stats.InUseConnections)
}

// TestFindPage_NilCollection 已在 pagination_test.go 中定义，此处不重复。

func TestWrapper_FindPageInternal_InvalidPage(t *testing.T) {
	mock := newMockCollectionOps()
	w := &mongoWrapper{
		client:  nil,
		options: defaultOptions(),
	}

	// 通过 findPageInternal 测试，绕过 findPage 的 nil collection 检查
	// 但 findPageInternal 不检查 Page，所以这里测试 findPage 的参数校验
	// 由于 findPage 需要真实的 *mongo.Collection，改为测试边界情况
	result, err := w.FindPage(context.Background(), nil, nil, PageOptions{Page: 0, PageSize: 10})
	assert.Error(t, err)
	assert.Equal(t, ErrNilCollection, err) // nil collection 先于 invalid page 检查
	assert.Nil(t, result)

	// 测试 findPageInternal 使用有效参数（Page < 1 不会被检查，因为 findPageInternal 不做此校验）
	mock.countResult = 0
	mock.findErr = errMockFind
	result, err = w.findPageInternal(context.Background(), mock, nil, PageOptions{Page: 0, PageSize: 10})
	// findPageInternal 会尝试执行，根据 countResult 决定后续行为
	// 由于 findErr 设置了，会在 Find 阶段失败
	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestWrapper_FindPageInternal_InvalidPageSize(t *testing.T) {
	mock := newMockCollectionOps()
	w := &mongoWrapper{
		client:  nil,
		options: defaultOptions(),
	}

	// 测试 PageSize 为负数边界情况
	mock.countResult = 100
	mock.findErr = errMockFind

	result, err := w.findPageInternal(context.Background(), mock, nil, PageOptions{Page: 1, PageSize: -1})
	assert.Error(t, err)
	assert.Nil(t, result)
}
