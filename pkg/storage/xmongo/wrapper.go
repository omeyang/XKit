package xmongo

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/omeyang/xkit/pkg/observability/xmetrics"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
)

// =============================================================================
// mongoWrapper 实现
// =============================================================================

// mongoWrapper 实现 Mongo 接口。
type mongoWrapper struct {
	client    *mongo.Client    // 用于 Client() 方法返回
	clientOps clientOperations // 用于内部操作（可注入 mock）
	options   *Options

	// 统计计数器
	pingCount   atomic.Int64
	pingErrors  atomic.Int64
	slowQueries atomic.Int64
}

const mongoComponent = "xmongo"

// Client 返回底层 MongoDB 客户端。
func (w *mongoWrapper) Client() *mongo.Client {
	return w.client
}

// Health 执行健康检查。
func (w *mongoWrapper) Health(ctx context.Context) (err error) {
	ctx, span := xmetrics.Start(ctx, w.options.Observer, xmetrics.SpanOptions{
		Component: mongoComponent,
		Operation: "health",
		Kind:      xmetrics.KindClient,
		Attrs: []xmetrics.Attr{
			xmetrics.String("db.system", "mongo"),
		},
	})
	defer func() {
		span.End(xmetrics.Result{Err: err})
	}()

	w.pingCount.Add(1)

	// 如果设置了超时，使用带超时的 context
	if w.options.HealthTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, w.options.HealthTimeout)
		defer cancel()
	}

	if err := w.clientOps.Ping(ctx, readpref.Primary()); err != nil {
		w.pingErrors.Add(1)
		return err
	}

	return nil
}

// Stats 返回统计信息。
func (w *mongoWrapper) Stats() Stats {
	return Stats{
		PingCount:   w.pingCount.Load(),
		PingErrors:  w.pingErrors.Load(),
		SlowQueries: w.slowQueries.Load(),
		Pool:        w.getPoolStats(),
	}
}

// getPoolStats 获取连接池状态。
// 注意：MongoDB driver v2 的连接池信息获取方式可能与此不同，
// 这里使用 NumberSessionsInProgress 作为近似值。
func (w *mongoWrapper) getPoolStats() PoolStats {
	if w.clientOps == nil {
		return PoolStats{}
	}

	// MongoDB driver v2 暂不直接暴露连接池详细信息
	// NumberSessionsInProgress 返回活跃会话数
	inUse := w.clientOps.NumberSessionsInProgress()

	return PoolStats{
		InUseConnections: inUse,
		// TotalConnections 和 AvailableConnections 需要通过服务器状态命令获取
		// 为简化实现，暂时只返回活跃会话数
	}
}

// Close 关闭 MongoDB 连接。
func (w *mongoWrapper) Close(ctx context.Context) error {
	if w.clientOps == nil {
		return nil
	}
	return w.clientOps.Disconnect(ctx)
}

// FindPage 分页查询。
// 实现在 pagination.go 中。
func (w *mongoWrapper) FindPage(ctx context.Context, coll *mongo.Collection, filter any, opts PageOptions) (*PageResult, error) {
	return w.findPage(ctx, coll, filter, opts)
}

// BulkWrite 批量写入。
// 实现在 batch.go 中。
func (w *mongoWrapper) BulkWrite(ctx context.Context, coll *mongo.Collection, docs []any, opts BulkOptions) (*BulkResult, error) {
	return w.bulkWrite(ctx, coll, docs, opts)
}

// =============================================================================
// 慢查询检测
// =============================================================================

// triggerSlowQueryHook 触发慢查询钩子。
func (w *mongoWrapper) triggerSlowQueryHook(ctx context.Context, info SlowQueryInfo) {
	if w.options.SlowQueryHook != nil {
		w.slowQueries.Add(1)
		w.options.SlowQueryHook(ctx, info)
	}
}

// maybeSlowQuery 检测并可能触发慢查询钩子。
func (w *mongoWrapper) maybeSlowQuery(ctx context.Context, info SlowQueryInfo) bool {
	// 如果阈值为 0，禁用慢查询检测
	if w.options.SlowQueryThreshold == 0 {
		return false
	}

	// 如果耗时超过阈值，触发钩子
	if info.Duration >= w.options.SlowQueryThreshold {
		w.triggerSlowQueryHook(ctx, info)
		return true
	}
	return false
}

// =============================================================================
// 辅助函数
// =============================================================================

// measureOperation 测量操作耗时。
func measureOperation(start time.Time) time.Duration {
	return time.Since(start)
}

// buildSlowQueryInfo 构建慢查询信息。
func buildSlowQueryInfo(coll *mongo.Collection, operation string, filter any, duration time.Duration) SlowQueryInfo {
	var dbName, collName string
	if coll != nil {
		dbName = coll.Database().Name()
		collName = coll.Name()
	}

	return SlowQueryInfo{
		Database:   dbName,
		Collection: collName,
		Operation:  operation,
		Filter:     filter,
		Duration:   duration,
	}
}

// =============================================================================
// 占位实现 - 将在对应文件中实现
// =============================================================================

// findPage 分页查询实现。
func (w *mongoWrapper) findPage(ctx context.Context, coll *mongo.Collection, filter any, opts PageOptions) (*PageResult, error) {
	if coll == nil {
		return nil, ErrNilCollection
	}
	if opts.Page < 1 {
		return nil, ErrInvalidPage
	}
	if opts.PageSize < 1 {
		return nil, ErrInvalidPageSize
	}

	// 适配集合为接口
	collOps := adaptCollection(coll)
	return w.findPageInternal(ctx, collOps, filter, opts)
}

// findPageInternal 分页查询内部实现，使用接口便于测试。
func (w *mongoWrapper) findPageInternal(ctx context.Context, coll collectionOperations, filter any, opts PageOptions) (result *PageResult, err error) {
	info := buildSlowQueryInfoFromOps(coll, "findPage", filter)

	start := time.Now()
	ctx, span := xmetrics.Start(ctx, w.options.Observer, xmetrics.SpanOptions{
		Component: mongoComponent,
		Operation: "find_page",
		Kind:      xmetrics.KindClient,
		Attrs: []xmetrics.Attr{
			xmetrics.String("db.system", "mongo"),
			xmetrics.String("db.name", info.Database),
			xmetrics.String("db.collection", info.Collection),
		},
	})
	defer func() {
		info.Duration = measureOperation(start)
		slow := w.maybeSlowQuery(ctx, info)

		var attrs []xmetrics.Attr
		if slow {
			attrs = append(attrs,
				xmetrics.Bool("slow", true),
				xmetrics.Int64("slow_threshold_ms", w.options.SlowQueryThreshold.Milliseconds()),
			)
		}
		span.End(xmetrics.Result{Err: err, Attrs: attrs})
	}()

	// 计算 skip
	skip := (opts.Page - 1) * opts.PageSize

	// 查询总数
	total, err := coll.CountDocuments(ctx, filter)
	if err != nil {
		return nil, err
	}

	// 构建查询选项
	findOpts := options.Find().
		SetSkip(skip).
		SetLimit(opts.PageSize)
	if len(opts.Sort) > 0 {
		findOpts = findOpts.SetSort(opts.Sort)
	}

	// 执行查询
	cursor, err := coll.Find(ctx, filter, findOpts)
	if err != nil {
		return nil, err
	}

	// 解析结果
	var data []bson.M
	dataErr := cursor.All(ctx, &data)
	closeErr := cursor.Close(ctx)
	if closeErr != nil {
		closeErr = fmt.Errorf("close cursor failed: %w", closeErr)
	}
	if err := errors.Join(dataErr, closeErr); err != nil {
		return nil, err
	}

	// 计算总页数
	totalPages := total / opts.PageSize
	if total%opts.PageSize > 0 {
		totalPages++
	}

	return &PageResult{
		Data:       data,
		Total:      total,
		Page:       opts.Page,
		PageSize:   opts.PageSize,
		TotalPages: totalPages,
	}, nil
}

// bulkWrite 批量写入实现。
func (w *mongoWrapper) bulkWrite(ctx context.Context, coll *mongo.Collection, docs []any, opts BulkOptions) (*BulkResult, error) {
	if coll == nil {
		return nil, ErrNilCollection
	}
	if len(docs) == 0 {
		return nil, ErrEmptyDocs
	}

	// 适配集合为接口
	collOps := adaptCollection(coll)
	return w.bulkWriteInternal(ctx, collOps, docs, opts)
}

// bulkWriteInternal 批量写入内部实现，使用接口便于测试。
func (w *mongoWrapper) bulkWriteInternal(ctx context.Context, coll collectionOperations, docs []any, opts BulkOptions) (result *BulkResult, err error) {
	batchSize := opts.BatchSize
	if batchSize < 1 {
		batchSize = 1000 // 默认批次大小
	}

	info := buildSlowQueryInfoFromOps(coll, "bulkWrite", nil)

	start := time.Now()
	ctx, span := xmetrics.Start(ctx, w.options.Observer, xmetrics.SpanOptions{
		Component: mongoComponent,
		Operation: "bulk_write",
		Kind:      xmetrics.KindClient,
		Attrs: []xmetrics.Attr{
			xmetrics.String("db.system", "mongo"),
			xmetrics.String("db.name", info.Database),
			xmetrics.String("db.collection", info.Collection),
		},
	})
	defer func() {
		info.Duration = measureOperation(start)
		slow := w.maybeSlowQuery(ctx, info)

		var attrs []xmetrics.Attr
		if slow {
			attrs = append(attrs,
				xmetrics.Bool("slow", true),
				xmetrics.Int64("slow_threshold_ms", w.options.SlowQueryThreshold.Milliseconds()),
			)
		}
		span.End(xmetrics.Result{Err: err, Attrs: attrs})
	}()

	// 执行批量插入
	insertedCount, errs := w.executeBatches(ctx, coll, docs, batchSize, opts.Ordered)

	// 当存在错误时，同时返回结果和合并的错误，让调用方能通过 err != nil 判断
	var resultErr error
	if len(errs) > 0 {
		resultErr = errors.Join(errs...)
	}

	return &BulkResult{
		InsertedCount: insertedCount,
		Errors:        errs,
	}, resultErr
}

// executeBatches 执行分批插入操作。
func (w *mongoWrapper) executeBatches(ctx context.Context, coll collectionOperations, docs []any, batchSize int, ordered bool) (int64, []error) {
	var insertedCount int64
	var errs []error

	// 构建 InsertMany 选项，传递 Ordered 配置
	insertOpts := options.InsertMany().SetOrdered(ordered)

	// 分批插入
	for i := 0; i < len(docs); i += batchSize {
		// 每批次开始前检查 context 是否已取消，避免无效工作
		if err := ctx.Err(); err != nil {
			errs = append(errs, fmt.Errorf("context canceled before batch %d: %w", i/batchSize, err))
			break
		}

		end := min(i+batchSize, len(docs))
		batch := docs[i:end]

		count, batchErr, shouldStop := w.executeSingleBatch(ctx, coll, batch, insertOpts, ordered)
		insertedCount += count
		if batchErr != nil {
			errs = append(errs, batchErr)
		}
		if shouldStop {
			break
		}
	}

	return insertedCount, errs
}

// executeSingleBatch 执行单个批次的插入，返回 (插入数量, 错误, 是否应停止)。
func (w *mongoWrapper) executeSingleBatch(ctx context.Context, coll collectionOperations, batch []any, insertOpts *options.InsertManyOptionsBuilder, ordered bool) (int64, error, bool) {
	result, err := coll.InsertMany(ctx, batch, insertOpts)
	if err == nil {
		return int64(len(result.InsertedIDs)), nil, false
	}

	// 即使有错误，也统计部分成功（MongoDB 在 ordered=false 时可能部分成功）
	var insertedCount int64
	if result != nil {
		insertedCount = int64(len(result.InsertedIDs))
	}

	if ordered {
		// 有序模式下遇到错误停止
		return insertedCount, err, true
	}

	// 无序模式下检查 context，避免继续无效工作
	if ctx.Err() != nil {
		return insertedCount, fmt.Errorf("context canceled after batch error: %w", ctx.Err()), true
	}

	return insertedCount, err, false
}
