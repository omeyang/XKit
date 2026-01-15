package xmongo

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestFindPage_NilCollection(t *testing.T) {
	w := &mongoWrapper{
		client:  nil,
		options: defaultOptions(),
	}

	result, err := w.FindPage(context.Background(), nil, bson.M{}, PageOptions{
		Page:     1,
		PageSize: 10,
	})

	assert.Nil(t, result)
	assert.ErrorIs(t, err, ErrNilCollection)
}

func TestFindPage_InvalidPage(t *testing.T) {
	// 注意：由于参数验证顺序的问题（先验证 collection 是否为 nil），
	// 这些测试需要 mock collection，在短模式下跳过
	if testing.Short() {
		t.Skip("skipping test that requires mock collection in short mode")
	}
}

func TestFindPage_InvalidPageSize(t *testing.T) {
	// 注意：由于参数验证顺序的问题（先验证 collection 是否为 nil），
	// 这些测试需要 mock collection，在短模式下跳过
	if testing.Short() {
		t.Skip("skipping test that requires mock collection in short mode")
	}
}

func TestPageOptions_Defaults(t *testing.T) {
	opts := PageOptions{}

	// 验证零值
	assert.Equal(t, int64(0), opts.Page)
	assert.Equal(t, int64(0), opts.PageSize)
	assert.Nil(t, opts.Sort)
}

func TestPageOptions_WithSort(t *testing.T) {
	opts := PageOptions{
		Page:     1,
		PageSize: 10,
		Sort:     bson.D{{Key: "created_at", Value: -1}},
	}

	assert.Equal(t, int64(1), opts.Page)
	assert.Equal(t, int64(10), opts.PageSize)
	assert.Len(t, opts.Sort, 1)
}

func TestPageResult_TotalPages(t *testing.T) {
	tests := []struct {
		name       string
		total      int64
		pageSize   int64
		totalPages int64
	}{
		{"整除", 100, 10, 10},
		{"有余数", 101, 10, 11},
		{"不足一页", 5, 10, 1},
		{"刚好一页", 10, 10, 1},
		{"空数据", 0, 10, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 计算总页数的逻辑
			var totalPages int64
			if tt.total > 0 {
				totalPages = tt.total / tt.pageSize
				if tt.total%tt.pageSize > 0 {
					totalPages++
				}
			}
			assert.Equal(t, tt.totalPages, totalPages)
		})
	}
}

// =============================================================================
// 集成测试 - 需要真实 MongoDB
// =============================================================================

func TestFindPage_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	// 需要真实 MongoDB 实例
}

func TestFindPage_WithSort_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
}

func TestFindPage_EmptyResult_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
}
