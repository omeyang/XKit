package xclickhouse

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/omeyang/xkit/internal/storageopt"
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

	// closed 标记客户端是否已关闭，防止重复关闭。
	closed atomic.Bool

	// 慢查询检测器
	slowQueryDetector *storageopt.SlowQueryDetector[SlowQueryInfo]

	// 统计计数器（使用 storageopt 通用实现）
	healthCounter    storageopt.HealthCounter
	queryCounter     storageopt.QueryCounter
	slowQueryCounter storageopt.SlowQueryCounter
}

const (
	clickhouseComponent = "xclickhouse"

	// DefaultBatchSize 默认批量插入每批大小。
	DefaultBatchSize = 10000

	// MaxPageSize 分页查询允许的最大页大小。
	// 限制单次查询返回的行数，防止超大 PageSize 导致内存暴涨或 ClickHouse 扫描压力过大。
	// 如需更大的结果集，请使用 Client() 直接执行查询或分多次请求。
	MaxPageSize int64 = 10000

	// MaxOffset 分页查询允许的最大偏移量。
	// ClickHouse 使用 OFFSET 分页时，大偏移量会导致扫描放大（先读取再丢弃行）。
	// 超过此限制时返回 ErrOffsetTooLarge，引导调用方使用游标分页。
	// 默认 100000：PageSize=100 时最多翻到第 1001 页，PageSize=10 时最多第 10001 页。
	MaxOffset int64 = 100000
)

// Client 返回底层 ClickHouse 连接。
//
// 设计决策: Client() 不检查 closed 状态。clickhouse-go driver.Conn 在关闭后
// 操作会返回明确错误，无需在此层重复检查。修改返回签名为 (driver.Conn, error)
// 会破坏 API 兼容性，且 Health/QueryPage/BatchInsert 已有 closed 检查。
func (w *clickhouseWrapper) Client() driver.Conn {
	return w.conn
}

// Health 执行健康检查。
func (w *clickhouseWrapper) Health(ctx context.Context) (err error) {
	if w.closed.Load() {
		return ErrClosed
	}

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

	w.healthCounter.IncPing()

	// 使用 storageopt 的健康检查超时
	ctx, cancel := storageopt.HealthContext(ctx, w.options.HealthTimeout)
	defer cancel()

	if err := w.conn.Ping(ctx); err != nil {
		w.healthCounter.IncPingError()
		return err
	}

	return nil
}

// Stats 返回统计信息。
func (w *clickhouseWrapper) Stats() Stats {
	s := Stats{
		PingCount:   w.healthCounter.PingCount(),
		PingErrors:  w.healthCounter.PingErrors(),
		QueryCount:  w.queryCounter.QueryCount(),
		QueryErrors: w.queryCounter.QueryErrors(),
		SlowQueries: w.slowQueryCounter.Count(),
	}
	if w.conn != nil {
		ds := w.conn.Stats()
		s.Pool = PoolStats{
			Open:  ds.Open,
			Idle:  ds.Idle,
			InUse: ds.Open - ds.Idle,
		}
	}
	return s
}

// Close 关闭 ClickHouse 连接。
// 多次调用 Close 是安全的，第二次及后续调用返回 ErrClosed。
//
// 已知限制: 若 goroutine 在 closed 检查后、defer maybeSlowQuery 前被调度，
// 而 Close 先完成了 slowQueryDetector.Close()，则 maybeSlowQuery 可能在已关闭的
// detector 上执行。SlowQueryDetector.Close 后调用 MaybeSlowQuery 是安全的（无 panic），
// 但慢查询可能不被记录。此窗口极小，实际影响可忽略。
func (w *clickhouseWrapper) Close() error {
	if !w.closed.CompareAndSwap(false, true) {
		return ErrClosed
	}

	// 关闭慢查询检测器
	if w.slowQueryDetector != nil {
		w.slowQueryDetector.Close()
	}

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
	if w.closed.Load() {
		return nil, ErrClosed
	}

	normalizedQuery, offset, err := validatePageOptions(query, opts)
	if err != nil {
		return nil, err
	}
	// 使用规范化后的查询
	query = normalizedQuery

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
		duration := storageopt.MeasureOperation(start)
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

	// 执行分页查询（传递已计算的 offset，避免重复计算）
	columns, data, err := w.executePageQuery(ctx, query, opts.PageSize, offset, args...)
	if err != nil {
		return nil, err
	}

	return &PageResult{
		Columns:    columns,
		Rows:       data,
		Total:      total,
		Page:       opts.Page,
		PageSize:   opts.PageSize,
		TotalPages: storageopt.CalculateTotalPages(total, opts.PageSize),
	}, nil
}

// queryClausePattern 用于检测查询中的 FORMAT 和 SETTINGS 子句。
// 使用单词边界匹配，忽略大小写。
//
// 已知局限性：此正则匹配可能产生误判，例如：
//   - WHERE name = 'FORMAT' → 会误判为包含 FORMAT 子句
//   - 字符串字面量或注释中的关键字可能被误判
//
// 对于复杂查询场景，建议直接使用 Client() 执行查询。
var queryClausePattern = regexp.MustCompile(`(?i)\b(FORMAT|SETTINGS)\b`)

// limitOffsetTailPattern 检测查询末尾的 LIMIT/OFFSET 子句。
// 使用末尾锚定（$）避免误匹配子查询中的 LIMIT/OFFSET。
//
// 匹配示例：
//   - "SELECT * FROM users LIMIT 10" → 匹配
//   - "SELECT * FROM users LIMIT 10 OFFSET 5" → 匹配
//   - "SELECT * FROM (SELECT * FROM t LIMIT 10) AS sub" → 不匹配（不在末尾）
var limitOffsetTailPattern = regexp.MustCompile(`(?i)\bLIMIT\s+\d+(\s+OFFSET\s+\d+)?\s*$`)

// normalizeQuery 规范化查询语句。
// 去除末尾的分号和空白字符。
func normalizeQuery(query string) string {
	return strings.TrimRight(query, " \t\n\r;")
}

// validateQuerySyntax 校验查询语法，检测不支持的子句。
// 返回规范化后的查询和可能的错误。
func validateQuerySyntax(query string) (string, error) {
	// 先规范化
	normalized := normalizeQuery(query)
	if normalized == "" {
		return "", ErrEmptyQuery
	}

	// 检测 FORMAT 和 SETTINGS 子句
	matches := queryClausePattern.FindAllString(normalized, -1)
	for _, match := range matches {
		if strings.EqualFold(match, "FORMAT") {
			return "", ErrQueryContainsFormat
		}
		if strings.EqualFold(match, "SETTINGS") {
			return "", ErrQueryContainsSettings
		}
	}

	// 检测末尾的 LIMIT/OFFSET 子句（QueryPage 自动管理分页）
	if limitOffsetTailPattern.MatchString(normalized) {
		return "", ErrQueryContainsLimitOffset
	}

	return normalized, nil
}

// validatePageOptions 验证分页参数并规范化查询。
// 返回规范化后的查询、计算后的 offset 和可能的错误。
func validatePageOptions(query string, opts PageOptions) (normalizedQuery string, offset int64, err error) {
	normalized, err := validateQuerySyntax(query)
	if err != nil {
		return "", 0, err
	}

	// 使用通用分页验证，包含溢出检查
	offset, validateErr := storageopt.ValidatePagination(opts.Page, opts.PageSize)
	if validateErr != nil {
		// 转换为包级别错误（storageopt 只返回这三种错误）
		// 使用 errors.Is 而非 == 比较，确保即使 storageopt 将来包装错误也能正确匹配
		switch {
		case errors.Is(validateErr, storageopt.ErrInvalidPage):
			return "", 0, ErrInvalidPage
		case errors.Is(validateErr, storageopt.ErrInvalidPageSize):
			return "", 0, ErrInvalidPageSize
		default:
			return "", 0, ErrPageOverflow
		}
	}

	// 限制页大小上限，防止超大 PageSize 导致 OOM
	if opts.PageSize > MaxPageSize {
		return "", 0, ErrPageSizeTooLarge
	}

	// 限制偏移量上限，防止大偏移量导致 ClickHouse 扫描放大
	if offset > MaxOffset {
		return "", 0, ErrOffsetTooLarge
	}

	return normalized, offset, nil
}

// executeCountQuery 执行计数查询。
func (w *clickhouseWrapper) executeCountQuery(ctx context.Context, query string, args ...any) (int64, error) {
	w.queryCounter.IncQuery()
	countQuery := buildCountQuery(query)
	var total int64
	if err := w.conn.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		w.queryCounter.IncQueryError()
		return 0, fmt.Errorf("count query failed: %w", err)
	}
	return total, nil
}

// executePageQuery 执行分页数据查询。
// pageSize 和 offset 由调用方传入，避免重复计算。
func (w *clickhouseWrapper) executePageQuery(ctx context.Context, query string, pageSize, offset int64, args ...any) (columns []string, data [][]any, err error) {
	w.queryCounter.IncQuery()
	pageQuery := fmt.Sprintf("%s LIMIT %d OFFSET %d", query, pageSize, offset)

	rows, queryErr := w.conn.Query(ctx, pageQuery, args...)
	if queryErr != nil {
		w.queryCounter.IncQueryError()
		return nil, nil, fmt.Errorf("page query failed: %w", queryErr)
	}
	defer func() {
		closeErr := rows.Close()
		if closeErr != nil {
			w.queryCounter.IncQueryError()
			err = errors.Join(err, fmt.Errorf("close rows failed: %w", closeErr))
		}
	}()

	columns = rows.Columns()
	data, err = w.scanRows(rows)
	return columns, data, err
}

// scanRows 扫描结果集中的所有行。
func (w *clickhouseWrapper) scanRows(rows driver.Rows) ([][]any, error) {
	columnTypes := rows.ColumnTypes()

	// 缓存每列的 ScanType，避免每行重复调用 ScanType()
	scanTypes := make([]reflect.Type, len(columnTypes))
	for i, ct := range columnTypes {
		scanTypes[i] = ct.ScanType()
	}

	var data [][]any

	for rows.Next() {
		// 为每列创建对应类型的值实例用于接收数据
		scanDest := make([]any, len(scanTypes))
		for i, scanType := range scanTypes {
			// 防护：某些特殊类型可能返回 nil ScanType，使用 *any 作为后备
			if scanType == nil {
				scanDest[i] = new(any)
			} else {
				// reflect.New 创建指向该类型零值的指针，Interface() 返回 any 类型
				scanDest[i] = reflect.New(scanType).Interface()
			}
		}

		if err := rows.Scan(scanDest...); err != nil {
			w.queryCounter.IncQueryError()
			return nil, fmt.Errorf("scan failed: %w", err)
		}

		// 提取实际值（解引用指针）
		row := make([]any, len(scanTypes))
		for i := range row {
			row[i] = reflect.ValueOf(scanDest[i]).Elem().Interface()
		}
		data = append(data, row)
	}

	if err := rows.Err(); err != nil {
		w.queryCounter.IncQueryError()
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return data, nil
}

// =============================================================================
// 批量插入实现
// =============================================================================

// tableNamePattern 用于校验表名的合法性。
// 支持格式：table_name、database.table_name、`database`.`table_name`
// 允许字母、数字、下划线、点号和反引号。
//
// 设计决策: 反引号内禁止控制字符（\x00-\x1f）以防止换行符注入风险。
// 不支持混合引用风格（如 db.`table`），这是有意的安全限制。
var tableNamePattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*(\.[a-zA-Z_][a-zA-Z0-9_]*)?$|^` + "`[^`\\x00-\\x1f]+`" + `(\.` + "`[^`\\x00-\\x1f]+`" + `)?$`)

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
	if w.closed.Load() {
		return nil, ErrClosed
	}

	if err := validateTableName(table); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrEmptyRows
	}

	batchSize := opts.BatchSize
	if batchSize < 1 {
		batchSize = DefaultBatchSize
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
		duration := storageopt.MeasureOperation(start)
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

		end := min(i+batchSize, len(rows))

		batch := rows[i:end]
		count, batchErrs := w.insertBatch(ctx, table, batch)
		insertedCount += count
		if len(batchErrs) > 0 {
			errs = append(errs, batchErrs...)
		}
	}

	return insertedCount, errs
}

// 设计决策: fmt.Sprintf 拼接表名是安全的，因为 table 在 BatchInsert 入口处
// 已通过 validateTableName 的严格正则校验，仅允许合法标识符字符。
func (w *clickhouseWrapper) insertBatch(ctx context.Context, table string, batch []any) (appendedCount int64, errs []error) {
	batchObj, err := w.conn.PrepareBatch(ctx, fmt.Sprintf("INSERT INTO %s", table))
	if err != nil {
		return 0, []error{fmt.Errorf("prepare batch failed: %w", err)}
	}

	// 追加所有行到批次
	appendedCount, errs = w.appendRowsToBatch(ctx, batchObj, batch)

	// 如果没有成功追加任何行，中止批次
	if appendedCount == 0 {
		w.abortBatch(batchObj, &errs)
		return 0, errs
	}

	// 设计决策: context 取消后中止批次而非发送部分数据。
	// 在重试场景下，发送部分数据可能导致重复写入和语义不一致。
	// 调用方应通过 InsertedCount 判断实际写入量并决定后续操作。
	if ctx.Err() != nil {
		errs = append(errs, fmt.Errorf("context canceled before send: %w", ctx.Err()))
		w.abortBatch(batchObj, &errs)
		return 0, errs
	}

	// 发送批次
	if err := batchObj.Send(); err != nil {
		errs = append(errs, fmt.Errorf("send batch failed: %w", err))
		return 0, errs
	}

	return appendedCount, errs
}

// appendRowsToBatch 将行追加到批次中。
// 每 100 行检查一次 context，平衡性能和响应性。
func (w *clickhouseWrapper) appendRowsToBatch(ctx context.Context, batchObj driver.Batch, batch []any) (appendedCount int64, errs []error) {
	const checkInterval = 100
	for i, row := range batch {
		// 定期检查 context 是否已取消
		if i > 0 && i%checkInterval == 0 && ctx.Err() != nil {
			errs = append(errs, fmt.Errorf("context canceled during append at row %d: %w", i, ctx.Err()))
			return appendedCount, errs
		}
		if err := batchObj.AppendStruct(row); err != nil {
			errs = append(errs, fmt.Errorf("append struct failed: %w", err))
			continue
		}
		appendedCount++
	}
	return appendedCount, errs
}

// abortBatch 中止批次并记录错误。
func (w *clickhouseWrapper) abortBatch(batchObj driver.Batch, errs *[]error) {
	if abortErr := batchObj.Abort(); abortErr != nil {
		*errs = append(*errs, fmt.Errorf("abort batch failed: %w", abortErr))
	}
}

// =============================================================================
// 慢查询检测
// =============================================================================

// maybeSlowQuery 检测并可能触发慢查询钩子。
// 使用 slowQueryDetector 统一处理同步和异步钩子。
func (w *clickhouseWrapper) maybeSlowQuery(ctx context.Context, info SlowQueryInfo) bool {
	if w.slowQueryDetector == nil {
		return false
	}

	isSlow := w.slowQueryDetector.MaybeSlowQuery(ctx, info, info.Duration)
	if isSlow {
		w.slowQueryCounter.Inc()
	}
	return isSlow
}

// =============================================================================
// 辅助函数
// =============================================================================

// buildCountQuery 根据原始查询构建 COUNT 查询。
// 使用子查询包装方式，避免复杂 SQL 解析问题（子查询、CTE、UNION 等）。
//
// 性能说明：
// 子查询方式对于简单查询可能比直接改写 SELECT 列表性能略差，
// 但能正确处理复杂 SQL（子查询、CTE、UNION、DISTINCT 等）。
// 对于性能敏感的简单查询，建议直接使用 Client() 执行手写的 COUNT 语句。
func buildCountQuery(query string) string {
	return fmt.Sprintf("SELECT COUNT(*) FROM (%s) AS _count_subquery", query)
}
