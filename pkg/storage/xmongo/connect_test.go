package xmongo

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// =============================================================================
// Connect 参数校验测试
// =============================================================================

func TestConnect_NilContext(t *testing.T) {
	t.Parallel()

	//nolint:staticcheck // 故意传 nil ctx 测试守卫
	_, err := Connect(nil, "mongodb://localhost:27017", nil)
	require.ErrorIs(t, err, ErrNilContext)
}

func TestConnect_EmptyURI(t *testing.T) {
	t.Parallel()

	_, err := Connect(context.Background(), "", nil)
	require.ErrorIs(t, err, ErrEmptyURI)
}

func TestConnect_WhitespaceURI(t *testing.T) {
	t.Parallel()

	_, err := Connect(context.Background(), "   ", nil)
	require.ErrorIs(t, err, ErrEmptyURI)
}

// =============================================================================
// Connect 功能测试（使用无效地址，验证 wrapper 创建流程）
// =============================================================================

func TestConnect_CreatesWrapper(t *testing.T) {
	t.Parallel()

	// mongo.Connect 不验证连通性，只验证配置是否合法
	m, err := Connect(context.Background(), "mongodb://localhost:27017", nil)
	require.NoError(t, err)
	require.NotNil(t, m)

	// 验证 Client() 返回非 nil
	assert.NotNil(t, m.Client())

	// 关闭
	require.NoError(t, m.Close(context.Background()))
}

func TestConnect_NilConfigFn(t *testing.T) {
	t.Parallel()

	m, err := Connect(context.Background(), "mongodb://localhost:27017", nil)
	require.NoError(t, err)
	require.NotNil(t, m)
	require.NoError(t, m.Close(context.Background()))
}

func TestConnect_WithConfigFn(t *testing.T) {
	t.Parallel()

	// configFn 可配置 mongo 原生选项（如压缩、连接池）
	m, err := Connect(context.Background(), "mongodb://localhost:27017",
		func(opts *options.ClientOptions) {
			opts.SetCompressors([]string{"zstd", "snappy"})
			opts.SetMaxPoolSize(50)
		},
	)
	require.NoError(t, err)
	require.NotNil(t, m)
	require.NoError(t, m.Close(context.Background()))
}

func TestConnect_WithXmongoOptions(t *testing.T) {
	t.Parallel()

	m, err := Connect(context.Background(), "mongodb://localhost:27017", nil,
		WithSlowQueryThreshold(200*time.Millisecond),
		WithQueryTimeout(10*time.Second),
	)
	require.NoError(t, err)
	require.NotNil(t, m)
	require.NoError(t, m.Close(context.Background()))
}

func TestConnect_InvalidURI(t *testing.T) {
	t.Parallel()

	// 非法的 MongoDB scheme 会导致 mongo.Connect 返回错误
	_, err := Connect(context.Background(), "://invalid", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "xmongo connect")
}

func TestConnect_WithConfigFnAndXmongoOptions(t *testing.T) {
	t.Parallel()

	m, err := Connect(context.Background(), "mongodb://localhost:27017",
		func(opts *options.ClientOptions) {
			opts.SetCompressors([]string{"zstd"})
		},
		WithSlowQueryThreshold(100*time.Millisecond),
	)
	require.NoError(t, err)
	require.NotNil(t, m)
	require.NoError(t, m.Close(context.Background()))
}

// 设计决策: 不为 Connect → New() 失败路径编写测试。
// 当前 New() 内部所有 Option 参数都被 clamp 到安全范围（WithAsyncSlowQueryWorkers
// 上限为 maxAsyncWorkers=1000，远低于 xpool.maxWorkers=65536），因此
// newSlowQueryDetector 在 client != nil 场景下实际上无法失败。
// connect.go 中的 client.Disconnect 清理逻辑是防御性代码，
// 用于兜底未来 New() 可能新增的失败路径（如配置项扩展、外部依赖校验）。
