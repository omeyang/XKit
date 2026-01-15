package xmongo

import (
	"context"

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
	// 注意：此方法执行两次查询（COUNT + 数据查询），
	// 在高并发写入场景下，Total 与实际返回数据可能不完全一致。
	// 如需强一致性，请使用事务或考虑游标分页方案。
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
	Sort bson.D
}

// PageResult 分页查询结果。
type PageResult struct {
	// Data 当前页数据。
	Data []bson.M

	// Total 总记录数。
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
type BulkResult struct {
	// InsertedCount 成功插入数量。
	InsertedCount int64

	// Errors 写入过程中的错误列表。
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

	return &mongoWrapper{
		client:    client,
		clientOps: client, // *mongo.Client 实现 clientOperations 接口
		options:   options,
	}, nil
}
