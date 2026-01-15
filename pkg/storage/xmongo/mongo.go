package xmongo

import (
	"context"

	"github.com/omeyang/xkit/internal/storageopt"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// =============================================================================
// 接口定义
// =============================================================================

// Mongo 定义 MongoDB 包装器接口。
// 只提供 mongo.Client 原生不具备的增值功能，基础操作请直接使用 Client()。
type Mongo interface {
	// Client 返回底层的 mongo.Client。
	// 用于执行所有 MongoDB 操作。
	Client() *mongo.Client

	// Health 执行健康检查。
	// 通过 Ping 命令检测连接状态。
	Health(ctx context.Context) error

	// Stats 返回统计信息。
	// 包含健康检查次数、慢查询次数、连接池状态等。
	Stats() Stats

	// Close 关闭 MongoDB 连接。
	// 注意：由于客户端由外部传入，此方法会断开连接。
	Close(ctx context.Context) error

	// FindPage 分页查询。
	// 自动计算总数和分页信息。
	//
	// ⚠️ 重要限制：非原子性查询
	// 此方法执行两次独立查询（COUNT + 数据查询），不在同一事务中。
	// 在以下场景中可能出现数据不一致：
	//   - 高并发写入：COUNT=100 但实际返回 101 条（查询间有新文档插入）
	//   - 高并发删除：COUNT=100 但实际返回 99 条（查询间有文档删除）
	//   - 翻页时数据变化：同一条数据可能在不同页面重复出现或被遗漏
	//
	// 解决方案：
	//   - 对于展示场景：通常可以接受短暂的不一致，无需特殊处理
	//   - 对于精确统计：使用 MongoDB 事务包装两次查询
	//   - 对于大数据量：考虑使用游标分页（基于 _id 或时间戳）避免 COUNT 开销
	FindPage(ctx context.Context, coll *mongo.Collection, filter any, opts PageOptions) (*PageResult, error)

	// BulkWrite 批量写入。
	// 将大量文档分批插入，提高写入效率。
	BulkWrite(ctx context.Context, coll *mongo.Collection, docs []any, opts BulkOptions) (*BulkResult, error)
}

// =============================================================================
// 分页查询类型
// =============================================================================

// PageOptions 分页查询选项。
type PageOptions struct {
	// Page 页码，从 1 开始。
	Page int64

	// PageSize 每页大小。
	PageSize int64

	// Sort 排序条件。
	// 例如: bson.D{{"created_at", -1}}
	//
	// 重要：强烈建议指定排序字段。
	// 如果 Sort 为 nil 或空，MongoDB 不保证结果顺序，
	// 这可能导致分页结果在翻页时出现数据重复或遗漏。
	// 常见做法是按 _id 或创建时间排序以确保稳定的分页结果。
	Sort bson.D
}

// PageResult 分页查询结果。
//
// 一致性说明：Total 通过独立的 COUNT 查询获取，与数据查询不在同一事务中。
// 在高并发写入场景下，Total 可能与 Data 的实际记录数略有差异。
// 例如：COUNT 返回 100，但在获取数据时已有新文档插入，实际可能返回 101 条。
// 如需强一致性，请使用 MongoDB 事务或考虑游标分页方案。
type PageResult struct {
	// Data 当前页数据。
	Data []bson.M

	// Total 总记录数。
	// 注意：此值来自独立的 COUNT 查询，可能与 Data 实际数量略有差异。
	Total int64

	// Page 当前页码。
	Page int64

	// PageSize 每页大小。
	PageSize int64

	// TotalPages 总页数。
	TotalPages int64
}

// =============================================================================
// 批量写入类型
// =============================================================================

// BulkOptions 批量写入选项。
type BulkOptions struct {
	// BatchSize 每批大小。
	// 默认为 1000。
	BatchSize int

	// Ordered 是否有序写入。
	// 有序写入时，遇到错误会停止后续操作。
	Ordered bool
}

// BulkResult 批量写入结果。
//
// 重要：即使返回的 error 不为 nil，result 仍可能包含有效数据！
// 调用方应同时检查 error 和 result.InsertedCount 以获取完整信息。
//
// 示例：
//
//	result, err := wrapper.BulkWrite(ctx, coll, docs, opts)
//	if err != nil {
//	    log.Printf("部分失败: %d/%d 条成功插入, 错误: %v",
//	        result.InsertedCount, len(docs), err)
//	    // 根据业务需求决定是否需要重试失败的部分
//	}
type BulkResult struct {
	// InsertedCount 成功插入数量。
	//
	// 此值基于 MongoDB InsertMany 返回的 InsertedIDs 长度计算。
	// 在以下场景中需要注意：
	//   - 有序模式（Ordered=true）：遇到第一个错误时停止，InsertedCount 为错误前成功插入的数量
	//   - 无序模式（Ordered=false）：MongoDB 会尝试插入所有文档，InsertedCount 为所有成功插入的数量
	//
	// 当 Errors 非空时，InsertedCount 可能小于请求插入的文档总数。
	// 即使 err != nil，InsertedCount 也可能 > 0，表示部分成功。
	InsertedCount int64

	// Errors 写入过程中的错误列表。
	// 每个批次的错误会被单独记录。
	// 无序模式下可能包含多个批次的错误。
	Errors []error
}

// =============================================================================
// 工厂函数
// =============================================================================

// New 创建 MongoDB 包装器。
// client 必须是已初始化的 mongo.Client。
func New(client *mongo.Client, opts ...Option) (Mongo, error) {
	if client == nil {
		return nil, ErrNilClient
	}

	options := defaultOptions()
	for _, opt := range opts {
		opt(options)
	}

	// 创建慢查询检测器
	detector := newSlowQueryDetector(options)

	return &mongoWrapper{
		client:            client,
		clientOps:         client, // *mongo.Client 实现 clientOperations 接口
		options:           options,
		slowQueryDetector: detector,
	}, nil
}

// newSlowQueryDetector 创建慢查询检测器。
func newSlowQueryDetector(opts *Options) *storageopt.SlowQueryDetector[SlowQueryInfo] {
	// 构建 storageopt 的慢查询选项
	sqOpts := storageopt.SlowQueryOptions[SlowQueryInfo]{
		Threshold:           opts.SlowQueryThreshold,
		AsyncWorkerPoolSize: opts.AsyncSlowQueryWorkers,
		AsyncQueueSize:      opts.AsyncSlowQueryQueueSize,
	}

	// 适配同步钩子
	if opts.SlowQueryHook != nil {
		sqOpts.SyncHook = func(ctx context.Context, info SlowQueryInfo) {
			opts.SlowQueryHook(ctx, info)
		}
	}

	// 适配异步钩子
	if opts.AsyncSlowQueryHook != nil {
		sqOpts.AsyncHook = func(info SlowQueryInfo) {
			opts.AsyncSlowQueryHook(info)
		}
	}

	return storageopt.NewSlowQueryDetector(sqOpts)
}
