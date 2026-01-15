package xclickhouse

import (
	"context"
	"testing"
	"time"
)

func BenchmarkQueryPage(b *testing.B) {
	conn := newMockConn()
	conn.queryRowFunc = func(_ context.Context, _ string, _ ...any) Row {
		return &mockRow{
			scanFunc: func(dest ...any) error {
				if ptr, ok := dest[0].(*int64); ok {
					*ptr = 100
				}
				return nil
			},
		}
	}
	conn.queryFunc = func(_ context.Context, _ string, _ ...any) (Rows, error) {
		rows := make([][]any, 100)
		for i := range rows {
			rows[i] = []any{int64(i)}
		}
		return newMockRows([]string{"id"}, rows), nil
	}

	w := &clickhouseWrapper{
		conn:    conn,
		options: defaultOptions(),
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := w.QueryPage(context.Background(), "SELECT id FROM bench", PageOptions{
			Page:     1,
			PageSize: 10,
		}); err != nil {
			b.Fatalf("QueryPage failed: %v", err)
		}
	}
}

func BenchmarkBatchInsert(b *testing.B) {
	conn := newMockConn()
	conn.batchFunc = func(_ context.Context, _ string) Batch {
		return &mockBatch{}
	}

	w := &clickhouseWrapper{
		conn:    conn,
		options: defaultOptions(),
	}

	type benchRow struct {
		ID   int
		Name string
		Time time.Time
	}

	rows := make([]any, 1000)
	for i := range rows {
		rows[i] = &benchRow{ID: i, Name: "bench", Time: time.Now()}
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := w.BatchInsert(context.Background(), "bench_table", rows, BatchOptions{
			BatchSize: 200,
		}); err != nil {
			b.Fatalf("BatchInsert failed: %v", err)
		}
	}
}
