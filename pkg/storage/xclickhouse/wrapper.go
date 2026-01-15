package xclickhouse

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"sync/atomic"
	"time"

	"github.com/omeyang/xkit/pkg/observability/xmetrics"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

// =============================================================================
// clickhouseWrapper 实现
// =============================================================================

// clickhouseWrapper 实现 ClickHouse 接口。
type clickhouseWrapper struct {
	conn    driver.Conn
	options *Options

	// 统计计数器
	pingCount   atomic.Int64
	pingErrors  atomic.Int64
	queryCount  atomic.Int64
	queryErrors atomic.Int64
	slowQueries atomic.Int64
}

const clickhouseComponent = "xclickhouse"

// Conn 返回底层 ClickHouse 连接。
func (w *clickhouseWrapper) Conn() driver.Conn {
	return w.conn
}

// Health 执行健康检查。
func (w *clickhouseWrapper) Health(ctx context.Context) (err error) {
	ctx, span := xmetrics.Start(ctx, w.options.Observer, xmetrics.SpanOptions{
		Component: clickhouseComponent,
		Operation: "health",
		Kind:      xmetrics.KindClient,
		Attrs: []xmetrics.Attr{
			xmetrics.String("db.system", "clickhouse"),
		},
	})
	defer func() {
		span.End(xmetrics.Result{Err: err})
	}()

	w.pingCount.Add(1)

	// 如果设置了超时，使用带超时的 context
	if w.options.HealthTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, w.options.HealthTimeout)
		defer cancel()
	}

	if err := w.conn.Ping(ctx); err != nil {
		w.pingErrors.Add(1)
		return err
	}

	return nil
}

// Stats 返回统计信息。
func (w *clickhouseWrapper) Stats() Stats {
	return Stats{
		PingCount:   w.pingCount.Load(),
		PingErrors:  w.pingErrors.Load(),
		QueryCount:  w.queryCount.Load(),
		QueryErrors: w.queryErrors.Load(),
		SlowQueries: w.slowQueries.Load(),
		Pool:        PoolStats{},
	}
}

// Close 关闭 ClickHouse 连接。
func (w *clickhouseWrapper) Close() error {
	if w.conn == nil {
		return nil
	}
	return w.conn.Close()
}

// =============================================================================
// 分页查询实现
// =============================================================================

// QueryPage 分页查询。
func (w *clickhouseWrapper) QueryPage(ctx context.Context, query string, opts PageOptions, args ...any) (result *PageResult, err error) {
	if err := validatePageOptions(query, opts); err != nil {
		return nil, err
	}

	start := time.Now()
	ctx, span := xmetrics.Start(ctx, w.options.Observer, xmetrics.SpanOptions{
		Component: clickhouseComponent,
		Operation: "query_page",
		Kind:      xmetrics.KindClient,
		Attrs: []xmetrics.Attr{
			xmetrics.String("db.system", "clickhouse"),
		},
	})
	defer func() {
		duration := measureOperation(start)
		slow := w.maybeSlowQuery(ctx, SlowQueryInfo{
			Query:    query,
			Args:     args,
			Duration: duration,
		})

		var attrs []xmetrics.Attr
		if slow {
			attrs = append(attrs,
				xmetrics.Bool("slow", true),
				xmetrics.Int64("slow_threshold_ms", w.options.SlowQueryThreshold.Milliseconds()),
			)
		}
		span.End(xmetrics.Result{Err: err, Attrs: attrs})
	}()

	// queryCount 在各子方法中分别增加，准确反映实际查询次数

	// 执行 COUNT 查询获取总数
	total, err := w.executeCountQuery(ctx, query, args...)
	if err != nil {
		return nil, err
	}

	// 执行分页查询
	columns, data, err := w.executePageQuery(ctx, query, opts, args...)
	if err != nil {
		return nil, err
	}

	return &PageResult{
		Columns:    columns,
		Rows:       data,
		Total:      total,
		Page:       opts.Page,
		PageSize:   opts.PageSize,
		TotalPages: calculateTotalPages(total, opts.PageSize),
	}, nil
}

// validatePageOptions 验证分页参数。
func validatePageOptions(query string, opts PageOptions) error {
	if query == "" {
		return ErrEmptyQuery
	}
	if opts.Page < 1 {
		return ErrInvalidPage
	}
	if opts.PageSize < 1 {
		return ErrInvalidPageSize
	}
	return nil
}

// executeCountQuery 执行计数查询。
func (w *clickhouseWrapper) executeCountQuery(ctx context.Context, query string, args ...any) (int64, error) {
	w.queryCount.Add(1)
	countQuery := buildCountQuery(query)
	var total int64
	if err := w.conn.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		w.queryErrors.Add(1)
		return 0, fmt.Errorf("count query failed: %w", err)
	}
	return total, nil
}

// executePageQuery 执行分页数据查询。
func (w *clickhouseWrapper) executePageQuery(ctx context.Context, query string, opts PageOptions, args ...any) ([]string, [][]any, error) {
	w.queryCount.Add(1)
	offset := (opts.Page - 1) * opts.PageSize
	pageQuery := fmt.Sprintf("%s LIMIT %d OFFSET %d", query, opts.PageSize, offset)

	rows, err := w.conn.Query(ctx, pageQuery, args...)
	if err != nil {
		w.queryErrors.Add(1)
		return nil, nil, fmt.Errorf("page query failed: %w", err)
	}

	columns := rows.Columns()
	data, scanErr := w.scanRows(rows)
	closeErr := rows.Close()
	var wrappedCloseErr error
	if closeErr != nil {
		w.queryErrors.Add(1)
		wrappedCloseErr = fmt.Errorf("close rows failed: %w", closeErr)
	}
	if err := errors.Join(scanErr, wrappedCloseErr); err != nil {
		return nil, nil, err
	}

	return columns, data, nil
}

// scanRows 扫描结果集中的所有行。
func (w *clickhouseWrapper) scanRows(rows driver.Rows) ([][]any, error) {
	columnTypes := rows.ColumnTypes()
	var data [][]any

	for rows.Next() {
		// 为每列创建对应类型的值实例用于接收数据
		scanDest := make([]any, len(columnTypes))
		for i := range scanDest {
			scanType := columnTypes[i].ScanType()
			// reflect.New 创建指向该类型零值的指针，Interface() 返回 any 类型
			scanDest[i] = reflect.New(scanType).Interface()
		}

		if err := rows.Scan(scanDest...); err != nil {
			w.queryErrors.Add(1)
			return nil, fmt.Errorf("scan failed: %w", err)
		}

		// 提取实际值（解引用指针）
		row := make([]any, len(columnTypes))
		for i := range row {
			row[i] = reflect.ValueOf(scanDest[i]).Elem().Interface()
		}
		data = append(data, row)
	}

	if err := rows.Err(); err != nil {
		w.queryErrors.Add(1)
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return data, nil
}

// calculateTotalPages 计算总页数。
func calculateTotalPages(total, pageSize int64) int64 {
	totalPages := total / pageSize
	if total%pageSize > 0 {
		totalPages++
	}
	return totalPages
}

// =============================================================================
// 批量插入实现
// =============================================================================

// tableNamePattern 用于校验表名的合法性。
// 支持格式：table_name、database.table_name、`database`.`table_name`
// 允许字母、数字、下划线、点号和反引号。
var tableNamePattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*(\.[a-zA-Z_][a-zA-Z0-9_]*)?$|^` + "`[^`]+`" + `(\.` + "`[^`]+`" + `)?$`)

// validateTableName 校验表名是否合法，防止 SQL 注入。
func validateTableName(table string) error {
	if table == "" {
		return ErrEmptyTable
	}
	if !tableNamePattern.MatchString(table) {
		return ErrInvalidTableName
	}
	return nil
}

// BatchInsert 批量插入。
func (w *clickhouseWrapper) BatchInsert(ctx context.Context, table string, rows []any, opts BatchOptions) (result *BatchResult, err error) {
	if err := validateTableName(table); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrEmptyRows
	}

	batchSize := opts.BatchSize
	if batchSize < 1 {
		batchSize = 10000 // 默认批次大小
	}

	start := time.Now()
	ctx, span := xmetrics.Start(ctx, w.options.Observer, xmetrics.SpanOptions{
		Component: clickhouseComponent,
		Operation: "batch_insert",
		Kind:      xmetrics.KindClient,
		Attrs: []xmetrics.Attr{
			xmetrics.String("db.system", "clickhouse"),
		},
	})
	defer func() {
		duration := measureOperation(start)
		slow := w.maybeSlowQuery(ctx, SlowQueryInfo{
			Query:    fmt.Sprintf("INSERT INTO %s", table),
			Duration: duration,
		})

		var attrs []xmetrics.Attr
		if slow {
			attrs = append(attrs,
				xmetrics.Bool("slow", true),
				xmetrics.Int64("slow_threshold_ms", w.options.SlowQueryThreshold.Milliseconds()),
			)
		}
		span.End(xmetrics.Result{Err: err, Attrs: attrs})
	}()

	var insertedCount int64
	var errs []error

	insertedCount, errs = w.insertBatches(ctx, table, rows, batchSize)

	// 当存在错误时，同时返回结果和合并的错误，让调用方能通过 err != nil 判断
	var resultErr error
	if len(errs) > 0 {
		resultErr = errors.Join(errs...)
	}

	return &BatchResult{
		InsertedCount: insertedCount,
		Errors:        errs,
	}, resultErr
}

func (w *clickhouseWrapper) insertBatches(ctx context.Context, table string, rows []any, batchSize int) (int64, []error) {
	var insertedCount int64
	var errs []error

	for i := 0; i < len(rows); i += batchSize {
		// 每批次前检查 context，避免在 context 取消后继续无效操作
		if err := ctx.Err(); err != nil {
			errs = append(errs, fmt.Errorf("context canceled before batch %d: %w", i/batchSize, err))
			break
		}

		end := i + batchSize
		if end > len(rows) {
			end = len(rows)
		}

		batch := rows[i:end]
		count, batchErrs := w.insertBatch(ctx, table, batch)
		insertedCount += count
		if len(batchErrs) > 0 {
			errs = append(errs, batchErrs...)
		}
	}

	return insertedCount, errs
}

func (w *clickhouseWrapper) insertBatch(ctx context.Context, table string, batch []any) (appendedCount int64, errs []error) {
	batchObj, err := w.conn.PrepareBatch(ctx, fmt.Sprintf("INSERT INTO %s", table))
	if err != nil {
		return 0, []error{fmt.Errorf("prepare batch failed: %w", err)}
	}

	// 标记是否已成功发送，用于 defer 中决定是否需要 Abort
	var sent bool
	defer func() {
		if !sent {
			// 未发送时调用 Abort 释放资源
			if abortErr := batchObj.Abort(); abortErr != nil {
				errs = append(errs, fmt.Errorf("abort batch failed: %w", abortErr))
			}
		}
	}()
	for _, row := range batch {
		if err := batchObj.AppendStruct(row); err != nil {
			errs = append(errs, fmt.Errorf("append struct failed: %w", err))
			continue
		}
		appendedCount++
	}

	// 如果没有成功追加任何行，跳过 Send（defer 会自动 Abort）
	if appendedCount == 0 {
		return 0, errs
	}

	if err := batchObj.Send(); err != nil {
		errs = append(errs, fmt.Errorf("send batch failed: %w", err))
		return 0, errs
	}

	sent = true
	return appendedCount, errs
}

// =============================================================================
// 慢查询检测
// =============================================================================

// triggerSlowQueryHook 触发慢查询钩子。
func (w *clickhouseWrapper) triggerSlowQueryHook(ctx context.Context, info SlowQueryInfo) {
	if w.options.SlowQueryHook != nil {
		w.slowQueries.Add(1)
		w.options.SlowQueryHook(ctx, info)
	}
}

// maybeSlowQuery 检测并可能触发慢查询钩子。
func (w *clickhouseWrapper) maybeSlowQuery(ctx context.Context, info SlowQueryInfo) bool {
	// 如果阈值为 0，禁用慢查询检测
	if w.options.SlowQueryThreshold == 0 {
		return false
	}

	// 如果耗时超过阈值，触发钩子
	if info.Duration >= w.options.SlowQueryThreshold {
		w.triggerSlowQueryHook(ctx, info)
		return true
	}
	return false
}

// =============================================================================
// 辅助函数
// =============================================================================

// measureOperation 测量操作耗时。
func measureOperation(start time.Time) time.Duration {
	return time.Since(start)
}

// buildCountQuery 根据原始查询构建 COUNT 查询。
// 使用子查询包装方式，避免复杂 SQL 解析问题（子查询、CTE、UNION 等）。
func buildCountQuery(query string) string {
	return fmt.Sprintf("SELECT COUNT(*) FROM (%s) AS _count_subquery", query)
}
