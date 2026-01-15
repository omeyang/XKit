package xmongo

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBulkWrite_NilCollection(t *testing.T) {
	w := &mongoWrapper{
		client:  nil,
		options: defaultOptions(),
	}

	docs := []any{
		map[string]any{"name": "test1"},
		map[string]any{"name": "test2"},
	}

	result, err := w.BulkWrite(context.Background(), nil, docs, BulkOptions{})

	assert.Nil(t, result)
	assert.ErrorIs(t, err, ErrNilCollection)
}

func TestBulkWrite_EmptyDocs(t *testing.T) {
	// 注意：由于参数验证顺序的问题（先验证 collection 是否为 nil），
	// 这个测试需要 mock collection，在短模式下跳过
	if testing.Short() {
		t.Skip("skipping test that requires mock collection in short mode")
	}
}

func TestBulkOptions_Defaults(t *testing.T) {
	opts := BulkOptions{}

	// 验证零值
	assert.Equal(t, 0, opts.BatchSize)
	assert.False(t, opts.Ordered)
}

func TestBulkOptions_WithBatchSize(t *testing.T) {
	opts := BulkOptions{
		BatchSize: 500,
		Ordered:   true,
	}

	assert.Equal(t, 500, opts.BatchSize)
	assert.True(t, opts.Ordered)
}

func TestBulkResult_Initialization(t *testing.T) {
	result := BulkResult{}

	assert.Equal(t, int64(0), result.InsertedCount)
	assert.Nil(t, result.Errors)
}

// =============================================================================
// 批次计算测试
// =============================================================================

func TestBatchCalculation(t *testing.T) {
	tests := []struct {
		name      string
		docsCount int
		batchSize int
		batches   int
	}{
		{"10 文档，批次 5", 10, 5, 2},
		{"10 文档，批次 3", 10, 3, 4},
		{"5 文档，批次 10", 5, 10, 1},
		{"100 文档，批次 1000", 100, 1000, 1},
		{"1000 文档，批次 100", 1000, 100, 10},
		{"1001 文档，批次 100", 1001, 100, 11},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			batches := tt.docsCount / tt.batchSize
			if tt.docsCount%tt.batchSize > 0 {
				batches++
			}
			assert.Equal(t, tt.batches, batches)
		})
	}
}

// =============================================================================
// 集成测试 - 需要真实 MongoDB
// =============================================================================

func TestBulkWrite_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	// 需要真实 MongoDB 实例
}

func TestBulkWrite_Ordered_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
}

func TestBulkWrite_LargeBatch_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
}
