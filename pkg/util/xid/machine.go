package xid

import (
	"errors"
	"fmt"
	"hash/fnv"
	"net"
	"net/netip"
	"os"
	"strconv"
)

// 测试注入点：允许测试替换系统调用以覆盖所有错误分支。
// 参照 xproc 包的 osExecutable 模式（FG-M2）。
var (
	osHostname        = os.Hostname        // machineIDFromOSHostname
	netInterfaceAddrs = net.InterfaceAddrs // privateIPv4
)

// =============================================================================
// 环境变量
// =============================================================================

const (
	// EnvMachineID 直接指定机器 ID 的环境变量（0-65535）
	EnvMachineID = "XID_MACHINE_ID"

	// EnvPodName K8s Pod 名称环境变量（通过 Downward API 注入）
	EnvPodName = "POD_NAME"

	// EnvHostname 主机名环境变量（某些环境会设置）
	EnvHostname = "HOSTNAME"
)

// =============================================================================
// 机器 ID 获取策略
// =============================================================================

// DefaultMachineID 获取机器 ID，按以下优先级尝试：
//
//  1. XID_MACHINE_ID 环境变量（直接指定数字 0-65535）
//  2. POD_NAME 环境变量的哈希值（K8s Downward API）
//  3. HOSTNAME 环境变量的哈希值
//  4. os.Hostname() 的哈希值
//  5. 私有 IP 地址的低 16 位（sonyflake 默认方式）
//
// 这种多层回退策略确保在各种环境下都能获取到可用机器 ID，
// 但仅 XID_MACHINE_ID 的显式分配能提供可控唯一性：
//   - 在线/离线 K8s 集群
//   - HostNetwork 模式
//   - 虚拟机、物理机、容器
//
// 注意：使用哈希方式（策略 2-4）时存在碰撞风险。根据生日悖论：
//   - 50 节点约 1.9% 碰撞概率
//   - 100 节点约 7.3% 碰撞概率
//   - 200 节点约 26% 碰撞概率
//
// 对于大规模部署（>50 节点）或高一致性要求的场景，
// 强烈建议通过 XID_MACHINE_ID 环境变量显式分配唯一 ID。
func DefaultMachineID() (uint16, error) {
	// 策略 1：直接从环境变量读取
	if s := os.Getenv(EnvMachineID); s != "" {
		id, err := strconv.ParseUint(s, 10, 16)
		if err != nil {
			return 0, fmt.Errorf("xid: invalid %s value %q: %w", EnvMachineID, s, err)
		}
		return uint16(id), nil
	}

	// 策略 2：从 Pod 名称哈希
	if id, ok := machineIDFromPodName(); ok {
		return id, nil
	}

	// 策略 3：从 HOSTNAME 环境变量哈希
	if id, ok := machineIDFromHostnameEnv(); ok {
		return id, nil
	}

	// 策略 4：从 os.Hostname() 哈希
	// 设计决策: 策略 2-3 使用 (id, bool) 因为失败原因总是"环境变量未设置"。
	// 策略 4 使用 (id, error)，因为 os.Hostname() 可能产生有诊断价值的系统错误
	// （如容器内内核限制），在全链路失败时聚合到最终错误中帮助排障（FG-S1）。
	hostnameID, hostnameErr := machineIDFromOSHostname()
	if hostnameErr == nil {
		return hostnameID, nil
	}

	// 策略 5：从私有 IP 地址
	id, err := machineIDFromPrivateIP()
	if err != nil {
		return 0, fmt.Errorf("xid: all machine ID strategies exhausted (os-hostname: %v): %w", hostnameErr, err)
	}
	return id, nil
}

// machineIDFromPodName 从 POD_NAME 环境变量的哈希值获取机器 ID
// K8s 中可通过 Downward API 注入：
//
//	env:
//	  - name: POD_NAME
//	    valueFrom:
//	      fieldRef:
//	        fieldPath: metadata.name
func machineIDFromPodName() (uint16, bool) {
	podName := os.Getenv(EnvPodName)
	if podName == "" {
		return 0, false
	}

	return hashToMachineID(podName), true
}

// machineIDFromHostnameEnv 从 HOSTNAME 环境变量的哈希值获取机器 ID
func machineIDFromHostnameEnv() (uint16, bool) {
	hostname := os.Getenv(EnvHostname)
	if hostname == "" {
		return 0, false
	}

	return hashToMachineID(hostname), true
}

// machineIDFromOSHostname 从 os.Hostname() 的哈希值获取机器 ID。
//
// 设计决策: 与策略 2-3 的 (id, bool) 签名不同，策略 4 返回 error，
// 因为 os.Hostname() 可能产生有诊断价值的系统错误（如容器内内核限制），
// 在全链路失败时需要聚合到最终错误信息中帮助排障。
func machineIDFromOSHostname() (uint16, error) {
	hostname, err := osHostname()
	if err != nil {
		return 0, err
	}
	if hostname == "" {
		return 0, errors.New("os.Hostname returned empty string")
	}

	return hashToMachineID(hostname), nil
}

// machineIDFromPrivateIP 从私有 IP 地址的低 16 位获取机器 ID。
// 这是 sonyflake 的默认方式。
//
// 注意：net.InterfaceAddrs 的枚举顺序依赖于操作系统，多网卡环境下
// 重启后可能选到不同的 IP，导致 machine ID 变化。
// 生产环境建议通过 XID_MACHINE_ID 环境变量显式分配。
func machineIDFromPrivateIP() (uint16, error) {
	ip, err := privateIPv4()
	if err != nil {
		return 0, err
	}
	b := ip.As4()
	return uint16(b[2])<<8 + uint16(b[3]), nil
}

// hashToMachineID 将字符串哈希为 16 位机器 ID。
// 使用 FNV-1a 哈希算法，通过 XOR 折叠将 32 位哈希压缩为 16 位，
// 比简单截断（仅取低 16 位）更充分利用完整哈希值，分布性更优。
func hashToMachineID(s string) uint16 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s)) // hash.Hash.Write never returns error
	// XOR 折叠：通过字节级操作避免 uint32→uint16 直接转换
	b := h.Sum(nil) // FNV-32 返回 4 字节大端序
	hi := uint16(b[0])<<8 | uint16(b[1])
	lo := uint16(b[2])<<8 | uint16(b[3])
	return hi ^ lo
}

// =============================================================================
// 私有 IP 获取（参考 sonyflake 实现）
// =============================================================================

// privateIPv4 获取私有 IPv4 地址。
func privateIPv4() (netip.Addr, error) {
	addrs, err := netInterfaceAddrs()
	if err != nil {
		return netip.Addr{}, err
	}

	for _, addr := range addrs {
		ipnet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}

		ip, ok := netip.AddrFromSlice(ipnet.IP)
		if !ok {
			continue
		}
		ip = ip.Unmap()
		if ip.IsLoopback() || !ip.Is4() {
			continue
		}

		if isPrivateIPv4(ip) {
			return ip, nil
		}
	}

	return netip.Addr{}, ErrNoPrivateAddress
}

// isPrivateIPv4 判断是否为私有 IPv4 地址
// 包括 RFC1918 私有地址和 RFC3927 链路本地地址
//
// 设计决策: Unmap() 是为独立调用场景的防御性处理——当调用方已调用过 Unmap
// 时（如 privateIPv4），此处的 Unmap 是幂等无开销的冗余调用（FG-L1）。
func isPrivateIPv4(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.Is4() {
		return false
	}
	return ip.IsPrivate() || ip.IsLinkLocalUnicast()
}
