package storageopt

import (
	"errors"
	"math"
)

// 分页相关错误。
var (
	// ErrInvalidPage 表示页码无效（必须 >= 1）。
	ErrInvalidPage = errors.New("storageopt: invalid page number, must be >= 1")

	// ErrInvalidPageSize 表示每页大小无效（必须 >= 1）。
	ErrInvalidPageSize = errors.New("storageopt: invalid page size, must be >= 1")

	// ErrPageOverflow 表示分页计算溢出。
	// 当 (page-1) * pageSize 超过 int64 最大值时返回此错误。
	ErrPageOverflow = errors.New("storageopt: page calculation overflow, reduce page number or page size")
)

// ValidatePagination 验证分页参数并返回计算后的 offset。
//
// 参数：
//   - page: 页码，必须 >= 1
//   - pageSize: 每页大小，必须 >= 1
//
// 返回：
//   - offset: 计算后的偏移量 (page-1) * pageSize
//   - err: 验证错误，可能是 ErrInvalidPage、ErrInvalidPageSize 或 ErrPageOverflow
func ValidatePagination(page, pageSize int64) (offset int64, err error) {
	if page < 1 {
		return 0, ErrInvalidPage
	}
	if pageSize < 1 {
		return 0, ErrInvalidPageSize
	}

	// 检查溢出：(page-1) * pageSize
	// 如果 page-1 > MaxInt64/pageSize，则乘法会溢出
	if page-1 > math.MaxInt64/pageSize {
		return 0, ErrPageOverflow
	}

	offset = (page - 1) * pageSize
	return offset, nil
}

// CalculateTotalPages 计算总页数。
//
// 参数：
//   - total: 总记录数
//   - pageSize: 每页大小
//
// 返回：
//   - 总页数，如果 total 或 pageSize <= 0 则返回 0
func CalculateTotalPages(total, pageSize int64) int64 {
	if total <= 0 || pageSize <= 0 {
		return 0
	}
	totalPages := total / pageSize
	if total%pageSize > 0 {
		totalPages++
	}
	return totalPages
}
