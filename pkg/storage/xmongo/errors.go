package xmongo

import "errors"

// =============================================================================
// 通用错误
// =============================================================================

var (
	// ErrNilClient 表示传入的客户端为 nil。
	ErrNilClient = errors.New("xmongo: nil client")

	// ErrClosed 表示客户端已关闭。
	ErrClosed = errors.New("xmongo: client closed")
)

// =============================================================================
// 分页查询错误
// =============================================================================

var (
	// ErrInvalidPage 表示页码无效（必须 >= 1）。
	ErrInvalidPage = errors.New("xmongo: invalid page number, must be >= 1")

	// ErrInvalidPageSize 表示每页大小无效（必须 >= 1）。
	ErrInvalidPageSize = errors.New("xmongo: invalid page size, must be >= 1")

	// ErrNilCollection 表示传入的 collection 为 nil。
	ErrNilCollection = errors.New("xmongo: nil collection")
)

// =============================================================================
// 批量写入错误
// =============================================================================

var (
	// ErrEmptyDocs 表示文档列表为空。
	ErrEmptyDocs = errors.New("xmongo: empty documents")

	// ErrInvalidBatchSize 表示批次大小无效（必须 >= 1）。
	ErrInvalidBatchSize = errors.New("xmongo: invalid batch size, must be >= 1")
)
