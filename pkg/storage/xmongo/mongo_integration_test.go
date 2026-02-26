//go:build integration

package xmongo

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// =============================================================================
// 测试环境设置
// =============================================================================

func setupMongo(t *testing.T) (*mongo.Client, func()) {
	t.Helper()

	uri := os.Getenv("XKIT_MONGO_URI")
	if uri == "" {
		uri = startMongoContainer(t)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatalf("mongo connect failed: %v", err)
	}

	// 验证连接
	if err := client.Ping(ctx, nil); err != nil {
		client.Disconnect(context.Background())
		t.Fatalf("mongo ping failed: %v", err)
	}

	return client, func() {
		client.Disconnect(context.Background())
	}
}

func startMongoContainer(t *testing.T) string {
	t.Helper()

	// 探测 Docker 可用性，避免 testcontainers 内部 panic
	// （例如 $XDG_RUNTIME_DIR 检查失败等环境问题）
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not found in PATH, skipping integration test")
	}

	ctx := context.Background()
	req := testcontainers.ContainerRequest{
		Image:        "mongo:7.0",
		ExposedPorts: []string{"27017/tcp"},
		WaitingFor:   wait.ForListeningPort("27017/tcp"),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Skipf("mongo container not available: %v", err)
	}

	t.Cleanup(func() {
		container.Terminate(ctx)
	})

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("mongo host failed: %v", err)
	}
	port, err := container.MappedPort(ctx, "27017/tcp")
	if err != nil {
		t.Fatalf("mongo port failed: %v", err)
	}

	return fmt.Sprintf("mongodb://%s:%s", host, port.Port())
}

// =============================================================================
// 基础 CRUD 操作测试
// =============================================================================

func TestMongo_CRUD_Integration(t *testing.T) {
	client, cleanup := setupMongo(t)
	defer cleanup()

	ctx := context.Background()
	db := client.Database("test_crud")
	coll := db.Collection("items")

	// 清理测试数据
	t.Cleanup(func() {
		coll.Drop(context.Background())
	})

	t.Run("InsertOne", func(t *testing.T) {
		doc := bson.M{"name": "item1", "value": 100}
		result, err := coll.InsertOne(ctx, doc)
		require.NoError(t, err)
		assert.NotNil(t, result.InsertedID)
	})

	t.Run("InsertMany", func(t *testing.T) {
		docs := []any{
			bson.M{"name": "item2", "value": 200},
			bson.M{"name": "item3", "value": 300},
			bson.M{"name": "item4", "value": 400},
		}
		result, err := coll.InsertMany(ctx, docs)
		require.NoError(t, err)
		assert.Len(t, result.InsertedIDs, 3)
	})

	t.Run("FindOne", func(t *testing.T) {
		var doc bson.M
		err := coll.FindOne(ctx, bson.M{"name": "item1"}).Decode(&doc)
		require.NoError(t, err)
		assert.Equal(t, "item1", doc["name"])
		assert.Equal(t, int32(100), doc["value"])
	})

	t.Run("Find 多条", func(t *testing.T) {
		cursor, err := coll.Find(ctx, bson.M{"value": bson.M{"$gte": 200}})
		require.NoError(t, err)

		var results []bson.M
		err = cursor.All(ctx, &results)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(results), 3)
	})

	t.Run("UpdateOne", func(t *testing.T) {
		result, err := coll.UpdateOne(ctx,
			bson.M{"name": "item1"},
			bson.M{"$set": bson.M{"value": 150}},
		)
		require.NoError(t, err)
		assert.Equal(t, int64(1), result.ModifiedCount)

		// 验证更新
		var doc bson.M
		err = coll.FindOne(ctx, bson.M{"name": "item1"}).Decode(&doc)
		require.NoError(t, err)
		assert.Equal(t, int32(150), doc["value"])
	})

	t.Run("UpdateMany", func(t *testing.T) {
		result, err := coll.UpdateMany(ctx,
			bson.M{"value": bson.M{"$gte": 200}},
			bson.M{"$inc": bson.M{"value": 10}},
		)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, result.ModifiedCount, int64(3))
	})

	t.Run("DeleteOne", func(t *testing.T) {
		result, err := coll.DeleteOne(ctx, bson.M{"name": "item4"})
		require.NoError(t, err)
		assert.Equal(t, int64(1), result.DeletedCount)
	})

	t.Run("DeleteMany", func(t *testing.T) {
		// 先插入一些用于删除的数据
		docs := []any{
			bson.M{"name": "temp1", "toDelete": true},
			bson.M{"name": "temp2", "toDelete": true},
		}
		_, err := coll.InsertMany(ctx, docs)
		require.NoError(t, err)

		result, err := coll.DeleteMany(ctx, bson.M{"toDelete": true})
		require.NoError(t, err)
		assert.Equal(t, int64(2), result.DeletedCount)
	})

	t.Run("ReplaceOne", func(t *testing.T) {
		result, err := coll.ReplaceOne(ctx,
			bson.M{"name": "item1"},
			bson.M{"name": "item1_replaced", "value": 999, "replaced": true},
		)
		require.NoError(t, err)
		assert.Equal(t, int64(1), result.ModifiedCount)

		var doc bson.M
		err = coll.FindOne(ctx, bson.M{"name": "item1_replaced"}).Decode(&doc)
		require.NoError(t, err)
		assert.Equal(t, true, doc["replaced"])
	})
}

// =============================================================================
// 分页查询测试
// =============================================================================

func TestMongo_FindPage_Integration(t *testing.T) {
	client, cleanup := setupMongo(t)
	defer cleanup()

	wrapper, err := New(client)
	require.NoError(t, err)

	ctx := context.Background()
	db := client.Database("test_pagination")
	coll := db.Collection("pages")

	// 清理并准备测试数据
	coll.Drop(ctx)
	docs := make([]any, 100)
	for i := 0; i < 100; i++ {
		docs[i] = bson.M{
			"index":    i,
			"name":     fmt.Sprintf("item-%03d", i),
			"category": i % 5, // 0-4 五个分类
			"score":    float64(i) * 1.5,
		}
	}
	_, err = coll.InsertMany(ctx, docs)
	require.NoError(t, err)

	t.Cleanup(func() {
		coll.Drop(context.Background())
	})

	t.Run("基本分页", func(t *testing.T) {
		page, err := wrapper.FindPage(ctx, coll, bson.M{}, PageOptions{
			Page:     1,
			PageSize: 10,
		})
		require.NoError(t, err)
		assert.Equal(t, int64(100), page.Total)
		assert.Equal(t, int64(1), page.Page)
		assert.Equal(t, int64(10), page.PageSize)
		assert.Equal(t, int64(10), page.TotalPages)
		assert.Len(t, page.Data, 10)
	})

	t.Run("第二页", func(t *testing.T) {
		page, err := wrapper.FindPage(ctx, coll, bson.M{}, PageOptions{
			Page:     2,
			PageSize: 10,
		})
		require.NoError(t, err)
		assert.Equal(t, int64(2), page.Page)
		assert.Len(t, page.Data, 10)
	})

	t.Run("最后一页（不满）", func(t *testing.T) {
		page, err := wrapper.FindPage(ctx, coll, bson.M{}, PageOptions{
			Page:     4,
			PageSize: 30,
		})
		require.NoError(t, err)
		assert.Equal(t, int64(4), page.TotalPages)
		assert.Len(t, page.Data, 10) // 100 - 30*3 = 10
	})

	t.Run("带条件过滤", func(t *testing.T) {
		page, err := wrapper.FindPage(ctx, coll, bson.M{"category": 0}, PageOptions{
			Page:     1,
			PageSize: 10,
		})
		require.NoError(t, err)
		assert.Equal(t, int64(20), page.Total) // 每5个有1个 category=0
		assert.Len(t, page.Data, 10)

		for _, doc := range page.Data {
			assert.Equal(t, int32(0), doc["category"])
		}
	})

	t.Run("带排序", func(t *testing.T) {
		page, err := wrapper.FindPage(ctx, coll, bson.M{}, PageOptions{
			Page:     1,
			PageSize: 5,
			Sort:     bson.D{{Key: "index", Value: -1}}, // 降序
		})
		require.NoError(t, err)

		// 验证排序正确
		for i, doc := range page.Data {
			expected := int32(99 - i)
			assert.Equal(t, expected, doc["index"])
		}
	})

	t.Run("复合排序", func(t *testing.T) {
		page, err := wrapper.FindPage(ctx, coll, bson.M{}, PageOptions{
			Page:     1,
			PageSize: 10,
			Sort: bson.D{
				{Key: "category", Value: 1},
				{Key: "score", Value: -1},
			},
		})
		require.NoError(t, err)

		// 验证先按 category 升序，再按 score 降序
		if len(page.Data) >= 2 {
			first := page.Data[0]
			second := page.Data[1]
			cat1 := first["category"].(int32)
			cat2 := second["category"].(int32)
			if cat1 == cat2 {
				score1 := first["score"].(float64)
				score2 := second["score"].(float64)
				assert.GreaterOrEqual(t, score1, score2)
			}
		}
	})

	t.Run("范围查询", func(t *testing.T) {
		page, err := wrapper.FindPage(ctx, coll, bson.M{
			"score": bson.M{
				"$gte": 50.0,
				"$lte": 100.0,
			},
		}, PageOptions{
			Page:     1,
			PageSize: 100,
		})
		require.NoError(t, err)

		for _, doc := range page.Data {
			score := doc["score"].(float64)
			assert.GreaterOrEqual(t, score, 50.0)
			assert.LessOrEqual(t, score, 100.0)
		}
	})

	t.Run("空结果", func(t *testing.T) {
		page, err := wrapper.FindPage(ctx, coll, bson.M{"nonexistent": true}, PageOptions{
			Page:     1,
			PageSize: 10,
		})
		require.NoError(t, err)
		assert.Equal(t, int64(0), page.Total)
		assert.Len(t, page.Data, 0)
		assert.Equal(t, int64(0), page.TotalPages)
	})

	t.Run("错误参数", func(t *testing.T) {
		_, err := wrapper.FindPage(ctx, coll, bson.M{}, PageOptions{
			Page:     0, // 无效页码
			PageSize: 10,
		})
		assert.ErrorIs(t, err, ErrInvalidPage)

		_, err = wrapper.FindPage(ctx, coll, bson.M{}, PageOptions{
			Page:     1,
			PageSize: 0, // 无效页大小
		})
		assert.ErrorIs(t, err, ErrInvalidPageSize)
	})
}

// =============================================================================
// 批量写入测试
// =============================================================================

func TestMongo_BulkInsert_Integration(t *testing.T) {
	client, cleanup := setupMongo(t)
	defer cleanup()

	wrapper, err := New(client)
	require.NoError(t, err)

	ctx := context.Background()
	db := client.Database("test_bulk")
	coll := db.Collection("bulk_items")

	t.Cleanup(func() {
		coll.Drop(context.Background())
	})

	t.Run("基本批量插入", func(t *testing.T) {
		coll.Drop(ctx)

		docs := make([]any, 100)
		for i := 0; i < 100; i++ {
			docs[i] = bson.M{"index": i, "name": fmt.Sprintf("bulk-%d", i)}
		}

		result, err := wrapper.BulkInsert(ctx, coll, docs, BulkOptions{})
		require.NoError(t, err)
		assert.Equal(t, int64(100), result.InsertedCount)
		assert.Empty(t, result.Errors)

		// 验证数据
		count, err := coll.CountDocuments(ctx, bson.M{})
		require.NoError(t, err)
		assert.Equal(t, int64(100), count)
	})

	t.Run("分批插入", func(t *testing.T) {
		coll.Drop(ctx)

		docs := make([]any, 250)
		for i := 0; i < 250; i++ {
			docs[i] = bson.M{"index": i}
		}

		result, err := wrapper.BulkInsert(ctx, coll, docs, BulkOptions{BatchSize: 100})
		require.NoError(t, err)
		assert.Equal(t, int64(250), result.InsertedCount)
	})

	t.Run("大批量插入", func(t *testing.T) {
		coll.Drop(ctx)

		docs := make([]any, 10000)
		for i := 0; i < 10000; i++ {
			docs[i] = bson.M{
				"index":   i,
				"content": fmt.Sprintf("content-%d", i),
			}
		}

		start := time.Now()
		result, err := wrapper.BulkInsert(ctx, coll, docs, BulkOptions{BatchSize: 1000})
		elapsed := time.Since(start)

		require.NoError(t, err)
		assert.Equal(t, int64(10000), result.InsertedCount)
		t.Logf("10000 条记录插入耗时: %v", elapsed)
	})

	t.Run("无序插入（部分失败）", func(t *testing.T) {
		coll.Drop(ctx)

		// 创建唯一索引
		_, err := coll.Indexes().CreateOne(ctx, mongo.IndexModel{
			Keys:    bson.D{{Key: "unique_field", Value: 1}},
			Options: options.Index().SetUnique(true),
		})
		require.NoError(t, err)

		docs := []any{
			bson.M{"unique_field": "a"},
			bson.M{"unique_field": "b"},
			bson.M{"unique_field": "a"}, // 重复，会失败
			bson.M{"unique_field": "c"},
		}

		result, err := wrapper.BulkInsert(ctx, coll, docs, BulkOptions{
			Ordered: false, // 无序，继续执行
		})
		// 部分失败：MongoDB InsertMany 对重复键返回 BulkWriteException，
		// xmongo 将其包装为错误并同时保留部分成功的计数。
		// 与 BulkResult 文档契约一致：即使 err != nil，result 仍包含有效数据。
		require.Error(t, err)
		require.NotNil(t, result)
		assert.Equal(t, int64(3), result.InsertedCount) // 3 条成功
		assert.NotEmpty(t, result.Errors)
	})

	t.Run("空文档列表", func(t *testing.T) {
		_, err := wrapper.BulkInsert(ctx, coll, []any{}, BulkOptions{})
		assert.ErrorIs(t, err, ErrEmptyDocs)
	})
}

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
			wg.Add(1)
			go func(page int64) {
				defer wg.Done()
				_, err := wrapper.FindPage(ctx, coll, bson.M{}, PageOptions{
					Page:     page%10 + 1,
					PageSize: 10,
				})
				if err != nil {
					errorCount.Add(1)
				}
			}(int64(i))
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
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				docs := make([]any, 10)
				for j := 0; j < 10; j++ {
					docs[j] = bson.M{"worker": id, "item": j}
				}
				result, err := wrapper.BulkInsert(ctx, writeColl, docs, BulkOptions{})
				if err == nil {
					successCount.Add(result.InsertedCount)
				}
			}(i)
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
