package xmongo

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBulkInsert_NilCollection(t *testing.T) {
	w := &mongoWrapper{
		client:  nil,
		options: defaultOptions(),
	}

	docs := []any{
		map[string]any{"name": "test1"},
		map[string]any{"name": "test2"},
	}

	result, err := w.BulkInsert(context.Background(), nil, docs, BulkOptions{})

	assert.Nil(t, result)
	assert.ErrorIs(t, err, ErrNilCollection)
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

