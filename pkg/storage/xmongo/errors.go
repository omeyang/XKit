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
)

// =============================================================================
// 批量写入错误
// =============================================================================

var (
	// ErrEmptyDocs 表示文档列表为空。
	ErrEmptyDocs = errors.New("xmongo: empty documents")
)
