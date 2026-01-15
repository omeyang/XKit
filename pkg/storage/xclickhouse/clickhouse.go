package xclickhouse

import (
	"context"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

// =============================================================================
// 接口定义
// =============================================================================

// ClickHouse 定义 ClickHouse 包装器接口。
type ClickHouse interface {
	// Conn 返回底层 ClickHouse 连接。
	// 可用于执行任意 ClickHouse 操作。
	Conn() driver.Conn

	// Health 执行健康检查。
	// 通过 Ping 检测连接状态。
	Health(ctx context.Context) error

	// Stats 返回统计信息。
	// 包含健康检查次数、查询次数、慢查询次数、连接池状态等。
	Stats() Stats

	// Close 关闭 ClickHouse 连接。
	Close() error

	// QueryPage 分页查询。
	// query 是 SQL 查询语句（不含 LIMIT/OFFSET），opts 指定分页参数。
	//
	// 注意事项：
	//   - 查询需要支持 COUNT(*) 以获取总数。
	//   - 此方法执行两次查询（COUNT + 数据查询），
	//     在高并发写入场景下，Total 与实际返回数据可能不完全一致。
	//     如需强一致性，请考虑游标分页或在应用层处理。
	QueryPage(ctx context.Context, query string, opts PageOptions, args ...any) (*PageResult, error)

	// BatchInsert 批量插入。
	// table 是目标表名，rows 是待插入的数据切片。
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
	// 如果为 0，使用默认值 10000。
	BatchSize int
}

// BatchResult 批量操作结果。
type BatchResult struct {
	// InsertedCount 是成功插入的记录数。
	InsertedCount int64

	// Errors 是发生的错误列表。
	Errors []error
}

// =============================================================================
// 工厂函数
// =============================================================================

// New 创建 ClickHouse 包装器。
// conn 是已创建的 ClickHouse 连接，opts 是可选配置。
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
func New(conn driver.Conn, opts ...Option) (ClickHouse, error) {
	if conn == nil {
		return nil, ErrNilConn
	}

	options := defaultOptions()
	for _, opt := range opts {
		opt(options)
	}

	return &clickhouseWrapper{
		conn:    conn,
		options: options,
	}, nil
}
