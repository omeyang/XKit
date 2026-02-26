package xclickhouse

import (
	"context"

	"github.com/omeyang/xkit/internal/storageopt"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

// =============================================================================
// 接口定义
// =============================================================================

// ClickHouse 定义 ClickHouse 包装器接口。
type ClickHouse interface {
	// Client 返回底层 ClickHouse 连接。
	// 可用于执行任意 ClickHouse 操作。
	// 关闭后仍可调用，但底层连接操作会返回驱动层错误。
	// 方法名与 xmongo.Mongo.Client()、xcache.Redis.Client() 保持一致。
	Client() driver.Conn

	// Health 执行健康检查。
	// 通过 Ping 检测连接状态。
	// 关闭后调用返回 ErrClosed。
	Health(ctx context.Context) error

	// Stats 返回统计信息。
	// 包含健康检查次数、查询次数、慢查询次数、连接池状态等。
	Stats() Stats

	// Close 关闭 ClickHouse 连接。
	Close() error

	// QueryPage 分页查询。
	// 设计决策: 方法名 QueryPage（而非 FindPage）遵循 SQL 领域惯用语。
	// xmongo 使用 FindPage 是 MongoDB 惯用语（find）。各存储包遵循自身领域命名，
	// 而非强制统一，以降低领域切换的认知负担。BatchInsert 同理（ClickHouse 使用 Batch 概念）。
	//
	// query 是 SQL 查询语句（不含 LIMIT/OFFSET），opts 指定分页参数。
	//
	// 注意事项：
	//   - 查询需要支持 COUNT(*) 以获取总数。
	//   - 此方法执行两次查询（COUNT + 数据查询），
	//     在高并发写入场景下，Total 与实际返回数据可能不完全一致。
	//     如需强一致性，请考虑游标分页或在应用层处理。
	//
	// 性能说明：
	//   - COUNT 查询使用子查询包装方式（SELECT COUNT(*) FROM (原查询) AS _count_subquery）
	//   - 这种方式能正确处理复杂 SQL（子查询、CTE、UNION、DISTINCT 等）
	//   - 对于简单查询可能比直接改写 SELECT 列表性能略差
	//   - 性能敏感场景建议直接使用 Client() 执行优化的 COUNT 语句
	//   - Stats().QueryCount 会 +2（COUNT 和分页各计一次）
	//   - 关闭后调用返回 ErrClosed
	QueryPage(ctx context.Context, query string, opts PageOptions, args ...any) (*PageResult, error)

	// BatchInsert 批量插入。
	// table 是目标表名，rows 是待插入的数据切片。
	// 关闭后调用返回 ErrClosed。
	BatchInsert(ctx context.Context, table string, rows []any, opts BatchOptions) (*BatchResult, error)
}

// =============================================================================
// 分页相关类型
// =============================================================================

// PageOptions 分页查询选项。
type PageOptions struct {
	// Page 是页码，从 1 开始。
	Page int64

	// PageSize 是每页大小。
	PageSize int64
}

// PageResult 分页查询结果。
type PageResult struct {
	// Columns 是列名列表。
	Columns []string

	// Rows 是查询结果行。
	Rows [][]any

	// Total 是符合条件的总记录数。
	Total int64

	// Page 是当前页码。
	Page int64

	// PageSize 是每页大小。
	PageSize int64

	// TotalPages 是总页数。
	TotalPages int64
}

// =============================================================================
// 批量操作相关类型
// =============================================================================

// BatchOptions 批量操作选项。
type BatchOptions struct {
	// BatchSize 是每批大小。
	// 如果为 0 或负值，使用默认值 DefaultBatchSize（10000）。
	// 不得超过 MaxBatchSize（100000），否则返回 ErrBatchSizeTooLarge。
	BatchSize int
}

// BatchResult 批量操作结果。
//
// 重要：即使返回的 error 不为 nil，result 仍可能包含有效数据！
// 调用方应同时检查 error 和 result.InsertedCount 以获取完整信息。
//
// 原子性说明：
// ClickHouse 的每个批次（Batch）是原子的：要么全部成功，要么全部失败。
// InsertedCount 只反映成功发送的批次中的记录数。
// 如果某批次 Send() 失败，该批次的所有记录都不会被插入。
// 这与 MongoDB 的部分成功行为不同。
//
// 示例：
//
//	result, err := wrapper.BatchInsert(ctx, table, rows, opts)
//	if err != nil {
//	    log.Printf("部分失败: %d/%d 条成功插入, 错误: %v",
//	        result.InsertedCount, len(rows), err)
//	}
type BatchResult struct {
	// InsertedCount 是成功插入的记录数。
	// 仅统计成功发送（Send）的批次中的记录。
	// 如果 AppendStruct 失败，该记录不计入；如果 Send 失败，整批次不计入。
	// 即使 err != nil，InsertedCount 也可能 > 0，表示部分成功。
	InsertedCount int64

	// Errors 是发生的错误列表。
	// 可能包含 AppendStruct 错误（单条记录）和 Send 错误（整批次）。
	Errors []error
}

// =============================================================================
// 工厂函数
// =============================================================================

// New 创建 ClickHouse 包装器。
// client 是已创建的 ClickHouse 连接，opts 是可选配置。
// 参数名 client 而非 conn，与 Client() 方法及 ErrNilClient 命名保持一致。
//
// 示例：
//
//	conn, err := clickhouse.Open(&clickhouse.Options{
//	    Addr: []string{"localhost:9000"},
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	ch, err := xclickhouse.New(conn)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer ch.Close()
func New(client driver.Conn, opts ...Option) (ClickHouse, error) {
	if client == nil {
		return nil, ErrNilClient
	}

	options := defaultOptions()
	for _, opt := range opts {
		opt(options)
	}

	// 创建慢查询检测器
	detector, err := newSlowQueryDetector(options)
	if err != nil {
		return nil, err
	}

	return &clickhouseWrapper{
		conn:              client,
		options:           options,
		slowQueryDetector: detector,
	}, nil
}

// newSlowQueryDetector 创建慢查询检测器。
func newSlowQueryDetector(opts *options) (*storageopt.SlowQueryDetector[SlowQueryInfo], error) {
	// 构建 storageopt 的慢查询选项
	sqOpts := storageopt.SlowQueryOptions[SlowQueryInfo]{
		Threshold:           opts.SlowQueryThreshold,
		AsyncWorkerPoolSize: opts.AsyncSlowQueryWorkers,
		AsyncQueueSize:      opts.AsyncSlowQueryQueueSize,
	}

	// 设计决策: 使用闭包适配而非直接赋值，因为 SlowQueryHook 和
	// storageopt.SlowQueryHook[SlowQueryInfo] 是不同的命名类型（Go 不允许直接赋值）。
	if opts.SlowQueryHook != nil {
		sqOpts.SyncHook = func(ctx context.Context, info SlowQueryInfo) {
			opts.SlowQueryHook(ctx, info)
		}
	}
	if opts.AsyncSlowQueryHook != nil {
		sqOpts.AsyncHook = func(info SlowQueryInfo) {
			opts.AsyncSlowQueryHook(info)
		}
	}

	return storageopt.NewSlowQueryDetector(sqOpts)
}
