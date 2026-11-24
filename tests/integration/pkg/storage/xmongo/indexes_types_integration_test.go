//go:build integration

package xmongo

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// =============================================================================
// 索引操作测试
// =============================================================================

func TestMongo_Indexes_Integration(t *testing.T) {
	client, cleanup := setupMongo(t)
	defer cleanup()

	ctx := context.Background()
	db := client.Database("test_indexes")
	coll := db.Collection("indexed_items")

	t.Cleanup(func() {
		coll.Drop(context.Background())
	})

	t.Run("创建单字段索引", func(t *testing.T) {
		name, err := coll.Indexes().CreateOne(ctx, mongo.IndexModel{
			Keys: bson.D{{Key: "name", Value: 1}},
		})
		require.NoError(t, err)
		assert.NotEmpty(t, name)
	})

	t.Run("创建复合索引", func(t *testing.T) {
		name, err := coll.Indexes().CreateOne(ctx, mongo.IndexModel{
			Keys: bson.D{
				{Key: "category", Value: 1},
				{Key: "created_at", Value: -1},
			},
		})
		require.NoError(t, err)
		assert.NotEmpty(t, name)
	})

	t.Run("创建唯一索引", func(t *testing.T) {
		name, err := coll.Indexes().CreateOne(ctx, mongo.IndexModel{
			Keys:    bson.D{{Key: "email", Value: 1}},
			Options: options.Index().SetUnique(true),
		})
		require.NoError(t, err)
		assert.NotEmpty(t, name)

		// 验证唯一性约束
		_, err = coll.InsertOne(ctx, bson.M{"email": "test@example.com"})
		require.NoError(t, err)

		_, err = coll.InsertOne(ctx, bson.M{"email": "test@example.com"})
		assert.Error(t, err) // 应该失败
	})

	t.Run("创建 TTL 索引", func(t *testing.T) {
		_, err := coll.Indexes().CreateOne(ctx, mongo.IndexModel{
			Keys:    bson.D{{Key: "expire_at", Value: 1}},
			Options: options.Index().SetExpireAfterSeconds(3600),
		})
		require.NoError(t, err)
	})

	t.Run("列出索引", func(t *testing.T) {
		cursor, err := coll.Indexes().List(ctx)
		require.NoError(t, err)

		var indexes []bson.M
		err = cursor.All(ctx, &indexes)
		require.NoError(t, err)

		// 应该至少有 _id 索引和我们创建的索引
		assert.GreaterOrEqual(t, len(indexes), 2)
	})
}

// =============================================================================
// 聚合操作测试
// =============================================================================

func TestMongo_Aggregate_Integration(t *testing.T) {
	client, cleanup := setupMongo(t)
	defer cleanup()

	ctx := context.Background()
	db := client.Database("test_aggregate")
	coll := db.Collection("orders")

	// 准备测试数据
	coll.Drop(ctx)
	docs := []any{
		bson.M{"customer": "alice", "product": "A", "amount": 100, "status": "completed"},
		bson.M{"customer": "alice", "product": "B", "amount": 200, "status": "completed"},
		bson.M{"customer": "bob", "product": "A", "amount": 150, "status": "pending"},
		bson.M{"customer": "bob", "product": "C", "amount": 300, "status": "completed"},
		bson.M{"customer": "charlie", "product": "B", "amount": 250, "status": "completed"},
	}
	_, err := coll.InsertMany(ctx, docs)
	require.NoError(t, err)

	t.Cleanup(func() {
		coll.Drop(context.Background())
	})

	t.Run("$group 分组统计", func(t *testing.T) {
		pipeline := []bson.M{
			{"$group": bson.M{
				"_id":       "$customer",
				"total":     bson.M{"$sum": "$amount"},
				"count":     bson.M{"$sum": 1},
				"avgAmount": bson.M{"$avg": "$amount"},
			}},
			{"$sort": bson.M{"total": -1}},
		}

		cursor, err := coll.Aggregate(ctx, pipeline)
		require.NoError(t, err)

		var results []bson.M
		err = cursor.All(ctx, &results)
		require.NoError(t, err)

		assert.Len(t, results, 3)
		// 第一个应该是总额最高的
		assert.Equal(t, "bob", results[0]["_id"])
	})

	t.Run("$match + $group", func(t *testing.T) {
		pipeline := []bson.M{
			{"$match": bson.M{"status": "completed"}},
			{"$group": bson.M{
				"_id":   "$product",
				"total": bson.M{"$sum": "$amount"},
			}},
		}

		cursor, err := coll.Aggregate(ctx, pipeline)
		require.NoError(t, err)

		var results []bson.M
		err = cursor.All(ctx, &results)
		require.NoError(t, err)

		// 只统计 completed 状态的订单
		for _, r := range results {
			t.Logf("Product: %v, Total: %v", r["_id"], r["total"])
		}
	})

	t.Run("$project 字段投影", func(t *testing.T) {
		pipeline := []bson.M{
			{"$project": bson.M{
				"customer":    1,
				"orderValue":  "$amount",
				"isCompleted": bson.M{"$eq": []any{"$status", "completed"}},
			}},
		}

		cursor, err := coll.Aggregate(ctx, pipeline)
		require.NoError(t, err)

		var results []bson.M
		err = cursor.All(ctx, &results)
		require.NoError(t, err)

		for _, r := range results {
			assert.Contains(t, r, "customer")
			assert.Contains(t, r, "orderValue")
			assert.Contains(t, r, "isCompleted")
		}
	})

	t.Run("$lookup 关联查询", func(t *testing.T) {
		// 创建关联集合
		customersColl := db.Collection("customers")
		customersColl.Drop(ctx)
		_, err := customersColl.InsertMany(ctx, []any{
			bson.M{"name": "alice", "email": "alice@example.com"},
			bson.M{"name": "bob", "email": "bob@example.com"},
		})
		require.NoError(t, err)

		pipeline := []bson.M{
			{"$lookup": bson.M{
				"from":         "customers",
				"localField":   "customer",
				"foreignField": "name",
				"as":           "customerInfo",
			}},
			{"$match": bson.M{"customer": "alice"}},
		}

		cursor, err := coll.Aggregate(ctx, pipeline)
		require.NoError(t, err)

		var results []bson.M
		err = cursor.All(ctx, &results)
		require.NoError(t, err)

		if len(results) > 0 {
			customerInfo := results[0]["customerInfo"].(bson.A)
			assert.NotEmpty(t, customerInfo)
		}
	})
}

// =============================================================================
// 健康检查和统计测试
// =============================================================================

func TestMongo_HealthAndStats_Integration(t *testing.T) {
	client, cleanup := setupMongo(t)
	defer cleanup()

	wrapper, err := New(client, WithHealthTimeout(5*time.Second))
	require.NoError(t, err)

	ctx := context.Background()

	t.Run("Health 检查", func(t *testing.T) {
		err := wrapper.Health(ctx)
		require.NoError(t, err)
	})

	t.Run("Stats 统计", func(t *testing.T) {
		// 执行几次健康检查
		for i := 0; i < 3; i++ {
			wrapper.Health(ctx)
		}

		stats := wrapper.Stats()
		assert.GreaterOrEqual(t, stats.PingCount, int64(3))
	})
}

// =============================================================================
// 慢查询钩子测试
// =============================================================================

func TestMongo_SlowQueryHook_Integration(t *testing.T) {
	client, cleanup := setupMongo(t)
	defer cleanup()

	var slowQueries []SlowQueryInfo
	var mu sync.Mutex

	wrapper, err := New(client,
		WithSlowQueryThreshold(1*time.Millisecond), // 很短的阈值，确保触发
		WithSlowQueryHook(func(ctx context.Context, info SlowQueryInfo) {
			mu.Lock()
			slowQueries = append(slowQueries, info)
			mu.Unlock()
		}),
	)
	require.NoError(t, err)

	ctx := context.Background()
	coll := client.Database("test_slow").Collection("items")

	// 准备测试数据
	docs := make([]any, 100)
	for i := 0; i < 100; i++ {
		docs[i] = bson.M{"index": i}
	}
	_, err = coll.InsertMany(ctx, docs)
	require.NoError(t, err)

	t.Cleanup(func() {
		coll.Drop(context.Background())
	})

	// 执行查询
	_, err = wrapper.FindPage(ctx, coll, bson.M{}, PageOptions{
		Page:     1,
		PageSize: 50,
	})
	require.NoError(t, err)

	// 验证慢查询被记录
	mu.Lock()
	defer mu.Unlock()
	assert.NotEmpty(t, slowQueries, "应该记录到慢查询")
	if len(slowQueries) > 0 {
		assert.Equal(t, "findPage", slowQueries[0].Operation)
	}
}

// =============================================================================
// 并发安全测试
// =============================================================================

func TestMongo_ConcurrentAccess_Integration(t *testing.T) {
	client, cleanup := setupMongo(t)
	defer cleanup()

	wrapper, err := New(client)
	require.NoError(t, err)

	ctx := context.Background()
	coll := client.Database("test_concurrent").Collection("items")
	coll.Drop(ctx)

	// 准备数据
	docs := make([]any, 100)
	for i := 0; i < 100; i++ {
		docs[i] = bson.M{"index": i}
	}
	_, err = coll.InsertMany(ctx, docs)
	require.NoError(t, err)

	t.Cleanup(func() {
		coll.Drop(context.Background())
	})

	t.Run("并发分页查询", func(t *testing.T) {
		const goroutines = 20
		var wg sync.WaitGroup
		var errorCount atomic.Int64

		for i := 0; i < goroutines; i++ {
			page := int64(i)
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := wrapper.FindPage(ctx, coll, bson.M{}, PageOptions{
					Page:     page%10 + 1,
					PageSize: 10,
				})
				if err != nil {
					errorCount.Add(1)
				}
			}()
		}

		wg.Wait()
		assert.Zero(t, errorCount.Load(), "不应该有错误")
	})

	t.Run("并发写入", func(t *testing.T) {
		writeColl := client.Database("test_concurrent").Collection("write_items")
		writeColl.Drop(ctx)
		t.Cleanup(func() {
			writeColl.Drop(context.Background())
		})

		const goroutines = 10
		var wg sync.WaitGroup
		var successCount atomic.Int64

		for i := 0; i < goroutines; i++ {
			id := i
			wg.Add(1)
			go func() {
				defer wg.Done()
				docs := make([]any, 10)
				for j := 0; j < 10; j++ {
					docs[j] = bson.M{"worker": id, "item": j}
				}
				result, err := wrapper.BulkInsert(ctx, writeColl, docs, BulkOptions{})
				if err == nil {
					successCount.Add(result.InsertedCount)
				}
			}()
		}

		wg.Wait()
		assert.Equal(t, int64(100), successCount.Load())
	})
}

// =============================================================================
// 事务测试
// =============================================================================

func TestMongo_Transaction_Integration(t *testing.T) {
	client, cleanup := setupMongo(t)
	defer cleanup()

	ctx := context.Background()
	db := client.Database("test_transaction")
	coll := db.Collection("accounts")

	// 检查是否支持事务（需要副本集）
	serverStatus, err := client.Database("admin").RunCommand(ctx, bson.M{"serverStatus": 1}).Raw()
	if err != nil {
		t.Skip("无法获取服务器状态")
	}

	// 检查是否为副本集
	_, lookupErr := serverStatus.LookupErr("repl")
	if lookupErr != nil {
		t.Skip("事务需要副本集环境，跳过测试")
	}

	// 准备账户
	coll.Drop(ctx)
	_, err = coll.InsertMany(ctx, []any{
		bson.M{"name": "alice", "balance": 1000},
		bson.M{"name": "bob", "balance": 500},
	})
	require.NoError(t, err)

	t.Run("成功的事务", func(t *testing.T) {
		session, err := client.StartSession()
		require.NoError(t, err)
		defer session.EndSession(ctx)

		_, err = session.WithTransaction(ctx, func(sc context.Context) (any, error) {
			// 从 alice 扣款
			_, err := coll.UpdateOne(sc, bson.M{"name": "alice"}, bson.M{"$inc": bson.M{"balance": -100}})
			if err != nil {
				return nil, err
			}
			// 给 bob 加款
			_, err = coll.UpdateOne(sc, bson.M{"name": "bob"}, bson.M{"$inc": bson.M{"balance": 100}})
			if err != nil {
				return nil, err
			}
			return nil, nil
		})
		require.NoError(t, err)

		// 验证结果
		var alice, bob bson.M
		err = coll.FindOne(ctx, bson.M{"name": "alice"}).Decode(&alice)
		require.NoError(t, err)
		err = coll.FindOne(ctx, bson.M{"name": "bob"}).Decode(&bob)
		require.NoError(t, err)

		assert.Equal(t, int32(900), alice["balance"])
		assert.Equal(t, int32(600), bob["balance"])
	})
}

// =============================================================================
// 特殊数据类型测试
// =============================================================================

func TestMongo_DataTypes_Integration(t *testing.T) {
	client, cleanup := setupMongo(t)
	defer cleanup()

	ctx := context.Background()
	coll := client.Database("test_types").Collection("docs")
	coll.Drop(ctx)

	t.Cleanup(func() {
		coll.Drop(context.Background())
	})

	t.Run("各种数据类型", func(t *testing.T) {
		now := time.Now().Truncate(time.Millisecond)
		doc := bson.M{
			"string":   "hello",
			"int":      42,
			"int64":    int64(9999999999),
			"float64":  3.14159,
			"bool":     true,
			"date":     now,
			"array":    []int{1, 2, 3, 4, 5},
			"nested":   bson.M{"key": "value", "num": 123},
			"null":     nil,
			"objectId": bson.NewObjectID(),
		}

		result, err := coll.InsertOne(ctx, doc)
		require.NoError(t, err)

		// 读取回来验证
		var readDoc bson.M
		err = coll.FindOne(ctx, bson.M{"_id": result.InsertedID}).Decode(&readDoc)
		require.NoError(t, err)

		assert.Equal(t, "hello", readDoc["string"])
		assert.Equal(t, int32(42), readDoc["int"])
		assert.Equal(t, int64(9999999999), readDoc["int64"])
		assert.InDelta(t, 3.14159, readDoc["float64"], 0.00001)
		assert.Equal(t, true, readDoc["bool"])
	})

	t.Run("嵌套文档查询", func(t *testing.T) {
		_, err := coll.InsertOne(ctx, bson.M{
			"user": bson.M{
				"name":  "john",
				"email": "john@example.com",
				"address": bson.M{
					"city":    "Beijing",
					"country": "China",
				},
			},
		})
		require.NoError(t, err)

		// 点表示法查询嵌套字段
		var doc bson.M
		err = coll.FindOne(ctx, bson.M{"user.address.city": "Beijing"}).Decode(&doc)
		require.NoError(t, err)

		user := doc["user"].(bson.M)
		assert.Equal(t, "john", user["name"])
	})

	t.Run("数组查询", func(t *testing.T) {
		_, err := coll.InsertOne(ctx, bson.M{
			"name": "tags-doc",
			"tags": []string{"go", "mongodb", "testing"},
		})
		require.NoError(t, err)

		// 查询数组包含特定元素
		var doc bson.M
		err = coll.FindOne(ctx, bson.M{"tags": "mongodb"}).Decode(&doc)
		require.NoError(t, err)
		assert.Equal(t, "tags-doc", doc["name"])

		// $all 查询数组包含所有指定元素
		err = coll.FindOne(ctx, bson.M{"tags": bson.M{"$all": []string{"go", "testing"}}}).Decode(&doc)
		require.NoError(t, err)

		// $size 查询数组长度
		err = coll.FindOne(ctx, bson.M{"tags": bson.M{"$size": 3}}).Decode(&doc)
		require.NoError(t, err)
	})
}
