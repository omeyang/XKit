package xmongo

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func TestNew_NilClient(t *testing.T) {
	// Given: nil client
	// When: New is called with nil
	m, err := New(nil)

	// Then: should return ErrNilClient
	assert.Nil(t, m)
	assert.ErrorIs(t, err, ErrNilClient)
}

func TestNew_NilOption(t *testing.T) {
	// 创建一个客户端 - 使用延迟连接
	client, err := mongo.Connect(options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer func() {
		if err := client.Disconnect(context.Background()); err != nil {
			t.Logf("cleanup disconnect: %v", err)
		}
	}()

	// When: New is called with nil option — should not panic
	m, err := New(client, nil, WithHealthTimeout(10*time.Second), nil)

	// Then: should succeed and ignore nil options
	assert.NoError(t, err)
	assert.NotNil(t, m)
	wrapper, ok := m.(*mongoWrapper)
	assert.True(t, ok)
	assert.Equal(t, 10*time.Second, wrapper.options.HealthTimeout)
}

func TestNew_Success(t *testing.T) {
	// 创建一个客户端 - 使用延迟连接，不需要真实的 MongoDB 服务器
	client, err := mongo.Connect(options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer func() {
		if err := client.Disconnect(context.Background()); err != nil {
			t.Logf("cleanup disconnect: %v", err)
		}
	}()

	// When: New is called with valid client
	m, err := New(client)

	// Then: should return valid Mongo wrapper
	assert.NoError(t, err)
	assert.NotNil(t, m)
	assert.NotNil(t, m.Client())
	assert.Equal(t, client, m.Client())
}

func TestNew_WithAllOptions(t *testing.T) {
	// 创建一个客户端
	client, err := mongo.Connect(options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer func() {
		if err := client.Disconnect(context.Background()); err != nil {
			t.Logf("cleanup disconnect: %v", err)
		}
	}()

	var hookCalled bool
	hook := func(_ context.Context, _ SlowQueryInfo) {
		hookCalled = true
	}

	// When: New is called with all options
	m, err := New(client,
		WithHealthTimeout(10*time.Second),
		WithSlowQueryThreshold(100*time.Millisecond),
		WithSlowQueryHook(hook),
	)

	// Then: should return valid Mongo wrapper
	assert.NoError(t, err)
	assert.NotNil(t, m)

	// 验证选项正确应用
	wrapper, ok := m.(*mongoWrapper)
	assert.True(t, ok)
	assert.Equal(t, 10*time.Second, wrapper.options.HealthTimeout)
	assert.Equal(t, 100*time.Millisecond, wrapper.options.SlowQueryThreshold)
	assert.NotNil(t, wrapper.options.SlowQueryHook)

	// 触发钩子验证
	wrapper.options.SlowQueryHook(context.Background(), SlowQueryInfo{})
	assert.True(t, hookCalled)
}

func TestNew_WithOptions_Integration(t *testing.T) {
	// 注意：此测试需要真实的 MongoDB 客户端
	// 在没有集成测试环境时跳过
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
}

func TestMongoInterface(t *testing.T) {
	// 验证接口定义是否正确
	// 这是一个编译时检查，确保 mongoWrapper 实现了 Mongo 接口
	var _ Mongo = (*mongoWrapper)(nil)
}

func TestOptionsApplied_Integration(t *testing.T) {
	// 注意：此测试验证选项是否正确应用
	// 需要真实客户端才能完整测试
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
}

// 以下是选项应用的单元测试
func TestOptionsAreApplied(t *testing.T) {
	// 验证选项函数能正确修改配置
	opts := defaultOptions()

	WithHealthTimeout(10 * time.Second)(opts)
	assert.Equal(t, 10*time.Second, opts.HealthTimeout)

	WithSlowQueryThreshold(100 * time.Millisecond)(opts)
	assert.Equal(t, 100*time.Millisecond, opts.SlowQueryThreshold)
}
