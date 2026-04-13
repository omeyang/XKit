package xid

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sony/sonyflake/v2"
)

// =============================================================================
// 错误定义
// =============================================================================

var (
	// ErrNotInitialized 生成器未初始化。
	// 当用户显式调用 Init 但失败后，后续包级函数（New/NewString 等）返回此错误。
	// 此时自动初始化被禁用以尊重用户意图，请修复 Init 失败原因后重新调用 Init。
	ErrNotInitialized = errors.New("xid: generator not initialized (Init was called but failed; call Init again to retry)")

	// ErrClockBackwardTimeout 时钟回拨等待超时
	ErrClockBackwardTimeout = errors.New("xid: clock backward wait timeout")

	// ErrAlreadyInitialized 生成器已初始化。
	// 第二次调用 Init 时返回此错误。如需多个生成器，请使用 NewGenerator。
	ErrAlreadyInitialized = errors.New("xid: generator already initialized")

	// ErrInvalidID ID 值无效（零或负数）。
	// Parse 解析出非正值时返回此错误。
	ErrInvalidID = errors.New("xid: invalid id")

	// ErrOverTimeLimit 时间分量溢出，生成器无法继续生成 ID。
	// 这是不可恢复的错误，NewWithRetry 不会重试此错误。
	ErrOverTimeLimit = errors.New("xid: time component overflow")

	// ErrNoPrivateAddress 无法找到私有 IP 地址。
	// 当所有机器 ID 获取策略（环境变量、主机名）均失败，
	// 且系统没有私有 IPv4 地址时，DefaultMachineID 返回此错误。
	ErrNoPrivateAddress = errors.New("xid: no private IP address found")

	// ErrNilContext context 参数为 nil。
	// 非 Must* API 不应 panic，调用方应传入有效的 context（至少 context.Background()）。
	ErrNilContext = errors.New("xid: nil context")

	// ErrInvalidConfig 配置参数无效。
	// NewGenerator 校验 maxWaitDuration 或 retryInterval 为负值时返回此错误。
	// sonyflake.New 初始化失败（如 CheckMachineID 验证不通过）时也包裹为此错误。
	ErrInvalidConfig = errors.New("xid: invalid config")

	// ErrNilGenerator 生成器实例为 nil 或未通过 NewGenerator 创建。
	// 当直接使用零值 Generator 或 nil *Generator 调用方法时返回此错误。
	// 请始终通过 NewGenerator 创建生成器实例。
	ErrNilGenerator = errors.New("xid: nil generator (use NewGenerator to create)")
)

// =============================================================================
// 时钟回拨配置
// =============================================================================

const (
	// DefaultMaxWaitDuration 默认最大等待时间（时钟回拨时）
	// sonyflake 时间精度是 10ms，通常回拨不会超过几百毫秒
	DefaultMaxWaitDuration = 500 * time.Millisecond

	// DefaultRetryInterval 默认重试间隔
	DefaultRetryInterval = 10 * time.Millisecond
)

// =============================================================================
// ID 位布局常量（Sonyflake v2）
// =============================================================================

// 设计决策: 以下常量对应 Sonyflake v2 的固定位布局（39+8+16=63 bits），
// 不随 sonyflake.Settings 配置变化。如果升级 Sonyflake 大版本且位布局改变，
// 需同步更新这些常量和 Decompose 函数。
const (
	machineBits  = 16
	sequenceBits = 8
	timeBits     = 39
	machineMask  = (1 << machineBits) - 1  // 0xFFFF
	sequenceMask = (1 << sequenceBits) - 1 // 0xFF
	maxTimeValue = (1 << timeBits) - 1     // 39 位最大值
)

// =============================================================================
// Decompose 结果
// =============================================================================

// Components 表示 Sonyflake ID 分解后的各组成部分。
//
// 设计决策: 所有字段统一使用 int64 而非精确类型（如 uint8/uint16），
// 理由：1) 简化 JSON 序列化（避免 uint 类型在某些序列化框架中的问题）；
// 2) 避免调用方频繁的类型转换噪音；3) 作为只读返回值结构体，值域约束
// 已由注释和 Decompose 实现保证，无需类型系统强制。
type Components struct {
	// ID 原始 ID 值
	ID int64
	// Time 时间戳部分（10ms 为单位，自 Sonyflake epoch 起，39 位，有效范围 0-549,755,813,887）
	Time int64
	// Sequence 序列号部分（同一时间单位内的递增序号，8 位，有效范围 0-255）
	Sequence int64
	// Machine 机器 ID 部分（16 位，有效范围 0-65535）
	Machine int64
}

// =============================================================================
// Generator - 实例化的 ID 生成器
// =============================================================================

// Generator 分布式唯一 ID 生成器。
//
// 支持两种使用方式：
//   - 实例化：通过 NewGenerator 创建独立实例，适用于依赖注入和测试隔离
//   - 全局函数：通过包级别函数（New/NewString 等）使用默认全局实例
//
// Generator 的所有方法都是并发安全的。
type Generator struct {
	// 设计决策: sf 字段运行时通过 generateID 间接使用。保留此引用
	// 明确 Generator 对 sonyflake 实例的所有权，便于调试和未来扩展。
	sf              *sonyflake.Sonyflake
	maxWaitDuration time.Duration
	retryInterval   time.Duration
	// generateID 生成下一个 ID。默认为 sf.NextID，测试中可替换。
	generateID func() (int64, error)
}

// NewGenerator 创建新的 ID 生成器实例。
//
// 与包级别函数（Init/New 等）不同，每次调用 NewGenerator 都会创建独立的生成器。
// 这使得：
//   - 测试可以创建独立的生成器实例，互不干扰
//   - 应用可以通过依赖注入传递生成器
//   - 不同组件可以使用不同配置的生成器
//
// 如果不传入 WithMachineID 选项，默认使用 DefaultMachineID 获取机器 ID。
//
// 设计决策: 返回 *Generator 而非接口。xid 是底层工具包，调用方通常使用包级函数
// 而非实例方法。需要依赖注入的场景（如 xsemaphore）已通过 IDGeneratorFunc 函数类型解耦，
// 无需额外接口。返回具体类型避免过度抽象，符合 "accept interfaces, return structs" 惯例。
func NewGenerator(opts ...Option) (*Generator, error) {
	cfg := &options{}
	// 设计决策: nil Option 静默跳过而非返回错误。xid 是底层工具包，
	// 跳过 nil 便于条件式构建 Option 列表（如 append 后展开）。
	// xconf 采用 fail-fast（ErrNilOption），两者适用场景不同，
	// 待跨包审查（99-cross-package）统一后在 04-style-baseline.md 锁定。
	for _, opt := range opts {
		if opt != nil {
			opt(cfg)
		}
	}

	// fail-fast：先校验配置参数，再创建 sonyflake 实例
	if cfg.maxWaitDuration < 0 {
		return nil, fmt.Errorf("%w: max wait duration must be non-negative, got %s", ErrInvalidConfig, cfg.maxWaitDuration)
	}
	if cfg.retryInterval < 0 {
		return nil, fmt.Errorf("%w: retry interval must be non-negative, got %s", ErrInvalidConfig, cfg.retryInterval)
	}

	settings := sonyflake.Settings{}

	// 使用自定义或默认的机器 ID 函数
	machineIDFn := cfg.machineID
	if machineIDFn == nil {
		machineIDFn = DefaultMachineID
	}
	settings.MachineID = func() (int, error) {
		id, err := machineIDFn()
		return int(id), err
	}

	if cfg.checkMachineID != nil {
		settings.CheckMachineID = func(id int) bool {
			return cfg.checkMachineID(uint16(id))
		}
	}

	sf, err := sonyflake.New(settings)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidConfig, err)
	}

	g := &Generator{
		sf:              sf,
		maxWaitDuration: DefaultMaxWaitDuration,
		retryInterval:   DefaultRetryInterval,
	}
	g.generateID = sf.NextID
	// 设计决策: 使用 maxWaitSet/retryIntervalSet 标志区分"未传入"与"显式传入 0"。
	// 未传入 → 使用默认值；显式传入 0 → 表示"不等待/无间隔"，语义明确。
	if cfg.maxWaitSet {
		g.maxWaitDuration = cfg.maxWaitDuration
	}
	if cfg.retryIntervalSet {
		g.retryInterval = cfg.retryInterval
	}

	return g, nil
}

// =============================================================================
// Generator 实例方法
// =============================================================================

// validate 校验生成器实例是否可用。
// 防止零值 Generator 或 nil *Generator 导致 nil pointer panic。
func (g *Generator) validate() error {
	if g == nil || g.generateID == nil {
		return ErrNilGenerator
	}
	return nil
}

// New 生成新的唯一 ID（int64 格式）。
//
// 时间分量溢出时返回 [ErrOverTimeLimit]（不可恢复）。
func (g *Generator) New() (int64, error) {
	if err := g.validate(); err != nil {
		return 0, err
	}
	id, err := g.generateID()
	if err != nil {
		// 统一映射底层溢出错误，保持与 NewWithRetry 一致的错误契约
		if errors.Is(err, sonyflake.ErrOverTimeLimit) {
			return 0, fmt.Errorf("%w: %w", ErrOverTimeLimit, err)
		}
		return 0, err
	}
	return id, nil
}

// NewWithRetry 生成新的唯一 ID，遇到可重试错误时自动等待重试。
//
// 重试策略：
//   - [sonyflake.ErrOverTimeLimit]（时间分量溢出）不可恢复，立即返回 [ErrOverTimeLimit]
//   - 其余错误视为可重试（Sonyflake v2 实际只返回 ErrOverTimeLimit，此策略为前向兼容预留）
//
// 支持通过 context 取消等待。如果等待超过 maxWaitDuration（默认 500ms）
// 仍无法生成，返回 [ErrClockBackwardTimeout]。
//
// 如果 ctx 为 nil，返回 [ErrNilContext]。
func (g *Generator) NewWithRetry(ctx context.Context) (int64, error) {
	if err := g.validate(); err != nil {
		return 0, err
	}
	if ctx == nil {
		return 0, ErrNilContext
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	// 快速路径：首次尝试成功则零额外分配（避免提前创建 timer）
	id, err := g.generateID()
	if err == nil {
		return id, nil
	}

	return g.retryGenerateID(ctx, err)
}

// retryGenerateID 处理 NewWithRetry 的重试循环。
// 从 NewWithRetry 中提取以降低 cyclomatic complexity（gocyclo ≤ 10）。
func (g *Generator) retryGenerateID(ctx context.Context, firstErr error) (int64, error) {
	// 不可恢复的溢出错误，立即返回
	if errors.Is(firstErr, sonyflake.ErrOverTimeLimit) {
		return 0, fmt.Errorf("%w: %w", ErrOverTimeLimit, firstErr)
	}

	// 惰性创建 timer：仅在需要重试时分配（Go 1.23+ Reset 安全清空 channel）
	deadline := time.Now().Add(g.maxWaitDuration)
	lastErr := firstErr
	timer := time.NewTimer(0)
	<-timer.C // 排空初始触发
	defer timer.Stop()

	for {
		// 检查剩余时间，按"剩余时间"裁剪等待间隔，避免超过 maxWaitDuration
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return 0, fmt.Errorf("%w: %w", ErrClockBackwardTimeout, lastErr)
		}

		// 等待后重试，支持 context 取消
		timer.Reset(min(g.retryInterval, remaining))
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-timer.C:
		}

		id, err := g.generateID()
		if err == nil {
			return id, nil
		}
		lastErr = err

		// 重试策略：仅 ErrOverTimeLimit 不可恢复，立即返回。
		// Sonyflake v2 的 NextID 只会返回 ErrOverTimeLimit（时钟回拨在内部处理），
		// 因此"其余错误均重试"在实践中等价于"仅重试时钟回拨"。
		if errors.Is(err, sonyflake.ErrOverTimeLimit) {
			return 0, fmt.Errorf("%w: %w", ErrOverTimeLimit, err)
		}
	}
}

// NewString 生成新的唯一 ID（字符串格式）。
//
// 使用 base36 编码，结果为 12-13 个字符。
func (g *Generator) NewString() (string, error) {
	id, err := g.New()
	if err != nil {
		return "", err
	}
	return strconv.FormatInt(id, 36), nil
}

// NewStringWithRetry 生成新的唯一 ID（字符串格式），遇到时钟回拨时自动等待重试。
//
// 支持通过 context 取消等待。详见 NewWithRetry。
func (g *Generator) NewStringWithRetry(ctx context.Context) (string, error) {
	id, err := g.NewWithRetry(ctx)
	if err != nil {
		return "", err
	}
	return strconv.FormatInt(id, 36), nil
}

// MustNewWithRetry 生成新的唯一 ID，遇到时钟回拨时自动等待重试，失败时 panic。
//
// 内部使用 context.Background()。如需自定义 context，请使用 NewWithRetry。
func (g *Generator) MustNewWithRetry() int64 {
	id, err := g.NewWithRetry(context.Background())
	if err != nil {
		panic(err)
	}
	return id
}

// MustNewStringWithRetry 生成新的唯一 ID（字符串格式），遇到时钟回拨时自动等待重试，失败时 panic。
//
// 适用于明确接受 crash-fast 策略的场景（如启动时预生成 ID）。
// 生产环境建议使用 [Generator.NewStringWithRetry] 以便自定义错误处理。
// 内部使用 context.Background()。如需自定义 context，请使用 NewStringWithRetry。
func (g *Generator) MustNewStringWithRetry() string {
	s, err := g.NewStringWithRetry(context.Background())
	if err != nil {
		panic(err)
	}
	return s
}

// =============================================================================
// 全局单例
// =============================================================================

var (
	defaultGen atomic.Pointer[Generator]
	initMu     sync.Mutex
	// initCalled 标记用户是否显式调用过 Init。一旦为 true，
	// ensureInitialized 不再自动初始化，避免覆盖用户意图。受 initMu 保护。
	initCalled bool
)

// =============================================================================
// 初始化
// =============================================================================

// Init 初始化全局 ID 生成器。
//
// 如果不调用 Init，首次生成 ID 时会使用默认配置自动初始化。
// 默认配置使用 DefaultMachineID 获取机器 ID。
//
// Init 只能成功一次，成功后重复调用返回 [ErrAlreadyInitialized]。
// 如果在 Init 之前已通过 New/NewString 等函数触发了自动初始化，
// 同样返回 [ErrAlreadyInitialized]。
// 建议在应用启动时调用，以便尽早发现配置问题。
//
// 与 sync.Once 不同，如果 Init 因瞬态错误失败（如网络不可用导致
// 机器 ID 获取失败），可以再次调用 Init 重试。
//
// 如果需要多个独立生成器（如测试场景），请使用 NewGenerator。
func Init(opts ...Option) error {
	initMu.Lock()
	defer initMu.Unlock()
	if defaultGen.Load() != nil {
		return ErrAlreadyInitialized
	}
	initCalled = true
	gen, err := NewGenerator(opts...)
	if err != nil {
		return err
	}
	defaultGen.Store(gen)
	return nil
}

// ensureInitialized 确保生成器已初始化，返回可用的生成器。
//
// 使用 double-checked locking：快速路径仅需一次原子 Load。
// 如果用户显式调用过 Init 但失败了，不会自动用默认配置覆盖用户意图，
// 而是返回 ErrNotInitialized，提示用户重新 Init。
func ensureInitialized() (*Generator, error) {
	if gen := defaultGen.Load(); gen != nil {
		return gen, nil
	}
	initMu.Lock()
	defer initMu.Unlock()
	if gen := defaultGen.Load(); gen != nil {
		return gen, nil
	}
	// 用户显式调用过 Init 但失败了，不覆盖用户意图
	if initCalled {
		return nil, ErrNotInitialized
	}
	gen, err := NewGenerator()
	if err != nil {
		return nil, err
	}
	defaultGen.Store(gen)
	return gen, nil
}

// =============================================================================
// 全局便捷函数 - ID 生成
// =============================================================================

// New 生成新的唯一 ID（int64 格式）。
//
// 如果生成器未初始化，会使用默认配置自动初始化。
// 时间分量溢出时返回 [ErrOverTimeLimit]（不可恢复）。
func New() (int64, error) {
	gen, err := ensureInitialized()
	if err != nil {
		return 0, err
	}
	return gen.New()
}

// NewWithRetry 生成新的唯一 ID，遇到可重试错误时自动等待重试。
//
// 这是生产环境推荐使用的方法，能够容忍短暂的时钟异常（NTP 同步等）。
// 支持通过 context 取消等待。
// 如果等待超过 maxWaitDuration（默认 500ms）仍无法生成，返回 [ErrClockBackwardTimeout]。
func NewWithRetry(ctx context.Context) (int64, error) {
	gen, err := ensureInitialized()
	if err != nil {
		return 0, err
	}
	return gen.NewWithRetry(ctx)
}

// NewString 生成新的唯一 ID（字符串格式）。
//
// 使用 base36 编码，结果为 12-13 个字符。
// 返回错误通常是因为时钟回拨。
func NewString() (string, error) {
	gen, err := ensureInitialized()
	if err != nil {
		return "", err
	}
	return gen.NewString()
}

// NewStringWithRetry 生成新的唯一 ID（字符串格式），遇到可重试错误时自动等待重试。
//
// 这是生产环境推荐使用的方法。支持通过 context 取消等待。详见 [NewWithRetry]。
func NewStringWithRetry(ctx context.Context) (string, error) {
	gen, err := ensureInitialized()
	if err != nil {
		return "", err
	}
	return gen.NewStringWithRetry(ctx)
}

// MustNewWithRetry 生成新的唯一 ID，遇到可重试错误时自动等待重试，失败时 panic。
//
// 适用于明确接受 crash-fast 策略的场景。
// 生产环境建议使用 [NewWithRetry] 以便自定义错误处理。
// 内部使用 context.Background()。如需自定义 context，请使用 NewWithRetry。
func MustNewWithRetry() int64 {
	gen, err := ensureInitialized()
	if err != nil {
		panic(err)
	}
	return gen.MustNewWithRetry()
}

// MustNewStringWithRetry 生成新的唯一 ID（字符串格式），遇到可重试错误时自动等待重试，失败时 panic。
//
// 适用于明确接受 crash-fast 策略的场景。
// 生产环境建议使用 [NewStringWithRetry] 以便自定义错误处理。
// 内部使用 context.Background()。如需自定义 context，请使用 NewStringWithRetry。
func MustNewStringWithRetry() string {
	gen, err := ensureInitialized()
	if err != nil {
		panic(err)
	}
	return gen.MustNewStringWithRetry()
}

// =============================================================================
// 全局便捷函数 - ID 解析
// =============================================================================

// Parse 从字符串解析 ID。
//
// 字符串必须是 base36 编码的格式（由 NewString 生成）。
// 验证格式和正数约束。
// 所有无效输入（语法错误、溢出、非正值）均返回 [ErrInvalidID]。
//
// 设计决策: Parse 采用宽松解析（大小写不敏感，允许前导 "+"），
// 与 strconv.ParseInt 行为一致。NewString 的输出（小写、无前缀）是规范形式，
// 但 Parse 不强制规范性校验，以便兼容外部系统可能引入的大小写变换。
//
// 注意：int64 正数范围恰好覆盖 Sonyflake 的 63 位有效位（39+8+16），
// 因此 strconv.ParseInt 的溢出检查已隐含位范围校验，无需额外检查。
func Parse(s string) (int64, error) {
	id, err := strconv.ParseInt(s, 36, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %w", ErrInvalidID, err)
	}
	if id <= 0 {
		return 0, fmt.Errorf("%w: value must be positive, got %d", ErrInvalidID, id)
	}
	return id, nil
}

// Decompose 分解 ID 为各个组成部分。
//
// 这是纯函数，不需要生成器初始化即可使用。
// 基于 Sonyflake v2 的固定位布局（39 bits 时间 + 8 bits 序列 + 16 bits 机器）
// 直接进行位提取。
//
// 返回 [ErrInvalidID] 如果 id 不是正数。
func Decompose(id int64) (Components, error) {
	if id <= 0 {
		return Components{}, fmt.Errorf("%w: value must be positive, got %d", ErrInvalidID, id)
	}
	return Components{
		ID:       id,
		Machine:  id & machineMask,
		Sequence: (id >> machineBits) & sequenceMask,
		Time:     id >> (machineBits + sequenceBits),
	}, nil
}
