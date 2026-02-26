package xid

import (
	"errors"
	"net"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultMachineID_EnvVar(t *testing.T) {
	// 清除其他环境变量，确保只测试 XID_MACHINE_ID
	t.Setenv(EnvPodName, "")
	t.Setenv(EnvHostname, "")

	t.Run("valid value", func(t *testing.T) {
		t.Setenv(EnvMachineID, "12345")
		id, err := DefaultMachineID()
		require.NoError(t, err)
		assert.Equal(t, uint16(12345), id)
	})

	t.Run("max value", func(t *testing.T) {
		t.Setenv(EnvMachineID, "65535")
		id, err := DefaultMachineID()
		require.NoError(t, err)
		assert.Equal(t, uint16(65535), id)
	})

	t.Run("zero value", func(t *testing.T) {
		t.Setenv(EnvMachineID, "0")
		id, err := DefaultMachineID()
		require.NoError(t, err)
		assert.Equal(t, uint16(0), id)
	})

	t.Run("empty falls through to next strategy", func(t *testing.T) {
		t.Setenv(EnvMachineID, "")
		// 不再测试 (_, false)，而是验证 fallback 能正常工作
		_, err := DefaultMachineID()
		assert.NoError(t, err)
	})

	t.Run("invalid - not a number returns error", func(t *testing.T) {
		t.Setenv(EnvMachineID, "abc")
		_, err := DefaultMachineID()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid")
		assert.Contains(t, err.Error(), "abc")
	})

	t.Run("invalid - out of range returns error", func(t *testing.T) {
		t.Setenv(EnvMachineID, "65536")
		_, err := DefaultMachineID()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid")
	})

	t.Run("invalid - negative returns error", func(t *testing.T) {
		t.Setenv(EnvMachineID, "-1")
		_, err := DefaultMachineID()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid")
	})
}

func TestMachineIDFromPodName(t *testing.T) {
	t.Run("with pod name", func(t *testing.T) {
		t.Setenv(EnvPodName, "my-app-deployment-abc123-xyz")
		id, ok := machineIDFromPodName()
		assert.True(t, ok)
		assert.NotZero(t, id)
	})

	t.Run("empty", func(t *testing.T) {
		t.Setenv(EnvPodName, "")
		_, ok := machineIDFromPodName()
		assert.False(t, ok)
	})

	t.Run("different pod names produce different IDs", func(t *testing.T) {
		t.Setenv(EnvPodName, "pod-a")
		idA, _ := machineIDFromPodName()

		t.Setenv(EnvPodName, "pod-b")
		idB, _ := machineIDFromPodName()

		assert.NotEqual(t, idA, idB)
	})
}

func TestMachineIDFromHostnameEnv(t *testing.T) {
	t.Run("with hostname", func(t *testing.T) {
		t.Setenv(EnvHostname, "worker-node-1")
		id, ok := machineIDFromHostnameEnv()
		assert.True(t, ok)
		assert.NotZero(t, id)
	})

	t.Run("empty", func(t *testing.T) {
		t.Setenv(EnvHostname, "")
		_, ok := machineIDFromHostnameEnv()
		assert.False(t, ok)
	})
}

func TestMachineIDFromOSHostname(t *testing.T) {
	// os.Hostname() 在大多数系统上都能成功。
	// 此测试依赖运行环境：有 hostname 时走成功路径，无 hostname 时验证返回 false。
	id, ok := machineIDFromOSHostname()
	if ok {
		assert.NotZero(t, id)
	} else {
		assert.Zero(t, id, "failed machineIDFromOSHostname should return zero ID")
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
	t.Run("priority - env var first", func(t *testing.T) {
		t.Setenv(EnvMachineID, "100")
		t.Setenv(EnvPodName, "my-pod")
		t.Setenv(EnvHostname, "my-host")

		id, err := DefaultMachineID()
		require.NoError(t, err)
		assert.Equal(t, uint16(100), id)
	})

	t.Run("priority - pod name second", func(t *testing.T) {
		t.Setenv(EnvMachineID, "")
		t.Setenv(EnvPodName, "my-pod")
		t.Setenv(EnvHostname, "my-host")

		id, err := DefaultMachineID()
		require.NoError(t, err)
		// 应该使用 pod name 的哈希
		expected := hashToMachineID("my-pod")
		assert.Equal(t, expected, id)
	})

	t.Run("priority - hostname env third", func(t *testing.T) {
		t.Setenv(EnvMachineID, "")
		t.Setenv(EnvPodName, "")
		t.Setenv(EnvHostname, "my-host")

		id, err := DefaultMachineID()
		require.NoError(t, err)
		// 应该使用 hostname 的哈希
		expected := hashToMachineID("my-host")
		assert.Equal(t, expected, id)
	})

	t.Run("fallback to hostname or IP", func(t *testing.T) {
		t.Setenv(EnvMachineID, "")
		t.Setenv(EnvPodName, "")
		t.Setenv(EnvHostname, "")

		id, err := DefaultMachineID()
		// 应该能通过 os.Hostname() 或私有 IP 获取到
		require.NoError(t, err)
		assert.NotZero(t, id)
	})
}

func TestIsPrivateIPv4(t *testing.T) {
	tests := []struct {
		name     string
		ip       string
		expected bool
	}{
		{"10.0.0.1", "10.0.0.1", true},
		{"10.255.255.255", "10.255.255.255", true},
		{"172.16.0.1", "172.16.0.1", true},
		{"172.31.255.255", "172.31.255.255", true},
		{"172.15.0.1", "172.15.0.1", false},
		{"172.32.0.1", "172.32.0.1", false},
		{"192.168.0.1", "192.168.0.1", true},
		{"192.168.255.255", "192.168.255.255", true},
		{"192.167.0.1", "192.167.0.1", false},
		{"169.254.0.1 (link-local)", "169.254.0.1", true},
		{"8.8.8.8 (public)", "8.8.8.8", false},
		{"127.0.0.1 (loopback)", "127.0.0.1", false},
		{"zero value", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr := netip.Addr{}
			if tt.ip != "" {
				addr = netip.MustParseAddr(tt.ip)
			}
			result := isPrivateIPv4(addr)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDefaultMachineIDWithInit(t *testing.T) {
	resetGlobal()

	t.Setenv(EnvMachineID, "42")

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

// =============================================================================
// 确定性注入测试（FG-M2/M3/M4 修复）
// 利用 osHostname/netInterfaceAddrs 注入点覆盖 machine.go 错误分支
// =============================================================================

func TestMachineIDFromOSHostname_Deterministic(t *testing.T) {
	t.Run("osHostname returns error", func(t *testing.T) {
		orig := osHostname
		osHostname = func() (string, error) {
			return "", errors.New("hostname unavailable")
		}
		t.Cleanup(func() { osHostname = orig })

		id, ok := machineIDFromOSHostname()
		assert.False(t, ok)
		assert.Zero(t, id)
	})

	t.Run("osHostname returns empty string", func(t *testing.T) {
		orig := osHostname
		osHostname = func() (string, error) {
			return "", nil
		}
		t.Cleanup(func() { osHostname = orig })

		id, ok := machineIDFromOSHostname()
		assert.False(t, ok)
		assert.Zero(t, id)
	})

	t.Run("osHostname returns valid hostname", func(t *testing.T) {
		orig := osHostname
		osHostname = func() (string, error) {
			return "test-host-injected", nil
		}
		t.Cleanup(func() { osHostname = orig })

		id, ok := machineIDFromOSHostname()
		assert.True(t, ok)
		assert.Equal(t, hashToMachineID("test-host-injected"), id)
	})
}

func TestPrivateIPv4_Deterministic(t *testing.T) {
	t.Run("netInterfaceAddrs returns error", func(t *testing.T) {
		orig := netInterfaceAddrs
		netInterfaceAddrs = func() ([]net.Addr, error) {
			return nil, errors.New("network unavailable")
		}
		t.Cleanup(func() { netInterfaceAddrs = orig })

		_, err := privateIPv4()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "network unavailable")
	})

	t.Run("no addresses", func(t *testing.T) {
		orig := netInterfaceAddrs
		netInterfaceAddrs = func() ([]net.Addr, error) {
			return []net.Addr{}, nil
		}
		t.Cleanup(func() { netInterfaceAddrs = orig })

		_, err := privateIPv4()
		assert.ErrorIs(t, err, ErrNoPrivateAddress)
	})

	t.Run("only loopback", func(t *testing.T) {
		orig := netInterfaceAddrs
		netInterfaceAddrs = func() ([]net.Addr, error) {
			return []net.Addr{
				&net.IPNet{IP: net.ParseIP("127.0.0.1"), Mask: net.CIDRMask(8, 32)},
			}, nil
		}
		t.Cleanup(func() { netInterfaceAddrs = orig })

		_, err := privateIPv4()
		assert.ErrorIs(t, err, ErrNoPrivateAddress)
	})

	t.Run("only public IP", func(t *testing.T) {
		orig := netInterfaceAddrs
		netInterfaceAddrs = func() ([]net.Addr, error) {
			return []net.Addr{
				&net.IPNet{IP: net.ParseIP("8.8.8.8"), Mask: net.CIDRMask(24, 32)},
			}, nil
		}
		t.Cleanup(func() { netInterfaceAddrs = orig })

		_, err := privateIPv4()
		assert.ErrorIs(t, err, ErrNoPrivateAddress)
	})

	t.Run("non-IPNet type skipped", func(t *testing.T) {
		orig := netInterfaceAddrs
		netInterfaceAddrs = func() ([]net.Addr, error) {
			// net.TCPAddr 实现了 net.Addr 但不是 *net.IPNet
			return []net.Addr{
				&net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 80},
			}, nil
		}
		t.Cleanup(func() { netInterfaceAddrs = orig })

		_, err := privateIPv4()
		assert.ErrorIs(t, err, ErrNoPrivateAddress)
	})

	t.Run("IPv6 only skipped", func(t *testing.T) {
		orig := netInterfaceAddrs
		netInterfaceAddrs = func() ([]net.Addr, error) {
			return []net.Addr{
				&net.IPNet{IP: net.ParseIP("2001:db8::1"), Mask: net.CIDRMask(64, 128)},
			}, nil
		}
		t.Cleanup(func() { netInterfaceAddrs = orig })

		_, err := privateIPv4()
		assert.ErrorIs(t, err, ErrNoPrivateAddress)
	})

	t.Run("private IP found", func(t *testing.T) {
		orig := netInterfaceAddrs
		netInterfaceAddrs = func() ([]net.Addr, error) {
			return []net.Addr{
				&net.IPNet{IP: net.ParseIP("127.0.0.1"), Mask: net.CIDRMask(8, 32)},
				&net.IPNet{IP: net.ParseIP("10.1.2.3"), Mask: net.CIDRMask(24, 32)},
			}, nil
		}
		t.Cleanup(func() { netInterfaceAddrs = orig })

		ip, err := privateIPv4()
		require.NoError(t, err)
		assert.Equal(t, netip.MustParseAddr("10.1.2.3"), ip)
	})
}

func TestMachineIDFromPrivateIP_Deterministic(t *testing.T) {
	t.Run("error from privateIPv4", func(t *testing.T) {
		orig := netInterfaceAddrs
		netInterfaceAddrs = func() ([]net.Addr, error) {
			return nil, errors.New("interface error")
		}
		t.Cleanup(func() { netInterfaceAddrs = orig })

		_, err := machineIDFromPrivateIP()
		require.Error(t, err)
	})

	t.Run("success extracts low 16 bits", func(t *testing.T) {
		orig := netInterfaceAddrs
		netInterfaceAddrs = func() ([]net.Addr, error) {
			return []net.Addr{
				&net.IPNet{IP: net.ParseIP("10.0.1.2"), Mask: net.CIDRMask(24, 32)},
			}, nil
		}
		t.Cleanup(func() { netInterfaceAddrs = orig })

		id, err := machineIDFromPrivateIP()
		require.NoError(t, err)
		// 10.0.1.2 → low 16 bits = (1 << 8) + 2 = 258
		assert.Equal(t, uint16(258), id)
	})
}

func TestDefaultMachineID_AllStrategiesExhausted(t *testing.T) {
	// 清除所有环境变量（策略 1-3）
	t.Setenv(EnvMachineID, "")
	t.Setenv(EnvPodName, "")
	t.Setenv(EnvHostname, "")

	// 注入 osHostname 失败（策略 4）
	origHostname := osHostname
	osHostname = func() (string, error) {
		return "", errors.New("hostname unavailable")
	}
	t.Cleanup(func() { osHostname = origHostname })

	// 注入 netInterfaceAddrs 失败（策略 5）
	origAddrs := netInterfaceAddrs
	netInterfaceAddrs = func() ([]net.Addr, error) {
		return nil, errors.New("no network interfaces")
	}
	t.Cleanup(func() { netInterfaceAddrs = origAddrs })

	_, err := DefaultMachineID()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "all machine ID strategies exhausted")
	// 验证错误链保留底层原因
	assert.Contains(t, err.Error(), "no network interfaces")
}

func TestDefaultMachineID_FallbackToOSHostname(t *testing.T) {
	// 清除环境变量（策略 1-3 均跳过）
	t.Setenv(EnvMachineID, "")
	t.Setenv(EnvPodName, "")
	t.Setenv(EnvHostname, "")

	// 注入 osHostname 成功（策略 4 命中）
	origHostname := osHostname
	osHostname = func() (string, error) {
		return "injected-hostname", nil
	}
	t.Cleanup(func() { osHostname = origHostname })

	id, err := DefaultMachineID()
	require.NoError(t, err)
	assert.Equal(t, hashToMachineID("injected-hostname"), id)
}

func TestDefaultMachineID_FallbackToPrivateIP(t *testing.T) {
	// 清除环境变量（策略 1-3 均跳过）
	t.Setenv(EnvMachineID, "")
	t.Setenv(EnvPodName, "")
	t.Setenv(EnvHostname, "")

	// 注入 osHostname 失败（策略 4 跳过）
	origHostname := osHostname
	osHostname = func() (string, error) {
		return "", errors.New("hostname unavailable")
	}
	t.Cleanup(func() { osHostname = origHostname })

	// 注入 netInterfaceAddrs 返回私有 IP（策略 5 命中）
	origAddrs := netInterfaceAddrs
	netInterfaceAddrs = func() ([]net.Addr, error) {
		return []net.Addr{
			&net.IPNet{IP: net.ParseIP("192.168.1.100"), Mask: net.CIDRMask(24, 32)},
		}, nil
	}
	t.Cleanup(func() { netInterfaceAddrs = origAddrs })

	id, err := DefaultMachineID()
	require.NoError(t, err)
	// 192.168.1.100 → low 16 bits = (1 << 8) + 100 = 356
	assert.Equal(t, uint16(356), id)
}

func TestDefaultMachineID_NoPrivateAddress(t *testing.T) {
	// 清除环境变量（策略 1-3 均跳过）
	t.Setenv(EnvMachineID, "")
	t.Setenv(EnvPodName, "")
	t.Setenv(EnvHostname, "")

	// 注入 osHostname 失败（策略 4 跳过）
	origHostname := osHostname
	osHostname = func() (string, error) {
		return "", errors.New("hostname unavailable")
	}
	t.Cleanup(func() { osHostname = origHostname })

	// 注入 netInterfaceAddrs 返回空（策略 5 返回 ErrNoPrivateAddress）
	origAddrs := netInterfaceAddrs
	netInterfaceAddrs = func() ([]net.Addr, error) {
		return []net.Addr{}, nil
	}
	t.Cleanup(func() { netInterfaceAddrs = origAddrs })

	_, err := DefaultMachineID()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "all machine ID strategies exhausted")
	// 验证 ErrNoPrivateAddress 在错误链中可解包
	assert.ErrorIs(t, err, ErrNoPrivateAddress)
}
