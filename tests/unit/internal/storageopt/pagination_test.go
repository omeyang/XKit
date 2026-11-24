package storageopt

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidatePagination(t *testing.T) {
	tests := []struct {
		name     string
		page     int64
		pageSize int64
		wantOff  int64
		wantErr  error
	}{
		{
			name:     "valid first page",
			page:     1,
			pageSize: 10,
			wantOff:  0,
			wantErr:  nil,
		},
		{
			name:     "valid second page",
			page:     2,
			pageSize: 10,
			wantOff:  10,
			wantErr:  nil,
		},
		{
			name:     "valid large page",
			page:     100,
			pageSize: 50,
			wantOff:  4950,
			wantErr:  nil,
		},
		{
			name:     "page zero",
			page:     0,
			pageSize: 10,
			wantOff:  0,
			wantErr:  ErrInvalidPage,
		},
		{
			name:     "negative page",
			page:     -1,
			pageSize: 10,
			wantOff:  0,
			wantErr:  ErrInvalidPage,
		},
		{
			name:     "pageSize zero",
			page:     1,
			pageSize: 0,
			wantOff:  0,
			wantErr:  ErrInvalidPageSize,
		},
		{
			name:     "negative pageSize",
			page:     1,
			pageSize: -1,
			wantOff:  0,
			wantErr:  ErrInvalidPageSize,
		},
		{
			name:     "overflow case 1",
			page:     math.MaxInt64,
			pageSize: 2,
			wantOff:  0,
			wantErr:  ErrPageOverflow,
		},
		{
			name:     "overflow case 2",
			page:     1 << 62,
			pageSize: 1 << 62,
			wantOff:  0,
			wantErr:  ErrPageOverflow,
		},
		{
			name:     "near overflow but safe",
			page:     1000000000,
			pageSize: 1000,
			wantOff:  999999999000,
			wantErr:  nil,
		},
		{
			name:     "single item page",
			page:     5,
			pageSize: 1,
			wantOff:  4,
			wantErr:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			offset, err := ValidatePagination(tt.page, tt.pageSize)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantOff, offset)
			}
		})
	}
}

func TestCalculateTotalPages(t *testing.T) {
	tests := []struct {
		name     string
		total    int64
		pageSize int64
		want     int64
	}{
		{"exact division", 100, 10, 10},
		{"with remainder", 101, 10, 11},
		{"single page", 5, 10, 1},
		{"empty result", 0, 10, 0},
		{"zero pageSize", 100, 0, 0},
		{"negative total", -1, 10, 0},
		{"negative pageSize", 100, -1, 0},
		{"large numbers", 1000000000, 100, 10000000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateTotalPages(tt.total, tt.pageSize)
			assert.Equal(t, tt.want, result)
		})
	}
}

func FuzzValidatePagination(f *testing.F) {
	f.Add(int64(1), int64(10))
	f.Add(int64(0), int64(10))
	f.Add(int64(-1), int64(10))
	f.Add(int64(math.MaxInt64), int64(2))
	f.Add(int64(1<<62), int64(1<<62))

	f.Fuzz(func(t *testing.T, page, pageSize int64) {
		offset, err := ValidatePagination(page, pageSize)

		if page < 1 {
			assert.ErrorIs(t, err, ErrInvalidPage)
			return
		}
		if pageSize < 1 {
			assert.ErrorIs(t, err, ErrInvalidPageSize)
			return
		}

		// 如果没有错误，验证结果正确
		if err == nil {
			// 验证 offset 计算正确
			expected := (page - 1) * pageSize
			assert.Equal(t, expected, offset)
			// 验证 offset 非负
			assert.GreaterOrEqual(t, offset, int64(0))
		} else {
			// 唯一可能的错误是 overflow
			assert.ErrorIs(t, err, ErrPageOverflow)
		}
	})
}
