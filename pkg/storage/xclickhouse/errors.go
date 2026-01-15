package xclickhouse

import "errors"

// 包级别错误定义。
var (
	// ErrNilConn 表示传入了 nil 连接。
	ErrNilConn = errors.New("xclickhouse: nil connection")

	// ErrClosed 表示连接已关闭。
	ErrClosed = errors.New("xclickhouse: connection closed")

	// ErrInvalidPage 表示页码无效。
	ErrInvalidPage = errors.New("xclickhouse: invalid page number, must be >= 1")

	// ErrInvalidPageSize 表示页大小无效。
	ErrInvalidPageSize = errors.New("xclickhouse: invalid page size, must be >= 1")

	// ErrEmptyQuery 表示查询语句为空。
	ErrEmptyQuery = errors.New("xclickhouse: empty query")

	// ErrEmptyTable 表示表名为空。
	ErrEmptyTable = errors.New("xclickhouse: empty table name")

	// ErrInvalidTableName 表示表名包含非法字符。
	ErrInvalidTableName = errors.New("xclickhouse: invalid table name, contains illegal characters")

	// ErrEmptyRows 表示待插入数据为空。
	ErrEmptyRows = errors.New("xclickhouse: empty rows")

	// ErrInvalidBatchSize 表示批次大小无效。
	ErrInvalidBatchSize = errors.New("xclickhouse: invalid batch size, must be >= 1")
)
