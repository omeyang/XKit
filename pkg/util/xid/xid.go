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
	// ErrNotInitialized 生成器未初始化
	ErrNotInitialized = errors.New("xid: generator not initialized")

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
type Components struct {
	// ID 原始 ID 值
	ID int64
	// Time 时间戳部分（10ms 为单位，自 Sonyflake epoch 起）
	Time int64
	// Sequence 序列号部分（同一时间单位内的递增序号）
	Sequence int64
	// Machine 机器 ID 部分
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
func NewGenerator(opts ...Option) (*Generator, error) {
	cfg := &options{}
	for _, opt := range opts {
		if opt != nil {
			opt(cfg)
		}
	}

	// fail-fast：先校验配置参数，再创建 sonyflake 实例
	if cfg.maxWaitDuration < 0 {
		return nil, fmt.Errorf("xid: max wait duration must be non-negative, got %s", cfg.maxWaitDuration)
	}
	if cfg.retryInterval < 0 {
		return nil, fmt.Errorf("xid: retry interval must be non-negative, got %s", cfg.retryInterval)
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
		return nil, err
	}

	g := &Generator{
		sf:              sf,
		maxWaitDuration: DefaultMaxWaitDuration,
		retryInterval:   DefaultRetryInterval,
	}
	g.generateID = sf.NextID
	if cfg.maxWaitDuration > 0 {
		g.maxWaitDuration = cfg.maxWaitDuration
	}
	if cfg.retryInterval > 0 {
		g.retryInterval = cfg.retryInterval
	}

	return g, nil
}

// =============================================================================
// Generator 实例方法
// =============================================================================

// New 生成新的唯一 ID（int64 格式）。
//
// 返回错误通常是因为时钟回拨。
func (g *Generator) New() (int64, error) {
	return g.generateID()
}

// NewWithRetry 生成新的唯一 ID，遇到时钟回拨时自动等待重试。
//
// 仅对可重试错误（如时钟回拨）进行等待重试。
// 不可恢复的错误（如 sonyflake 时间溢出）会立即返回，不会浪费等待时间。
//
// 支持通过 context 取消等待。如果等待超过 maxWaitDuration（默认 500ms）
// 仍无法生成，返回 ErrClockBackwardTimeout。
func (g *Generator) NewWithRetry(ctx context.Context) (int64, error) {
	if ctx == nil {
		panic("xid: nil Context")
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	deadline := time.Now().Add(g.maxWaitDuration)
	var lastErr error

	for {
		id, err := g.generateID()
		if err == nil {
			return id, nil
		}
		lastErr = err

		// 重试策略：仅 ErrOverTimeLimit 不可恢复，立即返回。
		// Sonyflake v2 的 NextID 只会返回 ErrOverTimeLimit（时钟回拨在内部处理），
		// 因此"其余错误均重试"在实践中等价于"仅重试时钟回拨"。
		if errors.Is(err, sonyflake.ErrOverTimeLimit) {
			return 0, fmt.Errorf("%w: %v", ErrOverTimeLimit, err)
		}

		// 检查是否超时
		if time.Now().After(deadline) {
			return 0, fmt.Errorf("%w: last error was %v", ErrClockBackwardTimeout, lastErr)
		}

		// 等待后重试，支持 context 取消
		timer := time.NewTimer(g.retryInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return 0, ctx.Err()
		case <-timer.C:
		}
	}
}

// NewString 生成新的唯一 ID（字符串格式）。
//
// 使用 base36 编码，结果约 11-13 个字符。
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
// 这是生产环境推荐的默认方法。
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
	initCalled bool // 受 initMu 保护
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
// 返回错误通常是因为时钟回拨。
func New() (int64, error) {
	gen, err := ensureInitialized()
	if err != nil {
		return 0, err
	}
	return gen.New()
}

// NewWithRetry 生成新的唯一 ID，遇到时钟回拨时自动等待重试。
//
// 这是生产环境推荐使用的方法，能够容忍短暂的时钟回拨（NTP 同步等）。
// 支持通过 context 取消等待。
// 如果等待超过 maxWaitDuration（默认 500ms）仍无法生成，返回 ErrClockBackwardTimeout。
func NewWithRetry(ctx context.Context) (int64, error) {
	gen, err := ensureInitialized()
	if err != nil {
		return 0, err
	}
	return gen.NewWithRetry(ctx)
}

// NewString 生成新的唯一 ID（字符串格式）。
//
// 使用 base36 编码，结果约 11-13 个字符。
// 返回错误通常是因为时钟回拨。
func NewString() (string, error) {
	gen, err := ensureInitialized()
	if err != nil {
		return "", err
	}
	return gen.NewString()
}

// NewStringWithRetry 生成新的唯一 ID（字符串格式），遇到时钟回拨时自动等待重试。
//
// 这是生产环境推荐使用的方法。支持通过 context 取消等待。详见 NewWithRetry。
func NewStringWithRetry(ctx context.Context) (string, error) {
	gen, err := ensureInitialized()
	if err != nil {
		return "", err
	}
	return gen.NewStringWithRetry(ctx)
}

// MustNewWithRetry 生成新的唯一 ID，遇到时钟回拨时自动等待重试，失败时 panic。
//
// 这是生产环境推荐使用的方法，自动处理短暂的时钟回拨。
// 内部使用 context.Background()。如需自定义 context，请使用 NewWithRetry。
func MustNewWithRetry() int64 {
	gen, err := ensureInitialized()
	if err != nil {
		panic(err)
	}
	return gen.MustNewWithRetry()
}

// MustNewStringWithRetry 生成新的唯一 ID（字符串格式），遇到时钟回拨时自动等待重试，失败时 panic。
//
// 这是生产环境推荐的默认方法，自动处理短暂的时钟回拨。
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
// 验证格式、正数约束以及 Sonyflake 有效位范围（39+8+16=63 位）。
// 所有无效输入（语法错误、溢出、非正值、超位范围）均返回 [ErrInvalidID]。
func Parse(s string) (int64, error) {
	id, err := strconv.ParseInt(s, 36, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrInvalidID, err)
	}
	if id <= 0 {
		return 0, fmt.Errorf("%w: value must be positive, got %d", ErrInvalidID, id)
	}
	// 校验时间分量是否在 39 位范围内
	if id>>(sequenceBits+machineBits) > maxTimeValue {
		return 0, fmt.Errorf("%w: time component exceeds %d-bit range", ErrInvalidID, timeBits)
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
