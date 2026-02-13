package xmongo

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func TestAdaptCollection_WithRealCollection(t *testing.T) {
	// 创建一个客户端 - 使用延迟连接
	client, err := mongo.Connect(options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer func() {
		_ = client.Disconnect(context.Background()) //nolint:errcheck // cleanup in test
	}()

	// 获取一个 collection
	coll := client.Database("testdb").Collection("testcoll")

	// When: adaptCollection is called
	adapted := adaptCollection(coll)

	// Then: should return valid adapter
	assert.NotNil(t, adapted)
	assert.Equal(t, "testcoll", adapted.Name())
	assert.NotNil(t, adapted.Database())
	assert.Equal(t, "testdb", adapted.Database().Name())
}

func TestBuildSlowQueryInfoFromOps_WithRealCollection(t *testing.T) {
	// 使用真实的 collection 来测试 Database().Name() 路径
	// 创建一个客户端 - 使用延迟连接
	client, err := mongo.Connect(options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer func() {
		_ = client.Disconnect(context.Background()) //nolint:errcheck // cleanup in test
	}()

	// 获取一个 collection 并适配为接口
	coll := client.Database("mydb").Collection("users")
	adapted := adaptCollection(coll)

	info := buildSlowQueryInfoFromOps(adapted, "find", map[string]any{"name": "test"})
	assert.Equal(t, "mydb", info.Database)
	assert.Equal(t, "users", info.Collection)
	assert.Equal(t, "find", info.Operation)
}

func TestBuildSlowQueryInfoFromOps_WithNilDatabase(t *testing.T) {
	// 使用默认 mock (Database 返回 nil)
	mock := newMockCollectionOps()
	mock.collName = "testcoll"

	info := buildSlowQueryInfoFromOps(mock, "insert", nil)
	assert.Equal(t, "", info.Database) // Database 为 nil，所以 dbName 为空
	assert.Equal(t, "testcoll", info.Collection)
	assert.Equal(t, "insert", info.Operation)
}

// 使用真实 collection 来测试 collectionAdapter 的所有方法
func TestCollectionAdapter_AllMethods(t *testing.T) {
	// 创建一个客户端 - 使用延迟连接
	client, err := mongo.Connect(options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer func() {
		_ = client.Disconnect(context.Background()) //nolint:errcheck // cleanup in test
	}()

	// 获取一个 collection
	coll := client.Database("testdb").Collection("testcoll")
	adapter := &collectionAdapter{coll: coll}

	// 测试 Name
	assert.Equal(t, "testcoll", adapter.Name())

	// 测试 Database
	db := adapter.Database()
	assert.NotNil(t, db)
	assert.Equal(t, "testdb", db.Name())
}

// 测试 collectionOperations 接口编译时检查
func TestCollectionOperationsInterface(t *testing.T) {
	// 编译时检查 collectionAdapter 实现 collectionOperations 接口
	var _ collectionOperations = (*collectionAdapter)(nil)

	// 编译时检查 mockCollectionOps 实现 collectionOperations 接口
	var _ collectionOperations = (*mockCollectionOps)(nil)
}

// 测试 clientOperations 接口编译时检查
func TestClientOperationsInterface(t *testing.T) {
	// 编译时检查 mockClientOps 实现 clientOperations 接口
	var _ clientOperations = (*mockClientOps)(nil)
}

// TestCollectionAdapter_OperationMethods 测试 collectionAdapter 的操作方法
// 此测试主要验证代码路径可达，可能成功（有 MongoDB）或失败（无 MongoDB）
func TestCollectionAdapter_OperationMethods(t *testing.T) {
	// 创建一个客户端 - 使用延迟连接
	client, err := mongo.Connect(options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer func() {
		_ = client.Disconnect(context.Background()) //nolint:errcheck // cleanup in test
	}()

	// 获取一个 collection
	coll := client.Database("testdb").Collection("testcoll")
	adapter := &collectionAdapter{coll: coll}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// 测试 CountDocuments - 可能成功或失败，主要验证代码路径可达
	count, err := adapter.CountDocuments(ctx, map[string]any{})
	if err != nil {
		// 无 MongoDB 或连接失败
		t.Logf("CountDocuments failed (expected without MongoDB): %v", err)
	} else {
		// 有 MongoDB 运行时
		assert.GreaterOrEqual(t, count, int64(0))
	}

	// 测试 Find
	cursor, err := adapter.Find(ctx, map[string]any{})
	if err != nil {
		t.Logf("Find failed (expected without MongoDB): %v", err)
	} else {
		assert.NotNil(t, cursor)
		cursor.Close(ctx) //nolint:errcheck // test cleanup
	}

	// 测试 InsertMany
	result, err := adapter.InsertMany(ctx, []any{map[string]any{"test": 1}})
	if err != nil {
		t.Logf("InsertMany failed (expected without MongoDB): %v", err)
	} else {
		assert.NotNil(t, result)
	}
}
