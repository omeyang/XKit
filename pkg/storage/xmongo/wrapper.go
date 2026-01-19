package xmongo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/omeyang/xkit/internal/storageopt"
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

	// 慢查询检测器
	slowQueryDetector *storageopt.SlowQueryDetector[SlowQueryInfo]

	// 统计计数器（使用 storageopt 通用实现）
	healthCounter    storageopt.HealthCounter
	slowQueryCounter storageopt.SlowQueryCounter
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

	w.healthCounter.IncPing()

	// 使用 storageopt 的健康检查超时
	ctx, cancel := storageopt.HealthContext(ctx, w.options.HealthTimeout)
	defer cancel()

	if err := w.clientOps.Ping(ctx, readpref.Primary()); err != nil {
		w.healthCounter.IncPingError()
		return err
	}

	return nil
}

// Stats 返回统计信息。
func (w *mongoWrapper) Stats() Stats {
	return Stats{
		PingCount:   w.healthCounter.PingCount(),
		PingErrors:  w.healthCounter.PingErrors(),
		SlowQueries: w.slowQueryCounter.Count(),
		Pool:        w.getPoolStats(),
	}
}

// getPoolStats 获取连接池状态。
//
// 限制说明：
// MongoDB Go driver v2 不直接暴露连接池详细信息（TotalConnections、AvailableConnections）。
// 这是 driver 的设计决策，因为 MongoDB 使用连接池复用和会话管理，
// 传统的"连接数"概念不能准确反映资源使用情况。
//
// 当前返回值：
//   - InUseConnections: 使用 NumberSessionsInProgress() 返回活跃会话数作为近似值
//   - TotalConnections: 始终为 0（driver 未暴露此信息）
//   - AvailableConnections: 始终为 0（driver 未暴露此信息）
//
// 获取详细连接池信息的替代方案：
//  1. 使用 MongoDB serverStatus 命令：db.runCommand({serverStatus: 1}).connections
//  2. 监控 MongoDB 服务端指标（推荐用于生产环境）
//  3. 使用 driver 的事件监控功能（PoolEvent）统计连接创建/关闭
func (w *mongoWrapper) getPoolStats() PoolStats {
	if w.clientOps == nil {
		return PoolStats{}
	}

	// MongoDB driver v2 暂不直接暴露连接池详细信息
	// NumberSessionsInProgress 返回活跃会话数，作为 InUseConnections 的近似值
	inUse := w.clientOps.NumberSessionsInProgress()

	return PoolStats{
		InUseConnections: inUse,
	}
}

// Close 关闭 MongoDB 连接。
func (w *mongoWrapper) Close(ctx context.Context) error {
	// 关闭慢查询检测器
	if w.slowQueryDetector != nil {
		w.slowQueryDetector.Close()
	}

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

// maybeSlowQuery 检测并可能触发慢查询钩子。
// 使用 SlowQueryDetector 统一处理同步/异步钩子。
func (w *mongoWrapper) maybeSlowQuery(ctx context.Context, info SlowQueryInfo) bool {
	if w.slowQueryDetector == nil {
		return false
	}

	triggered := w.slowQueryDetector.MaybeSlowQuery(ctx, info, info.Duration)
	if triggered {
		w.slowQueryCounter.Inc()
	}
	return triggered
}

// =============================================================================
// 辅助函数
// =============================================================================

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

	// 适配集合为接口
	// 参数验证（包括 overflow 检查）在 findPageInternal 中统一处理
	collOps := adaptCollection(coll)
	return w.findPageInternal(ctx, collOps, filter, opts)
}

// convertPaginationError 将 storageopt 的分页错误转换为 xmongo 的错误类型。
// 由于 xmongo 的分页错误已包装 storageopt 的错误，errors.Is 可以匹配任一错误。
func convertPaginationError(err error) error {
	switch err {
	case storageopt.ErrInvalidPage:
		return ErrInvalidPage
	case storageopt.ErrInvalidPageSize:
		return ErrInvalidPageSize
	case storageopt.ErrPageOverflow:
		return ErrPageOverflow
	default:
		return err
	}
}

// findPageInternal 分页查询内部实现，使用接口便于测试。
func (w *mongoWrapper) findPageInternal(ctx context.Context, coll collectionOperations, filter any, opts PageOptions) (result *PageResult, err error) {
	// 使用 storageopt 验证分页参数并计算 skip，防止溢出
	skip, err := storageopt.ValidatePagination(opts.Page, opts.PageSize)
	if err != nil {
		return nil, convertPaginationError(err)
	}

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
		info.Duration = storageopt.MeasureOperation(start)
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

	return &PageResult{
		Data:       data,
		Total:      total,
		Page:       opts.Page,
		PageSize:   opts.PageSize,
		TotalPages: storageopt.CalculateTotalPages(total, opts.PageSize),
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
		info.Duration = storageopt.MeasureOperation(start)
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
