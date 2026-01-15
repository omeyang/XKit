package xid

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/sony/sonyflake/v2"
)

// =============================================================================
// 错误定义
// =============================================================================

var (
	// ErrNotInitialized 生成器未初始化
	ErrNotInitialized = errors.New("xid: generator not initialized")

	// ErrClockBackward 时钟回拨
	ErrClockBackward = errors.New("xid: clock moved backward")

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
	cfg := &config{}
	for _, opt := range opts {
		if opt != nil {
			opt(cfg)
		}
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
			return cfg.checkMachineID(uint16(id)) //nolint:gosec // id is guaranteed to be 0-65535 by sonyflake
		}
	}

	sf, err := sonyflake.New(settings)
	if err != nil {
		return nil, err
	}

	// 验证时钟回拨配置（fail-fast：显式传入非正值立即报错）
	if cfg.maxWaitDuration < 0 {
		return nil, fmt.Errorf("xid: max wait duration must be non-negative, got %s", cfg.maxWaitDuration)
	}
	if cfg.retryInterval < 0 {
		return nil, fmt.Errorf("xid: retry interval must be non-negative, got %s", cfg.retryInterval)
	}

	g := &Generator{
		sf:              sf,
		maxWaitDuration: DefaultMaxWaitDuration,
		retryInterval:   DefaultRetryInterval,
	}
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
	return g.sf.NextID()
}

// NewWithRetry 生成新的唯一 ID，遇到时钟回拨时自动等待重试。
//
// 仅对可重试错误（如时钟回拨）进行等待重试。
// 不可恢复的错误（如 sonyflake 时间溢出）会立即返回，不会浪费等待时间。
//
// 支持通过 context 取消等待。如果等待超过 maxWaitDuration（默认 500ms）
// 仍无法生成，返回 ErrClockBackwardTimeout。
func (g *Generator) NewWithRetry(ctx context.Context) (int64, error) {
	deadline := time.Now().Add(g.maxWaitDuration)
	var lastErr error

	for {
		id, err := g.sf.NextID()
		if err == nil {
			return id, nil
		}
		lastErr = err

		// 不可恢复的错误（如时间溢出）立即返回，不重试
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

// MustNew 生成新的唯一 ID，失败时 panic。
func (g *Generator) MustNew() int64 {
	id, err := g.New()
	if err != nil {
		panic(err)
	}
	return id
}

// MustNewString 生成新的唯一 ID（字符串格式），失败时 panic。
func (g *Generator) MustNewString() string {
	s, err := g.NewString()
	if err != nil {
		panic(err)
	}
	return s
}

// =============================================================================
// 全局单例
// =============================================================================

var (
	defaultGen     *Generator
	defaultGenOnce sync.Once
	defaultGenErr  error
)

// =============================================================================
// 初始化
// =============================================================================

// Init 初始化全局 ID 生成器。
//
// 如果不调用 Init，首次生成 ID 时会使用默认配置自动初始化。
// 默认配置使用 DefaultMachineID 获取机器 ID。
//
// Init 只能调用一次，重复调用返回 [ErrAlreadyInitialized]。
// 如果在 Init 之前已通过 New/NewString 等函数触发了自动初始化，
// 同样返回 [ErrAlreadyInitialized]。
// 建议在应用启动时调用，以便尽早发现配置问题。
//
// 如果需要多个独立生成器（如测试场景），请使用 NewGenerator。
func Init(opts ...Option) error {
	called := false
	defaultGenOnce.Do(func() {
		called = true
		defaultGen, defaultGenErr = NewGenerator(opts...)
	})
	if !called {
		return ErrAlreadyInitialized
	}
	return defaultGenErr
}

// ensureInitialized 确保生成器已初始化
func ensureInitialized() error {
	defaultGenOnce.Do(func() {
		defaultGen, defaultGenErr = NewGenerator()
	})
	return defaultGenErr
}

// =============================================================================
// 全局便捷函数 - ID 生成
// =============================================================================

// New 生成新的唯一 ID（int64 格式）。
//
// 如果生成器未初始化，会使用默认配置自动初始化。
// 返回错误通常是因为时钟回拨。
func New() (int64, error) {
	if err := ensureInitialized(); err != nil {
		return 0, err
	}
	return defaultGen.New()
}

// NewWithRetry 生成新的唯一 ID，遇到时钟回拨时自动等待重试。
//
// 这是生产环境推荐使用的方法，能够容忍短暂的时钟回拨（NTP 同步等）。
// 支持通过 context 取消等待。
// 如果等待超过 maxWaitDuration（默认 500ms）仍无法生成，返回 ErrClockBackwardTimeout。
func NewWithRetry(ctx context.Context) (int64, error) {
	if err := ensureInitialized(); err != nil {
		return 0, err
	}
	return defaultGen.NewWithRetry(ctx)
}

// NewString 生成新的唯一 ID（字符串格式）。
//
// 使用 base36 编码，结果约 11-13 个字符。
// 返回错误通常是因为时钟回拨。
func NewString() (string, error) {
	if err := ensureInitialized(); err != nil {
		return "", err
	}
	return defaultGen.NewString()
}

// NewStringWithRetry 生成新的唯一 ID（字符串格式），遇到时钟回拨时自动等待重试。
//
// 这是生产环境推荐使用的方法。支持通过 context 取消等待。详见 NewWithRetry。
func NewStringWithRetry(ctx context.Context) (string, error) {
	if err := ensureInitialized(); err != nil {
		return "", err
	}
	return defaultGen.NewStringWithRetry(ctx)
}

// MustNew 生成新的唯一 ID，失败时 panic。
//
// 注意：时钟回拨时会 panic。生产环境建议使用 MustNewWithRetry。
func MustNew() int64 {
	if err := ensureInitialized(); err != nil {
		panic(err)
	}
	return defaultGen.MustNew()
}

// MustNewString 生成新的唯一 ID（字符串格式），失败时 panic。
//
// 注意：时钟回拨时会 panic。生产环境建议使用 MustNewStringWithRetry。
func MustNewString() string {
	if err := ensureInitialized(); err != nil {
		panic(err)
	}
	return defaultGen.MustNewString()
}

// =============================================================================
// 全局便捷函数 - ID 解析
// =============================================================================

// Parse 从字符串解析 ID。
//
// 字符串必须是 base36 编码的格式（由 NewString 生成）。
// 验证格式、正数约束以及 Sonyflake 有效位范围（39+8+16=63 位）。
// 返回 [ErrInvalidID] 如果解析结果不是有效的 Sonyflake ID。
func Parse(s string) (int64, error) {
	id, err := strconv.ParseInt(s, 36, 64)
	if err != nil {
		return 0, err
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
