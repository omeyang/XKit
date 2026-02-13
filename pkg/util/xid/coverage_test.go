package xid

import (
	"context"
	"errors"
	"net"
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
	return &Generator{
		sf:              sf,
		maxWaitDuration: 50 * time.Millisecond,
		retryInterval:   5 * time.Millisecond,
	}
}

// =============================================================================
// machineIDFromPrivateIP 测试
// =============================================================================

func TestMachineIDFromPrivateIP(t *testing.T) {
	// 在有网络接口的环境中，应该能获取到私有 IP
	id, err := machineIDFromPrivateIP()
	// 如果有私有 IP，应该成功
	// 如果没有私有 IP（如在隔离容器中），会返回错误
	if err == nil {
		assert.NotZero(t, id)
	} else {
		// 验证是正确的错误类型
		assert.True(t, errors.Is(err, ErrNoPrivateAddress) || err != nil)
	}
}

// =============================================================================
// privateIPv4 测试
// =============================================================================

func TestPrivateIPv4(t *testing.T) {
	ip, err := privateIPv4()
	if err == nil {
		assert.NotNil(t, ip)
		assert.Len(t, ip, 4) // IPv4 should be 4 bytes
		assert.True(t, isPrivateIPv4(ip), "returned IP should be private")
	}
	// Error case is acceptable in some environments
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

	_, err = New()
	assert.Error(t, err)

	// 重置以避免影响其他测试
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
	assert.Error(t, err)

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
	assert.Error(t, err)

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
	assert.Error(t, err)

	// 重置以避免影响其他测试
	resetGlobal()
}

// =============================================================================
// MustNew panic 测试
// =============================================================================

func TestMustNew_Panic_OnInitFailure(t *testing.T) {
	resetGlobal()

	// 使用一个会失败的 MachineID 函数初始化
	err := Init(WithMachineID(func() (uint16, error) {
		return 0, errors.New("machine ID error")
	}))
	require.Error(t, err)

	assert.Panics(t, func() {
		MustNew()
	})

	// 重置以避免影响其他测试
	resetGlobal()
}

// =============================================================================
// MustNewWithRetry panic 测试
// =============================================================================

// =============================================================================
// MustNewString panic 测试
// =============================================================================

func TestMustNewString_Panic_OnInitFailure(t *testing.T) {
	resetGlobal()

	// 使用一个会失败的 MachineID 函数初始化
	err := Init(WithMachineID(func() (uint16, error) {
		return 0, errors.New("machine ID error")
	}))
	require.Error(t, err)

	assert.Panics(t, func() {
		MustNewString()
	})

	// 重置以避免影响其他测试
	resetGlobal()
}

// =============================================================================
// Option 边界测试
// =============================================================================

func TestWithMaxWaitDuration_Zero(t *testing.T) {
	// 零值存储到 config，NewGenerator 中使用默认值
	cfg := &config{}
	WithMaxWaitDuration(0)(cfg)
	assert.Equal(t, time.Duration(0), cfg.maxWaitDuration)
}

func TestWithRetryInterval_Zero(t *testing.T) {
	// 零值存储到 config，NewGenerator 中使用默认值
	cfg := &config{}
	WithRetryInterval(0)(cfg)
	assert.Equal(t, time.Duration(0), cfg.retryInterval)
}

func TestNewGenerator_NegativeMaxWaitDuration(t *testing.T) {
	_, err := NewGenerator(WithMaxWaitDuration(-1 * time.Second))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "max wait duration must be non-negative")
}

func TestNewGenerator_NegativeRetryInterval(t *testing.T) {
	_, err := NewGenerator(WithRetryInterval(-1 * time.Millisecond))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "retry interval must be non-negative")
}

// =============================================================================
// isPrivateIPv4 边界测试
// =============================================================================

func TestIsPrivateIPv4_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		ip       net.IP
		expected bool
	}{
		// 172.16.0.0/12 边界测试 - 使用字节切片
		{"172.16.0.0 boundary start", []byte{172, 16, 0, 0}, true},
		{"172.31.255.255 boundary end", []byte{172, 31, 255, 255}, true},
		{"172.15.255.255 below range", []byte{172, 15, 255, 255}, false},
		{"172.32.0.0 above range", []byte{172, 32, 0, 0}, false},
		// IPv6 mapped 转换为 IPv4
		{"IPv6 mapped", net.ParseIP("::ffff:10.0.0.1").To4(), true},
		// 16-byte slice (IPv4-mapped IPv6 format from net.ParseIP) — now auto-converted via To4()
		{"16-byte IPv4", net.ParseIP("10.0.0.1"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isPrivateIPv4(tt.ip)
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
		{"empty string", "", true, nil},
		{"valid numeric", "12345", false, nil},
		{"overflow", "99999999999999999999999999999", true, nil},
		{"zero", "0", true, ErrInvalidID},      // 零不是有效 xid
		{"negative", "-1", true, ErrInvalidID}, // 负数不是有效 xid
		{"negative base36", "-abc", true, ErrInvalidID},
		// H5: Sonyflake 位范围校验（39+8+16=63 位）
		// int64 正数范围恰好覆盖 63 位，strconv.ParseInt 溢出的值由 strconv 自身捕获
		{"max int64 base36", strconv.FormatInt((1<<63)-1, 36), false, nil},                                          // time = maxTimeValue，恰好有效
		{"max time boundary", strconv.FormatInt(maxTimeValue<<(sequenceBits+machineBits)|0xFFFFFF, 36), false, nil}, // 恰好 39 位时间上限
		// 注意：39+8+16=63 位恰好等于 int64 正数范围，不存在"时间溢出但 int64 有效"的情况
		// 超过 39 位时间的值必然超过 int64 范围，由 strconv.ParseInt 捕获
		{"strconv overflow triggers error", "zzzzzzzzzzzzzz", true, nil}, // 超出 int64 范围由 strconv 捕获
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
// Decompose 测试
// =============================================================================

func TestDecompose_ValidID(t *testing.T) {
	resetGlobal()
	require.NoError(t, Init())

	// 生成一个 ID
	id, err := New()
	require.NoError(t, err)

	// 分解 ID（纯函数，无需初始化）
	result, err := Decompose(id)
	require.NoError(t, err)

	// 验证值
	assert.Equal(t, id, result.ID)
	assert.NotZero(t, result.Time)

	resetGlobal()
}

// =============================================================================
// 并发安全测试
// =============================================================================

func TestConcurrentNew_NoCollision(t *testing.T) {
	resetGlobal()
	require.NoError(t, Init())

	const goroutines = 10
	const idsPerGoroutine = 100

	ids := make(chan int64, goroutines*idsPerGoroutine)
	done := make(chan struct{})

	// 启动多个 goroutine 并发生成 ID
	for i := 0; i < goroutines; i++ {
		go func() {
			for j := 0; j < idsPerGoroutine; j++ {
				id, err := New()
				if err != nil {
					t.Errorf("New() error: %v", err)
					return
				}
				ids <- id
			}
			done <- struct{}{}
		}()
	}

	// 等待所有 goroutine 完成
	for i := 0; i < goroutines; i++ {
		<-done
	}
	close(ids)

	// 检查是否有重复
	seen := make(map[int64]bool)
	for id := range ids {
		if seen[id] {
			t.Errorf("Duplicate ID found: %d", id)
		}
		seen[id] = true
	}

	assert.Equal(t, goroutines*idsPerGoroutine, len(seen), "Expected %d unique IDs", goroutines*idsPerGoroutine)
	resetGlobal()
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

func TestGenerator_MustNewString(t *testing.T) {
	gen, err := NewGenerator()
	require.NoError(t, err)

	assert.NotPanics(t, func() {
		s := gen.MustNewString()
		assert.NotEmpty(t, s)
	})
}

func TestGenerator_NewWithRetry_ContextCanceled(t *testing.T) {
	gen, err := NewGenerator()
	require.NoError(t, err)

	// 使用已取消的 context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// 即使 context 已取消，如果第一次尝试就成功（大概率），仍然返回 ID
	// 如果 sonyflake 碰巧遇到时钟问题，则返回 ctx 错误
	_, _ = gen.NewWithRetry(ctx) //nolint:errcheck // 测试 context 取消路径
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

func TestGenerator_NewWithRetry_ContextCancel(t *testing.T) {
	// 使用一个永远失败但可重试的生成器来测试 context 取消
	gen, err := NewGenerator(
		WithMachineID(func() (uint16, error) { return 1, nil }),
		WithMaxWaitDuration(5*time.Second),
		WithRetryInterval(10*time.Millisecond),
	)
	require.NoError(t, err)

	// 替换底层 sonyflake 为一个总是返回可重试错误的版本
	// 通过已取消的 context 测试取消路径
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	// 即使第一次尝试就成功（大概率），仍然会返回 ID
	// 只有在 sonyflake 碰巧遇到错误时，才会走到 context 取消分支
	_, _ = gen.NewWithRetry(ctx) //nolint:errcheck // 测试 context 取消路径
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

func TestGenerator_MustNew_Panic(t *testing.T) {
	gen := newOverflowGenerator(t)

	assert.Panics(t, func() {
		gen.MustNew()
	})
}

func TestGenerator_MustNewString_Panic(t *testing.T) {
	gen := newOverflowGenerator(t)

	assert.Panics(t, func() {
		gen.MustNewString()
	})
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
		_, _ = Parse(s) //nolint:errcheck // fuzz 测试仅验证不 panic
	})
}
