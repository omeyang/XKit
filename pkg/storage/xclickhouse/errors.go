package xclickhouse

import (
	"errors"
	"fmt"

	"github.com/omeyang/xkit/internal/storageopt"
)

// 包级别错误定义。
var (
	// ErrNilConn 表示传入了 nil 连接。
	ErrNilConn = errors.New("xclickhouse: nil connection")

	// ErrClosed 表示连接已关闭。
	ErrClosed = errors.New("xclickhouse: connection closed")

	// ErrInvalidPage 表示页码无效。
	// 此错误包装了 storageopt.ErrInvalidPage，可以使用 errors.Is 检查任一错误。
	ErrInvalidPage = fmt.Errorf("xclickhouse: %w", storageopt.ErrInvalidPage)

	// ErrInvalidPageSize 表示页大小无效。
	// 此错误包装了 storageopt.ErrInvalidPageSize，可以使用 errors.Is 检查任一错误。
	ErrInvalidPageSize = fmt.Errorf("xclickhouse: %w", storageopt.ErrInvalidPageSize)

	// ErrPageOverflow 表示分页计算溢出（页码或每页大小过大）。
	// 此错误包装了 storageopt.ErrPageOverflow，可以使用 errors.Is 检查任一错误。
	ErrPageOverflow = fmt.Errorf("xclickhouse: %w", storageopt.ErrPageOverflow)

	// ErrEmptyQuery 表示查询语句为空。
	ErrEmptyQuery = errors.New("xclickhouse: empty query")

	// ErrEmptyTable 表示表名为空。
	ErrEmptyTable = errors.New("xclickhouse: empty table name")

	// ErrInvalidTableName 表示表名包含非法字符。
	ErrInvalidTableName = errors.New("xclickhouse: invalid table name, contains illegal characters")

	// ErrEmptyRows 表示待插入数据为空。
	ErrEmptyRows = errors.New("xclickhouse: empty rows")

	// ErrQueryContainsFormat 表示查询包含 FORMAT 子句。
	// QueryPage 使用子查询包装，不支持 FORMAT 子句。
	//
	// 注意：检测使用正则匹配，可能对字符串字面量中的 FORMAT 产生误判。
	// 如遇误判，请使用 Conn() 直接执行查询。
	ErrQueryContainsFormat = errors.New("xclickhouse: query contains FORMAT clause, not supported in QueryPage")

	// ErrQueryContainsSettings 表示查询包含 SETTINGS 子句。
	// QueryPage 使用子查询包装，SETTINGS 应通过连接参数配置。
	//
	// 注意：检测使用正则匹配，可能对字符串字面量中的 SETTINGS 产生误判。
	// 如遇误判，请使用 Conn() 直接执行查询。
	ErrQueryContainsSettings = errors.New("xclickhouse: query contains SETTINGS clause, use connection options instead")
)
