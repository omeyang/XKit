package xclickhouse

import (
	"context"
	"errors"
	"reflect"

	"github.com/ClickHouse/clickhouse-go/v2/lib/column"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/ClickHouse/clickhouse-go/v2/lib/proto"
)

// 类型别名，方便测试代码使用
type (
	Row   = driver.Row
	Rows  = driver.Rows
	Batch = driver.Batch
)

// =============================================================================
// Mock 实现 - 用于单元测试
// =============================================================================

// mockConn 实现 driver.Conn 接口
type mockConn struct {
	pingErr         error
	pingCount       int
	closeErr        error
	closed          bool
	queryRowFunc    func(ctx context.Context, query string, args ...any) driver.Row
	queryFunc       func(ctx context.Context, query string, args ...any) (driver.Rows, error)
	prepareBatchErr error
	batchFunc       func(ctx context.Context, query string) driver.Batch
	stats           driver.Stats
}

func (m *mockConn) Contributors() []string {
	return []string{"test"}
}

func (m *mockConn) ServerVersion() (*proto.ServerHandshake, error) {
	return &proto.ServerHandshake{}, nil
}

func (m *mockConn) Select(_ context.Context, _ any, _ string, _ ...any) error {
	return nil
}

func (m *mockConn) Query(ctx context.Context, query string, args ...any) (driver.Rows, error) {
	if m.queryFunc != nil {
		return m.queryFunc(ctx, query, args...)
	}
	return nil, errors.New("query not implemented")
}

func (m *mockConn) QueryRow(ctx context.Context, query string, args ...any) driver.Row {
	if m.queryRowFunc != nil {
		return m.queryRowFunc(ctx, query, args...)
	}
	return &mockRow{err: errors.New("queryRow not implemented")}
}

func (m *mockConn) PrepareBatch(ctx context.Context, query string, _ ...driver.PrepareBatchOption) (driver.Batch, error) {
	if m.prepareBatchErr != nil {
		return nil, m.prepareBatchErr
	}
	if m.batchFunc != nil {
		return m.batchFunc(ctx, query), nil
	}
	return &mockBatch{}, nil
}

func (m *mockConn) Exec(_ context.Context, _ string, _ ...any) error {
	return nil
}

func (m *mockConn) AsyncInsert(_ context.Context, _ string, _ bool, _ ...any) error {
	return nil
}

func (m *mockConn) Ping(_ context.Context) error {
	m.pingCount++
	return m.pingErr
}

func (m *mockConn) Stats() driver.Stats {
	return m.stats
}

func (m *mockConn) Close() error {
	m.closed = true
	return m.closeErr
}

// mockRow 实现 driver.Row 接口
type mockRow struct {
	err      error
	scanFunc func(dest ...any) error
}

func (m *mockRow) Err() error {
	return m.err
}

func (m *mockRow) Scan(dest ...any) error {
	if m.err != nil {
		return m.err
	}
	if m.scanFunc != nil {
		return m.scanFunc(dest...)
	}
	return nil
}

func (m *mockRow) ScanStruct(_ any) error {
	return m.err
}

// mockRows 实现 driver.Rows 接口
type mockRows struct {
	data        [][]any
	columns     []string
	columnTypes []driver.ColumnType
	index       int
	scanErr     error
	closeErr    error
	err         error
}

func (m *mockRows) Next() bool {
	if m.index < len(m.data) {
		m.index++
		return true
	}
	return false
}

func (m *mockRows) Scan(dest ...any) error {
	if m.scanErr != nil {
		return m.scanErr
	}
	if m.index > 0 && m.index <= len(m.data) {
		row := m.data[m.index-1]
		for i := 0; i < len(dest) && i < len(row); i++ {
			// 简单赋值，实际使用需要类型转换
			if ptr, ok := dest[i].(*any); ok {
				*ptr = row[i]
			}
		}
	}
	return nil
}

func (m *mockRows) ScanStruct(_ any) error {
	return m.scanErr
}

func (m *mockRows) ColumnTypes() []driver.ColumnType {
	return m.columnTypes
}

func (m *mockRows) Totals(_ ...any) error {
	return nil
}

func (m *mockRows) Columns() []string {
	return m.columns
}

func (m *mockRows) Close() error {
	return m.closeErr
}

func (m *mockRows) Err() error {
	return m.err
}

// mockColumnType 实现 driver.ColumnType 接口
type mockColumnType struct {
	name     string
	nullable bool
	scanType reflect.Type
	dbType   string
}

func (m *mockColumnType) Name() string {
	return m.name
}

func (m *mockColumnType) Nullable() bool {
	return m.nullable
}

func (m *mockColumnType) ScanType() reflect.Type {
	if m.scanType != nil {
		return m.scanType
	}
	return reflect.TypeFor[any]()
}

func (m *mockColumnType) DatabaseTypeName() string {
	return m.dbType
}

// mockBatchColumn 实现 driver.BatchColumn 接口
type mockBatchColumn struct{}

func (m *mockBatchColumn) Append(_ any) error {
	return nil
}

func (m *mockBatchColumn) AppendRow(_ any) error {
	return nil
}

// mockBatch 实现 driver.Batch 接口
type mockBatch struct {
	appendErr error
	sendErr   error
	abortErr  error
	sent      bool
	rows      int
}

func (m *mockBatch) Abort() error {
	return m.abortErr
}

func (m *mockBatch) Append(_ ...any) error {
	if m.appendErr != nil {
		return m.appendErr
	}
	m.rows++
	return nil
}

func (m *mockBatch) AppendStruct(_ any) error {
	if m.appendErr != nil {
		return m.appendErr
	}
	m.rows++
	return nil
}

func (m *mockBatch) Column(_ int) driver.BatchColumn {
	return &mockBatchColumn{}
}

func (m *mockBatch) Flush() error {
	return nil
}

func (m *mockBatch) Send() error {
	if m.sendErr != nil {
		return m.sendErr
	}
	m.sent = true
	return nil
}

func (m *mockBatch) IsSent() bool {
	return m.sent
}

func (m *mockBatch) Rows() int {
	return m.rows
}

func (m *mockBatch) Columns() []column.Interface {
	return nil
}

func (m *mockBatch) Close() error {
	return nil
}

// cancelOnAppendBatch 在指定行数后取消 context 的 mock batch。
// 用于测试 append 成功后、send 前 context 被取消的场景。
type cancelOnAppendBatch struct {
	cancel          context.CancelFunc
	cancelAfterRows int
	appendCount     *int
}

func (b *cancelOnAppendBatch) Abort() error                    { return nil }
func (b *cancelOnAppendBatch) Send() error                     { return nil }
func (b *cancelOnAppendBatch) Flush() error                    { return nil }
func (b *cancelOnAppendBatch) IsSent() bool                    { return false }
func (b *cancelOnAppendBatch) Rows() int                       { return *b.appendCount }
func (b *cancelOnAppendBatch) Columns() []column.Interface     { return nil }
func (b *cancelOnAppendBatch) Column(_ int) driver.BatchColumn { return &mockBatchColumn{} }
func (b *cancelOnAppendBatch) Append(_ ...any) error           { return nil }
func (b *cancelOnAppendBatch) Close() error                    { return nil }

func (b *cancelOnAppendBatch) AppendStruct(_ any) error {
	*b.appendCount++
	if *b.appendCount >= b.cancelAfterRows {
		b.cancel()
	}
	return nil
}

func (b *cancelOnAppendBatch) ScanStruct(_ any) error { return nil }

// =============================================================================
// 辅助构造函数
// =============================================================================

// newMockConn 创建一个新的 mock 连接
func newMockConn() *mockConn {
	return &mockConn{}
}

// newMockRows 创建 mock 行集
func newMockRows(columns []string, data [][]any) *mockRows {
	columnTypes := make([]driver.ColumnType, len(columns))
	for i, name := range columns {
		columnTypes[i] = &mockColumnType{name: name, dbType: "String"}
	}
	return &mockRows{
		columns:     columns,
		columnTypes: columnTypes,
		data:        data,
	}
}
