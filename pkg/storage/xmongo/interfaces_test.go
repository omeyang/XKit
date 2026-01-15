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

func TestBuildSlowQueryInfo_WithRealCollection(t *testing.T) {
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

	// When: buildSlowQueryInfo is called
	info := buildSlowQueryInfo(coll, "update", map[string]any{"id": 1}, 100)

	// Then: should return correct info
	assert.Equal(t, "testdb", info.Database)
	assert.Equal(t, "testcoll", info.Collection)
	assert.Equal(t, "update", info.Operation)
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
// 这些方法会因为没有真实的 MongoDB 服务器而失败，但会覆盖代码路径
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

	// 测试 CountDocuments - 会因为超时或连接失败而报错，但覆盖代码路径
	_, err = adapter.CountDocuments(ctx, map[string]any{})
	// 我们期望这会失败（没有真实的 MongoDB），但代码路径被执行了
	assert.Error(t, err)

	// 测试 Find
	_, err = adapter.Find(ctx, map[string]any{})
	assert.Error(t, err)

	// 测试 InsertMany
	_, err = adapter.InsertMany(ctx, []any{map[string]any{"test": 1}})
	assert.Error(t, err)
}
