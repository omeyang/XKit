package xmongo

import (
	"errors"
	"fmt"

	"github.com/omeyang/xkit/internal/storageopt"
)

// =============================================================================
// 通用错误
// =============================================================================

var (
	// ErrNilClient 表示传入的客户端为 nil。
	ErrNilClient = errors.New("xmongo: nil client")

	// ErrNilContext 表示传入的 context 为 nil。
	// 所有接受 context 的公开方法（Health、FindPage、BulkInsert）在入口处检查此条件。
	// Close 是例外：nil context 会被替换为 context.Background()，因为关闭操作不应因 nil ctx 而失败。
	ErrNilContext = errors.New("xmongo: context must not be nil")

	// ErrClosed 表示客户端已关闭。
	ErrClosed = errors.New("xmongo: client closed")

	// ErrEmptyURI 表示传入的 MongoDB 连接 URI 为空。
	ErrEmptyURI = errors.New("xmongo: empty URI")
)

// =============================================================================
// 分页查询错误
// =============================================================================

var (
	// ErrInvalidPage 表示页码无效（必须 >= 1）。
	// 此错误包装了 storageopt.ErrInvalidPage，可以使用 errors.Is 检查任一错误。
	ErrInvalidPage = fmt.Errorf("xmongo: %w", storageopt.ErrInvalidPage)

	// ErrInvalidPageSize 表示每页大小无效（必须 >= 1）。
	// 此错误包装了 storageopt.ErrInvalidPageSize，可以使用 errors.Is 检查任一错误。
	ErrInvalidPageSize = fmt.Errorf("xmongo: %w", storageopt.ErrInvalidPageSize)

	// ErrPageOverflow 表示分页计算溢出（页码或每页大小过大）。
	// 此错误包装了 storageopt.ErrPageOverflow，可以使用 errors.Is 检查任一错误。
	ErrPageOverflow = fmt.Errorf("xmongo: %w", storageopt.ErrPageOverflow)

	// ErrNilCollection 表示传入的 collection 为 nil。
	ErrNilCollection = errors.New("xmongo: nil collection")

	// ErrPageSizeTooLarge 表示每页大小超过 MaxPageSize 上限。
	// 防止超大分页请求导致 cursor.All 一次性载入大量数据引发 OOM。
	ErrPageSizeTooLarge = errors.New("xmongo: page size exceeds maximum limit")

	// ErrSkipTooLarge 表示计算后的 skip 值超过 MaxSkip 上限。
	// MongoDB 对大 skip 查询会扫描/丢弃大量文档，效率极低。
	// 建议使用游标分页（基于 _id 或时间戳 seek）代替深度分页。
	ErrSkipTooLarge = errors.New("xmongo: skip exceeds maximum limit, use cursor pagination instead")
)

// =============================================================================
// 批量写入错误
// =============================================================================

var (
	// ErrEmptyDocs 表示文档列表为空。
	ErrEmptyDocs = errors.New("xmongo: empty documents")
)

// BulkBatchError 包装单个批次的写入错误，附带该批次在原始文档切片中的起始偏移。
//
// 背景: BulkInsert 将 docs 分批调用 InsertMany；mongo-driver 的 BulkWriteException.WriteErrors[].Index
// 是相对于当前 batch 的局部索引。调用方需要把局部索引加上 BatchOffset 才能定位到原始 docs 中的全局位置，
// 否则重试失败项时会映射到错误的文档。
type BulkBatchError struct {
	// BatchIndex 批次序号（从 0 开始）。
	BatchIndex int
	// BatchOffset 本批在原始 docs 中的起始下标。
	// 若 BulkWriteException.WriteErrors[i].Index = k，则对应原始 docs 下标为 BatchOffset + k。
	BatchOffset int
	// BatchSize 本批次的文档数量。
	BatchSize int
	// Err 底层错误（通常是 mongo.BulkWriteException，也可能是 context.Canceled 等）。
	Err error
}

// Error 实现 error 接口。
func (e *BulkBatchError) Error() string {
	return fmt.Sprintf("xmongo bulk_insert batch %d [offset=%d size=%d]: %v",
		e.BatchIndex, e.BatchOffset, e.BatchSize, e.Err)
}

// Unwrap 返回底层错误，支持 errors.Is/errors.As 向下匹配 BulkWriteException 等。
func (e *BulkBatchError) Unwrap() error { return e.Err }
