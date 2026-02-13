package xid

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultMachineID_EnvVar(t *testing.T) {
	// 保存原始环境变量
	origMachineID := os.Getenv(EnvMachineID)
	origPodName := os.Getenv(EnvPodName)
	origHostname := os.Getenv(EnvHostname)
	defer func() {
		os.Setenv(EnvMachineID, origMachineID)
		os.Setenv(EnvPodName, origPodName)
		os.Setenv(EnvHostname, origHostname)
	}()

	// 清除其他环境变量，确保只测试 XID_MACHINE_ID
	os.Setenv(EnvPodName, "")
	os.Setenv(EnvHostname, "")

	t.Run("valid value", func(t *testing.T) {
		os.Setenv(EnvMachineID, "12345")
		id, err := DefaultMachineID()
		require.NoError(t, err)
		assert.Equal(t, uint16(12345), id)
	})

	t.Run("max value", func(t *testing.T) {
		os.Setenv(EnvMachineID, "65535")
		id, err := DefaultMachineID()
		require.NoError(t, err)
		assert.Equal(t, uint16(65535), id)
	})

	t.Run("zero value", func(t *testing.T) {
		os.Setenv(EnvMachineID, "0")
		id, err := DefaultMachineID()
		require.NoError(t, err)
		assert.Equal(t, uint16(0), id)
	})

	t.Run("empty falls through to next strategy", func(t *testing.T) {
		os.Setenv(EnvMachineID, "")
		// 不再测试 (_, false)，而是验证 fallback 能正常工作
		_, err := DefaultMachineID()
		assert.NoError(t, err)
	})

	t.Run("invalid - not a number returns error", func(t *testing.T) {
		os.Setenv(EnvMachineID, "abc")
		_, err := DefaultMachineID()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid")
		assert.Contains(t, err.Error(), "abc")
	})

	t.Run("invalid - out of range returns error", func(t *testing.T) {
		os.Setenv(EnvMachineID, "65536")
		_, err := DefaultMachineID()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid")
	})

	t.Run("invalid - negative returns error", func(t *testing.T) {
		os.Setenv(EnvMachineID, "-1")
		_, err := DefaultMachineID()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid")
	})
}

func TestMachineIDFromPodName(t *testing.T) {
	original := os.Getenv(EnvPodName)
	defer os.Setenv(EnvPodName, original)

	t.Run("with pod name", func(t *testing.T) {
		os.Setenv(EnvPodName, "my-app-deployment-abc123-xyz")
		id, ok := machineIDFromPodName()
		assert.True(t, ok)
		assert.NotZero(t, id)
	})

	t.Run("empty", func(t *testing.T) {
		os.Setenv(EnvPodName, "")
		_, ok := machineIDFromPodName()
		assert.False(t, ok)
	})

	t.Run("different pod names produce different IDs", func(t *testing.T) {
		os.Setenv(EnvPodName, "pod-a")
		idA, _ := machineIDFromPodName()

		os.Setenv(EnvPodName, "pod-b")
		idB, _ := machineIDFromPodName()

		assert.NotEqual(t, idA, idB)
	})
}

func TestMachineIDFromHostnameEnv(t *testing.T) {
	original := os.Getenv(EnvHostname)
	defer os.Setenv(EnvHostname, original)

	t.Run("with hostname", func(t *testing.T) {
		os.Setenv(EnvHostname, "worker-node-1")
		id, ok := machineIDFromHostnameEnv()
		assert.True(t, ok)
		assert.NotZero(t, id)
	})

	t.Run("empty", func(t *testing.T) {
		os.Setenv(EnvHostname, "")
		_, ok := machineIDFromHostnameEnv()
		assert.False(t, ok)
	})
}

func TestMachineIDFromOSHostname(t *testing.T) {
	// os.Hostname() 在大多数系统上都能成功
	id, ok := machineIDFromOSHostname()
	// 只有在系统没有 hostname 时才会失败
	if ok {
		assert.NotZero(t, id)
	}
}

func TestHashToMachineID(t *testing.T) {
	t.Run("deterministic", func(t *testing.T) {
		id1 := hashToMachineID("test-string")
		id2 := hashToMachineID("test-string")
		assert.Equal(t, id1, id2)
	})

	t.Run("different strings produce different IDs", func(t *testing.T) {
		id1 := hashToMachineID("string-a")
		id2 := hashToMachineID("string-b")
		assert.NotEqual(t, id1, id2)
	})

	t.Run("empty string", func(t *testing.T) {
		id := hashToMachineID("")
		// 空字符串的哈希是确定的
		assert.NotZero(t, id)
	})
}

func TestDefaultMachineID(t *testing.T) {
	// 保存原始环境变量
	origMachineID := os.Getenv(EnvMachineID)
	origPodName := os.Getenv(EnvPodName)
	origHostname := os.Getenv(EnvHostname)
	defer func() {
		os.Setenv(EnvMachineID, origMachineID)
		os.Setenv(EnvPodName, origPodName)
		os.Setenv(EnvHostname, origHostname)
	}()

	t.Run("priority - env var first", func(t *testing.T) {
		os.Setenv(EnvMachineID, "100")
		os.Setenv(EnvPodName, "my-pod")
		os.Setenv(EnvHostname, "my-host")

		id, err := DefaultMachineID()
		require.NoError(t, err)
		assert.Equal(t, uint16(100), id)
	})

	t.Run("priority - pod name second", func(t *testing.T) {
		os.Setenv(EnvMachineID, "")
		os.Setenv(EnvPodName, "my-pod")
		os.Setenv(EnvHostname, "my-host")

		id, err := DefaultMachineID()
		require.NoError(t, err)
		// 应该使用 pod name 的哈希
		expected := hashToMachineID("my-pod")
		assert.Equal(t, expected, id)
	})

	t.Run("priority - hostname env third", func(t *testing.T) {
		os.Setenv(EnvMachineID, "")
		os.Setenv(EnvPodName, "")
		os.Setenv(EnvHostname, "my-host")

		id, err := DefaultMachineID()
		require.NoError(t, err)
		// 应该使用 hostname 的哈希
		expected := hashToMachineID("my-host")
		assert.Equal(t, expected, id)
	})

	t.Run("fallback to hostname or IP", func(t *testing.T) {
		os.Setenv(EnvMachineID, "")
		os.Setenv(EnvPodName, "")
		os.Setenv(EnvHostname, "")

		id, err := DefaultMachineID()
		// 应该能通过 os.Hostname() 或私有 IP 获取到
		require.NoError(t, err)
		assert.NotZero(t, id)
	})
}

func TestIsPrivateIPv4(t *testing.T) {
	tests := []struct {
		name     string
		ip       []byte
		expected bool
	}{
		{"10.0.0.1", []byte{10, 0, 0, 1}, true},
		{"10.255.255.255", []byte{10, 255, 255, 255}, true},
		{"172.16.0.1", []byte{172, 16, 0, 1}, true},
		{"172.31.255.255", []byte{172, 31, 255, 255}, true},
		{"172.15.0.1", []byte{172, 15, 0, 1}, false},
		{"172.32.0.1", []byte{172, 32, 0, 1}, false},
		{"192.168.0.1", []byte{192, 168, 0, 1}, true},
		{"192.168.255.255", []byte{192, 168, 255, 255}, true},
		{"192.167.0.1", []byte{192, 167, 0, 1}, false},
		{"169.254.0.1 (link-local)", []byte{169, 254, 0, 1}, true},
		{"8.8.8.8 (public)", []byte{8, 8, 8, 8}, false},
		{"127.0.0.1 (loopback)", []byte{127, 0, 0, 1}, false},
		{"nil", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isPrivateIPv4(tt.ip)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDefaultMachineIDWithInit(t *testing.T) {
	resetGlobal()

	// 设置环境变量
	original := os.Getenv(EnvMachineID)
	defer os.Setenv(EnvMachineID, original)

	os.Setenv(EnvMachineID, "42")

	// 初始化（不传入自定义 MachineID，应该使用 DefaultMachineID）
	err := Init()
	require.NoError(t, err)

	// 生成 ID 并检查机器 ID 部分
	id, err := New()
	require.NoError(t, err)

	parts, err := Decompose(id)
	require.NoError(t, err)
	assert.Equal(t, int64(42), parts.Machine)
}
