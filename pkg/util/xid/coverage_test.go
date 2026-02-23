package xid

import (
	"context"
	"errors"
	"net/netip"
	"strconv"
	"testing"
	"time"

	"github.com/sony/sonyflake/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newOverflowGenerator 创建一个 NextID() 总是返回 ErrOverTimeLimit 的生成器。
// 原理：将 sonyflake 的 StartTime 设为 200 年前，时间分量溢出 39 位上限。
func newOverflowGenerator(t *testing.T) *Generator {
	t.Helper()
	sf, err := sonyflake.New(sonyflake.Settings{
		StartTime: time.Now().Add(-200 * 365 * 24 * time.Hour),
		MachineID: func() (int, error) { return 1, nil },
	})
	require.NoError(t, err)
	g := &Generator{
		sf:              sf,
		maxWaitDuration: 50 * time.Millisecond,
		retryInterval:   5 * time.Millisecond,
	}
	g.generateID = sf.NextID
	return g
}

// newRetryableFailGenerator 创建一个 generateID 总是返回可重试错误的生成器。
// 用于测试 NewWithRetry 的超时和 context 取消路径。
func newRetryableFailGenerator(t *testing.T) *Generator {
	t.Helper()
	sf, err := sonyflake.New(sonyflake.Settings{
		MachineID: func() (int, error) { return 1, nil },
	})
	require.NoError(t, err)
	retryableErr := errors.New("clock backward (simulated)")
	g := &Generator{
		sf:              sf,
		maxWaitDuration: 50 * time.Millisecond,
		retryInterval:   5 * time.Millisecond,
	}
	g.generateID = func() (int64, error) {
		return 0, retryableErr
	}
	return g
}

// =============================================================================
// machineIDFromPrivateIP 测试
// =============================================================================

func TestMachineIDFromPrivateIP(t *testing.T) {
	// 环境依赖测试：结果取决于运行环境是否有私有 IP 地址。
	// 有私有 IP（大多数开发/CI 环境）→ 走成功路径
	// 无私有 IP（隔离容器、无网络）→ 走错误路径
	// 两条路径均有断言。纯函数 isPrivateIPv4 的确定性测试见 TestIsPrivateIPv4。
	id, err := machineIDFromPrivateIP()
	if err == nil {
		assert.NotZero(t, id)
	} else {
		assert.ErrorIs(t, err, ErrNoPrivateAddress)
	}
}

// =============================================================================
// privateIPv4 测试
// =============================================================================

func TestPrivateIPv4(t *testing.T) {
	// 环境依赖测试：结果取决于运行环境的网络配置。
	// 纯函数 isPrivateIPv4 的确定性测试见 TestIsPrivateIPv4 和 TestIsPrivateIPv4_EdgeCases。
	ip, err := privateIPv4()
	if err == nil {
		assert.True(t, ip.IsValid())
		assert.True(t, ip.Is4())
		assert.True(t, isPrivateIPv4(ip), "returned IP should be private")
	} else {
		assert.ErrorIs(t, err, ErrNoPrivateAddress)
	}
}

// =============================================================================
// ErrNoPrivateAddress 测试
// =============================================================================

func TestErrNoPrivateAddress(t *testing.T) {
	err := ErrNoPrivateAddress
	assert.Equal(t, "xid: no private IP address found", err.Error())

	// Test errors.Is
	assert.True(t, errors.Is(ErrNoPrivateAddress, ErrNoPrivateAddress))
}

// =============================================================================
// Decompose 未初始化测试
// =============================================================================

func TestDecompose_InvalidInput(t *testing.T) {
	tests := []struct {
		name string
		id   int64
	}{
		{"zero", 0},
		{"negative", -1},
		{"negative large", -999999},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Decompose(tt.id)
			assert.ErrorIs(t, err, ErrInvalidID)
		})
	}
}

func TestDecompose_PureFunction(t *testing.T) {
	// Decompose 是纯函数，不需要初始化
	// 即使全局生成器未初始化也能正常工作
	resetGlobal()

	parts, err := Decompose(12345)
	require.NoError(t, err)
	assert.Equal(t, int64(12345), parts.ID)

	// 验证位提取正确性：machine = id & 0xFFFF
	assert.Equal(t, int64(12345&0xFFFF), parts.Machine)
}

func TestDecompose_KnownValues(t *testing.T) {
	// 使用已知值验证位提取
	// 构造 ID: time=100, sequence=5, machine=42
	// id = (100 << 24) | (5 << 16) | 42
	id := int64(100)<<24 | int64(5)<<16 | int64(42)

	parts, err := Decompose(id)
	require.NoError(t, err)
	assert.Equal(t, id, parts.ID)
	assert.Equal(t, int64(100), parts.Time)
	assert.Equal(t, int64(5), parts.Sequence)
	assert.Equal(t, int64(42), parts.Machine)
}

// =============================================================================
// New 初始化失败测试
// =============================================================================

func TestNew_InitFailure(t *testing.T) {
	resetGlobal()

	// 使用一个会失败的 MachineID 函数初始化
	err := Init(WithMachineID(func() (uint16, error) {
		return 0, errors.New("machine ID error")
	}))
	require.Error(t, err)

	// 显式 Init 失败后，自动初始化不应覆盖用户意图
	_, err = New()
	assert.ErrorIs(t, err, ErrNotInitialized)

	// 重置以避免影响其他测试
	resetGlobal()
}

func TestNew_AutoInitFailure(t *testing.T) {
	resetGlobal()

	// 设置无效的 XID_MACHINE_ID 使 DefaultMachineID 返回错误，
	// 覆盖 ensureInitialized 中 NewGenerator() 失败的路径
	t.Setenv(EnvMachineID, "invalid")

	_, err := New()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid")

	resetGlobal()
}

// =============================================================================
// NewWithRetry 初始化失败测试
// =============================================================================

func TestNewWithRetry_InitFailure(t *testing.T) {
	resetGlobal()

	// 使用一个会失败的 MachineID 函数初始化
	err := Init(WithMachineID(func() (uint16, error) {
		return 0, errors.New("machine ID error")
	}))
	require.Error(t, err)

	_, err = NewWithRetry(context.Background())
	assert.ErrorIs(t, err, ErrNotInitialized)

	// 重置以避免影响其他测试
	resetGlobal()
}

// =============================================================================
// NewString 错误测试
// =============================================================================

func TestNewString_InitFailure(t *testing.T) {
	resetGlobal()

	// 使用一个会失败的 MachineID 函数初始化
	err := Init(WithMachineID(func() (uint16, error) {
		return 0, errors.New("machine ID error")
	}))
	require.Error(t, err)

	_, err = NewString()
	assert.ErrorIs(t, err, ErrNotInitialized)

	// 重置以避免影响其他测试
	resetGlobal()
}

// =============================================================================
// NewStringWithRetry 错误测试
// =============================================================================

func TestNewStringWithRetry_InitFailure(t *testing.T) {
	resetGlobal()

	// 使用一个会失败的 MachineID 函数初始化
	err := Init(WithMachineID(func() (uint16, error) {
		return 0, errors.New("machine ID error")
	}))
	require.Error(t, err)

	_, err = NewStringWithRetry(context.Background())
	assert.ErrorIs(t, err, ErrNotInitialized)

	// 重置以避免影响其他测试
	resetGlobal()
}

// =============================================================================
// MustNewWithRetry / MustNewStringWithRetry 测试
// =============================================================================

func TestMustNewWithRetry_Panic_OnInitFailure(t *testing.T) {
	resetGlobal()
	err := Init(WithMachineID(func() (uint16, error) {
		return 0, errors.New("machine ID error")
	}))
	require.Error(t, err)

	assert.Panics(t, func() { MustNewWithRetry() })
	resetGlobal()
}

func TestMustNewStringWithRetry_Panic_OnInitFailure(t *testing.T) {
	resetGlobal()
	err := Init(WithMachineID(func() (uint16, error) {
		return 0, errors.New("machine ID error")
	}))
	require.Error(t, err)

	assert.Panics(t, func() { MustNewStringWithRetry() })
	resetGlobal()
}

func TestGenerator_MustNewWithRetry(t *testing.T) {
	gen, err := NewGenerator()
	require.NoError(t, err)

	assert.NotPanics(t, func() {
		id := gen.MustNewWithRetry()
		assert.NotZero(t, id)
	})
}

func TestGenerator_MustNewStringWithRetry(t *testing.T) {
	gen, err := NewGenerator()
	require.NoError(t, err)

	assert.NotPanics(t, func() {
		s := gen.MustNewStringWithRetry()
		assert.NotEmpty(t, s)
	})
}

func TestGenerator_MustNewWithRetry_Panic(t *testing.T) {
	gen := newOverflowGenerator(t)
	assert.Panics(t, func() { gen.MustNewWithRetry() })
}

func TestGenerator_MustNewStringWithRetry_Panic(t *testing.T) {
	gen := newOverflowGenerator(t)
	assert.Panics(t, func() { gen.MustNewStringWithRetry() })
}

// =============================================================================
// Option 边界测试
// =============================================================================

func TestWithMaxWaitDuration_Zero(t *testing.T) {
	// 显式传入零值应被记录，供 NewGenerator 区分"未传入"与"显式传入 0"
	cfg := &options{}
	WithMaxWaitDuration(0)(cfg)
	assert.Equal(t, time.Duration(0), cfg.maxWaitDuration)
	assert.True(t, cfg.maxWaitSet)
}

func TestWithRetryInterval_Zero(t *testing.T) {
	// 显式传入零值应被记录，供 NewGenerator 区分"未传入"与"显式传入 0"
	cfg := &options{}
	WithRetryInterval(0)(cfg)
	assert.Equal(t, time.Duration(0), cfg.retryInterval)
	assert.True(t, cfg.retryIntervalSet)
}

func TestNewGenerator_NegativeMaxWaitDuration(t *testing.T) {
	_, err := NewGenerator(WithMaxWaitDuration(-1 * time.Second))
	assert.ErrorIs(t, err, ErrInvalidConfig)
	assert.Contains(t, err.Error(), "max wait duration must be non-negative")
}

func TestNewGenerator_NegativeRetryInterval(t *testing.T) {
	_, err := NewGenerator(WithRetryInterval(-1 * time.Millisecond))
	assert.ErrorIs(t, err, ErrInvalidConfig)
	assert.Contains(t, err.Error(), "retry interval must be non-negative")
}

// =============================================================================
// isPrivateIPv4 边界测试
// =============================================================================

func TestIsPrivateIPv4_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		ip       string
		expected bool
	}{
		{"172.16.0.0 boundary start", "172.16.0.0", true},
		{"172.31.255.255 boundary end", "172.31.255.255", true},
		{"172.15.255.255 below range", "172.15.255.255", false},
		{"172.32.0.0 above range", "172.32.0.0", false},
		{"IPv6 mapped", "::ffff:10.0.0.1", true},
		{"native IPv4", "10.0.0.1", true},
		{"zero value", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr := netip.Addr{}
			if tt.ip != "" {
				addr = netip.MustParseAddr(tt.ip)
			}
			result := isPrivateIPv4(addr)
			assert.Equal(t, tt.expected, result, "IP: %v", tt.ip)
		})
	}
}

// =============================================================================
// createGenerator nil option 测试
// =============================================================================

func TestCreateGenerator_NilOption(t *testing.T) {
	resetGlobal()

	// Test with nil option
	gen, err := NewGenerator(nil)
	require.NoError(t, err)
	assert.NotNil(t, gen)
}

// =============================================================================
// Parse 边界测试
// =============================================================================

func TestParse_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		errType error // 可选的错误类型检查
	}{
		{"valid base36 ID", "1a2b3c", false, nil},
		{"empty string", "", true, ErrInvalidID},
		{"valid numeric", "12345", false, nil},
		{"overflow", "99999999999999999999999999999", true, ErrInvalidID},
		{"zero", "0", true, ErrInvalidID},
		{"negative", "-1", true, ErrInvalidID},
		{"negative base36", "-abc", true, ErrInvalidID},
		// int64 正数范围恰好覆盖 Sonyflake 63 位有效位，strconv.ParseInt 溢出由 strconv 自身捕获
		{"max int64 base36", strconv.FormatInt((1<<63)-1, 36), false, nil},
		{"max time boundary", strconv.FormatInt(maxTimeValue<<(sequenceBits+machineBits)|0xFFFFFF, 36), false, nil},
		// 所有 Parse 错误统一包裹为 ErrInvalidID，调用方可稳定分类
		{"strconv overflow triggers error", "zzzzzzzzzzzzzz", true, ErrInvalidID},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errType != nil {
					assert.ErrorIs(t, err, tt.errType)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// =============================================================================
// Generator 实例方法覆盖测试
// =============================================================================

func TestGenerator_NewStringWithRetry(t *testing.T) {
	gen, err := NewGenerator()
	require.NoError(t, err)

	s, err := gen.NewStringWithRetry(context.Background())
	require.NoError(t, err)
	assert.NotEmpty(t, s)
}

func TestGenerator_NewWithRetry_ContextCanceled(t *testing.T) {
	gen, err := NewGenerator()
	require.NoError(t, err)

	// M2 修复：已取消的 context 应在入口处立即失败
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = gen.NewWithRetry(ctx)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestGenerator_NewString_Success(t *testing.T) {
	gen, err := NewGenerator()
	require.NoError(t, err)

	s, err := gen.NewString()
	require.NoError(t, err)
	assert.NotEmpty(t, s)
	assert.LessOrEqual(t, len(s), 13)
}

// =============================================================================
// Generator 溢出场景 — 覆盖 NextID 失败的错误路径
// =============================================================================

func TestGenerator_NewWithRetry_OverTimeLimit(t *testing.T) {
	gen := newOverflowGenerator(t)

	ctx := context.Background()
	_, err := gen.NewWithRetry(ctx)
	require.Error(t, err)
	// 不可恢复的溢出错误应立即返回 ErrOverTimeLimit，而非等待超时
	assert.ErrorIs(t, err, ErrOverTimeLimit)
	assert.NotErrorIs(t, err, ErrClockBackwardTimeout)
}

func TestGenerator_NewWithRetry_Timeout(t *testing.T) {
	gen := newRetryableFailGenerator(t)

	start := time.Now()
	_, err := gen.NewWithRetry(context.Background())
	elapsed := time.Since(start)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrClockBackwardTimeout)
	// 应在 maxWaitDuration (50ms) 附近超时
	assert.GreaterOrEqual(t, elapsed, 40*time.Millisecond)
	assert.Less(t, elapsed, 200*time.Millisecond)
}

func TestGenerator_NewWithRetry_Timeout_UsesRemainingBudget(t *testing.T) {
	gen := newRetryableFailGenerator(t)
	gen.maxWaitDuration = 20 * time.Millisecond
	gen.retryInterval = 300 * time.Millisecond

	start := time.Now()
	_, err := gen.NewWithRetry(context.Background())
	elapsed := time.Since(start)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrClockBackwardTimeout)
	assert.Less(t, elapsed, 150*time.Millisecond, "should cap wait by remaining budget instead of full retry interval")
}

func TestGenerator_NewWithRetry_ContextCancelDuringWait(t *testing.T) {
	gen := newRetryableFailGenerator(t)
	gen.maxWaitDuration = 5 * time.Second // 设置很长的超时，确保走到 context 取消分支

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	_, err := gen.NewWithRetry(ctx)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestGenerator_NewWithRetry_ContextCancel(t *testing.T) {
	gen, err := NewGenerator(
		WithMachineID(func() (uint16, error) { return 1, nil }),
		WithMaxWaitDuration(5*time.Second),
		WithRetryInterval(10*time.Millisecond),
	)
	require.NoError(t, err)

	// M2 修复：已取消的 context 在入口处快速失败
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = gen.NewWithRetry(ctx)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestGenerator_NewWithRetry_OverTimeLimit_Immediate(t *testing.T) {
	// 验证溢出错误不会等待，应立即返回
	gen := newOverflowGenerator(t)
	gen.maxWaitDuration = 5 * time.Second // 即使设置很长的等待时间

	start := time.Now()
	_, err := gen.NewWithRetry(context.Background())
	elapsed := time.Since(start)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrOverTimeLimit)
	// 应该立即返回，不应等待
	assert.Less(t, elapsed, 100*time.Millisecond, "should return immediately for non-retryable error")
}

func TestGenerator_NewString_Error(t *testing.T) {
	gen := newOverflowGenerator(t)

	_, err := gen.NewString()
	assert.Error(t, err)
}

func TestGenerator_NewStringWithRetry_Error(t *testing.T) {
	gen := newOverflowGenerator(t)

	_, err := gen.NewStringWithRetry(context.Background())
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrOverTimeLimit)
}

func TestGenerator_NewWithRetry_NilContext(t *testing.T) {
	gen, err := NewGenerator()
	require.NoError(t, err)

	ctxMap := map[string]context.Context{}
	_, err = gen.NewWithRetry(ctxMap["nil"])
	assert.ErrorIs(t, err, ErrNilContext)
}

// =============================================================================
// Generator nil/零值接收者测试
// =============================================================================

func TestGenerator_NilReceiver(t *testing.T) {
	var g *Generator

	_, err := g.New()
	assert.ErrorIs(t, err, ErrNilGenerator)

	_, err = g.NewWithRetry(context.Background())
	assert.ErrorIs(t, err, ErrNilGenerator)

	_, err = g.NewString()
	assert.ErrorIs(t, err, ErrNilGenerator)

	_, err = g.NewStringWithRetry(context.Background())
	assert.ErrorIs(t, err, ErrNilGenerator)
}

func TestGenerator_ZeroValue(t *testing.T) {
	g := &Generator{}

	_, err := g.New()
	assert.ErrorIs(t, err, ErrNilGenerator)

	_, err = g.NewWithRetry(context.Background())
	assert.ErrorIs(t, err, ErrNilGenerator)
}

// =============================================================================
// Generator.New 溢出错误映射一致性测试
// =============================================================================

func TestGenerator_New_OverTimeLimit(t *testing.T) {
	gen := newOverflowGenerator(t)

	_, err := gen.New()
	require.Error(t, err)
	// New() 应与 NewWithRetry() 一致地映射 ErrOverTimeLimit
	assert.ErrorIs(t, err, ErrOverTimeLimit)
}

func TestGenerator_New_NonOverflowError(t *testing.T) {
	// 覆盖 New() 中非溢出错误的透传路径（前向兼容预留）
	gen := newRetryableFailGenerator(t)

	_, err := gen.New()
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrOverTimeLimit)
}

// =============================================================================
// NewGenerator 包裹 sonyflake 配置错误测试
// =============================================================================

func TestNewGenerator_WrapsConfigErrors(t *testing.T) {
	// sonyflake.New 的错误（如 CheckMachineID 验证失败）应包裹为 ErrInvalidConfig
	_, err := NewGenerator(
		WithMachineID(func() (uint16, error) { return 100, nil }),
		WithCheckMachineID(func(id uint16) bool { return false }),
	)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidConfig)
}

// =============================================================================
// Fuzz 测试
// =============================================================================

func FuzzParse(f *testing.F) {
	// 添加种子语料库 - base36 格式
	f.Add("1a2b3c")
	f.Add("0")
	f.Add("")
	f.Add("abc")
	f.Add("zzzzzzzzzzzzzzz")

	resetGlobal()
	if err := Init(); err != nil {
		f.Fatalf("Init failed: %v", err)
	}

	f.Fuzz(func(t *testing.T, s string) {
		// Parse 不应该 panic
		_, _ = Parse(s) // fuzz 测试仅验证不 panic
	})
}
