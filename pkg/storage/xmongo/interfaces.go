package xmongo

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
)

// =============================================================================
// 内部接口定义 - 用于依赖注入和测试
// =============================================================================

// clientOperations 定义客户端级别操作接口。
// *mongo.Client 实现此接口。
type clientOperations interface {
	Ping(ctx context.Context, rp *readpref.ReadPref) error
	Disconnect(ctx context.Context) error
	NumberSessionsInProgress() int
}

// collectionOperations 定义集合级别操作接口。
// *mongo.Collection 实现此接口。
type collectionOperations interface {
	CountDocuments(ctx context.Context, filter any, opts ...options.Lister[options.CountOptions]) (int64, error)
	Find(ctx context.Context, filter any, opts ...options.Lister[options.FindOptions]) (*mongo.Cursor, error)
	InsertMany(ctx context.Context, documents []any, opts ...options.Lister[options.InsertManyOptions]) (*mongo.InsertManyResult, error)
	Database() *mongo.Database
	Name() string
}

// =============================================================================
// 集合适配器 - 将 *mongo.Collection 适配为 collectionOperations
// =============================================================================

// collectionAdapter 将 *mongo.Collection 适配为 collectionOperations 接口。
type collectionAdapter struct {
	coll *mongo.Collection
}

func (a *collectionAdapter) CountDocuments(ctx context.Context, filter any, opts ...options.Lister[options.CountOptions]) (int64, error) {
	return a.coll.CountDocuments(ctx, filter, opts...)
}

func (a *collectionAdapter) Find(ctx context.Context, filter any, opts ...options.Lister[options.FindOptions]) (*mongo.Cursor, error) {
	return a.coll.Find(ctx, filter, opts...)
}

func (a *collectionAdapter) InsertMany(ctx context.Context, documents []any, opts ...options.Lister[options.InsertManyOptions]) (*mongo.InsertManyResult, error) {
	return a.coll.InsertMany(ctx, documents, opts...)
}

func (a *collectionAdapter) Database() *mongo.Database {
	return a.coll.Database()
}

func (a *collectionAdapter) Name() string {
	return a.coll.Name()
}

// =============================================================================
// 辅助函数
// =============================================================================

// adaptCollection 将 *mongo.Collection 适配为 collectionOperations 接口。
func adaptCollection(coll *mongo.Collection) collectionOperations {
	if coll == nil {
		return nil
	}
	return &collectionAdapter{coll: coll}
}

// buildSlowQueryInfoFromOps 使用 collectionOperations 构建慢查询信息。
func buildSlowQueryInfoFromOps(coll collectionOperations, operation string, filter any) SlowQueryInfo {
	var dbName, collName string
	if coll != nil {
		if db := coll.Database(); db != nil {
			dbName = db.Name()
		}
		collName = coll.Name()
	}

	return SlowQueryInfo{
		Database:   dbName,
		Collection: collName,
		Operation:  operation,
		Filter:     filter,
	}
}
