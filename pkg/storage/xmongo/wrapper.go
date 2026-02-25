package xmongo

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
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

	// 设计决策: 使用 atomic.Bool 保护 closed 状态，
	// 确保并发调用 Close() 和其他方法时的线程安全。
	closed atomic.Bool
}

const (
	mongoComponent = "xmongo"

	// defaultBatchSize 批量写入默认每批文档数。
	defaultBatchSize = 1000

	// maxBatchSize 批量写入每批文档数上限。
	// 避免单次 InsertMany 请求过大导致 MongoDB 16MB BSON 限制或内存问题。
	maxBatchSize = 10000
)

// Client 返回底层 MongoDB 客户端。
//
// 设计决策: 不检查 closed 状态，与 xetcd.RawClient() 保持一致。
// 原因：(1) mongo.Client 在 Disconnect 后会自行保护返回明确错误;
// (2) 改为返回 (*mongo.Client, error) 会破坏接口兼容性且增加调用方负担。
func (w *mongoWrapper) Client() *mongo.Client {
	return w.client
}

// Health 执行健康检查。
func (w *mongoWrapper) Health(ctx context.Context) (err error) {
	if ctx == nil {
		return ErrNilContext
	}
	if w.closed.Load() {
		return ErrClosed
	}

	ctx, span := xmetrics.Start(ctx, w.options.Observer, xmetrics.SpanOptions{
		Component: mongoComponent,
		Operation: "health",
		Kind:      xmetrics.KindClient,
		Attrs: []xmetrics.Attr{
			xmetrics.String("db.system", "mongodb"),
		},
	})
	defer func() {
		span.End(xmetrics.Result{Err: err})
	}()

	w.healthCounter.IncPing()

	// 使用 storageopt 的健康检查超时
	ctx, cancel := storageopt.HealthContext(ctx, w.options.HealthTimeout)
	defer cancel()

	if err = w.clientOps.Ping(ctx, readpref.Primary()); err != nil {
		w.healthCounter.IncPingError()
		return fmt.Errorf("xmongo health: %w", err)
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
// 详细信息见 PoolStats 文档。
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
// 重复调用返回 ErrClosed。并发安全。
//
// 设计决策: nil context 替换为 context.Background() 而非返回 ErrNilContext，
// 因为关闭操作不应因 nil ctx 而失败（资源释放优先于参数校验）。
//
// 设计决策: Disconnect 失败时不回滚 closed 状态（不支持重试）。
// 原因：(1) 标准 Go 模式，io.Closer 契约为"调用一次释放资源";
// (2) 允许重试会引入复杂的并发状态管理;
// (3) Disconnect 失败通常意味着网络不可达，重试同样会失败。
func (w *mongoWrapper) Close(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if !w.closed.CompareAndSwap(false, true) {
		return ErrClosed
	}

	// 关闭慢查询检测器
	if w.slowQueryDetector != nil {
		w.slowQueryDetector.Close()
	}

	if w.clientOps == nil {
		return nil
	}
	if err := w.clientOps.Disconnect(ctx); err != nil {
		return fmt.Errorf("xmongo close: %w", err)
	}
	return nil
}

// FindPage 分页查询。
func (w *mongoWrapper) FindPage(ctx context.Context, coll *mongo.Collection, filter any, opts PageOptions) (*PageResult, error) {
	if ctx == nil {
		return nil, ErrNilContext
	}
	if w.closed.Load() {
		return nil, ErrClosed
	}
	return w.findPage(ctx, coll, filter, opts)
}

// BulkInsert 批量插入。
func (w *mongoWrapper) BulkInsert(ctx context.Context, coll *mongo.Collection, docs []any, opts BulkOptions) (*BulkResult, error) {
	if ctx == nil {
		return nil, ErrNilContext
	}
	if w.closed.Load() {
		return nil, ErrClosed
	}
	return w.bulkInsert(ctx, coll, docs, opts)
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
// 内部实现
// =============================================================================

// applyTimeout 当调用方未设置 deadline 且 timeout > 0 时，添加超时兜底。
// 返回可能带 deadline 的 ctx 和 cancel 函数（调用方需 defer cancel）。
func applyTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout > 0 {
		if _, hasDeadline := ctx.Deadline(); !hasDeadline {
			return context.WithTimeout(ctx, timeout)
		}
	}
	return ctx, func() {}
}

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
	switch {
	case errors.Is(err, storageopt.ErrInvalidPage):
		return ErrInvalidPage
	case errors.Is(err, storageopt.ErrInvalidPageSize):
		return ErrInvalidPageSize
	case errors.Is(err, storageopt.ErrPageOverflow):
		return ErrPageOverflow
	default:
		return err
	}
}

// findPageInternal 分页查询内部实现，使用接口便于测试。
func (w *mongoWrapper) findPageInternal(ctx context.Context, coll collectionOperations, filter any, opts PageOptions) (result *PageResult, err error) {
	// 设计决策: 将 nil filter 归一化为 bson.D{}，避免依赖 driver 对 nil 的隐式处理，
	// 使 API 语义更明确。
	if filter == nil {
		filter = bson.D{}
	}

	// 当调用方未设置 deadline 且配置了 QueryTimeout 时，添加超时兜底
	var cancel context.CancelFunc
	ctx, cancel = applyTimeout(ctx, w.options.QueryTimeout)
	defer cancel()

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
			xmetrics.String("db.system", "mongodb"),
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
		return nil, fmt.Errorf("xmongo find_page count %s.%s: %w", info.Database, info.Collection, err)
	}

	// 执行查询
	cursor, err := coll.Find(ctx, filter, buildFindOptions(skip, opts))
	if err != nil {
		return nil, fmt.Errorf("xmongo find_page find %s.%s: %w", info.Database, info.Collection, err)
	}
	defer func() {
		if closeErr := cursor.Close(ctx); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("xmongo find_page close cursor: %w", closeErr))
		}
	}()

	// 解析结果
	var data []bson.M
	if err = cursor.All(ctx, &data); err != nil {
		return nil, fmt.Errorf("xmongo find_page decode %s.%s: %w", info.Database, info.Collection, err)
	}
	// 设计决策: 空结果返回空切片而非 nil，确保 JSON 序列化为 [] 而非 null。
	if data == nil {
		data = []bson.M{}
	}

	return &PageResult{
		Data:       data,
		Total:      total,
		Page:       opts.Page,
		PageSize:   opts.PageSize,
		TotalPages: storageopt.CalculateTotalPages(total, opts.PageSize),
	}, nil
}

// buildFindOptions 构建分页查询的 FindOptions。
func buildFindOptions(skip int64, opts PageOptions) *options.FindOptionsBuilder {
	findOpts := options.Find().
		SetSkip(skip).
		SetLimit(opts.PageSize)
	if len(opts.Sort) > 0 {
		findOpts = findOpts.SetSort(opts.Sort)
	}
	if len(opts.Projection) > 0 {
		findOpts = findOpts.SetProjection(opts.Projection)
	}
	return findOpts
}

// bulkInsert 批量插入实现。
func (w *mongoWrapper) bulkInsert(ctx context.Context, coll *mongo.Collection, docs []any, opts BulkOptions) (*BulkResult, error) {
	if coll == nil {
		return nil, ErrNilCollection
	}
	if len(docs) == 0 {
		return nil, ErrEmptyDocs
	}

	// 适配集合为接口
	collOps := adaptCollection(coll)
	return w.bulkInsertInternal(ctx, collOps, docs, opts)
}

// bulkInsertInternal 批量插入内部实现，使用接口便于测试。
func (w *mongoWrapper) bulkInsertInternal(ctx context.Context, coll collectionOperations, docs []any, opts BulkOptions) (result *BulkResult, err error) {
	// 当调用方未设置 deadline 且配置了 WriteTimeout 时，添加超时兜底
	var cancel context.CancelFunc
	ctx, cancel = applyTimeout(ctx, w.options.WriteTimeout)
	defer cancel()

	batchSize := opts.BatchSize
	if batchSize < 1 {
		batchSize = defaultBatchSize
	} else if batchSize > maxBatchSize {
		batchSize = maxBatchSize
	}

	info := buildSlowQueryInfoFromOps(coll, "bulkInsert", nil)

	start := time.Now()
	ctx, span := xmetrics.Start(ctx, w.options.Observer, xmetrics.SpanOptions{
		Component: mongoComponent,
		Operation: "bulk_insert",
		Kind:      xmetrics.KindClient,
		Attrs: []xmetrics.Attr{
			xmetrics.String("db.system", "mongodb"),
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

	wrappedErr := fmt.Errorf("xmongo bulk_insert: %w", err)

	if ordered {
		// 有序模式下遇到错误停止
		return insertedCount, wrappedErr, true
	}

	// 无序模式下检查 context，避免继续无效工作
	if ctx.Err() != nil {
		return insertedCount, fmt.Errorf("xmongo bulk_insert context canceled: %w", ctx.Err()), true
	}

	return insertedCount, wrappedErr, false
}
