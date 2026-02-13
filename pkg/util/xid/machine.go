package xid

import (
	"errors"
	"fmt"
	"hash/fnv"
	"net"
	"os"
	"strconv"
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
// 这种多层回退策略确保在各种环境下都能获取到唯一的机器 ID：
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
	if id, ok := machineIDFromOSHostname(); ok {
		return id, nil
	}

	// 策略 5：从私有 IP 地址
	return machineIDFromPrivateIP()
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

// machineIDFromOSHostname 从 os.Hostname() 的哈希值获取机器 ID
func machineIDFromOSHostname() (uint16, bool) {
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		return 0, false
	}

	return hashToMachineID(hostname), true
}

// machineIDFromPrivateIP 从私有 IP 地址的低 16 位获取机器 ID
// 这是 sonyflake 的默认方式
func machineIDFromPrivateIP() (uint16, error) {
	ip, err := privateIPv4()
	if err != nil {
		return 0, err
	}
	return uint16(ip[2])<<8 + uint16(ip[3]), nil
}

// hashToMachineID 将字符串哈希为 16 位机器 ID
// 使用 FNV-1a 哈希算法，具有良好的分布性
func hashToMachineID(s string) uint16 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s)) // hash.Hash.Write never returns error
	// 取低 16 位，这是有意的截断以适配机器 ID 范围
	return uint16(h.Sum32()) //nolint:gosec // intentional truncation to 16-bit machine ID
}

// =============================================================================
// 私有 IP 获取（参考 sonyflake 实现）
// =============================================================================

// ErrNoPrivateAddress 无法找到私有 IP 地址
var ErrNoPrivateAddress = errors.New("xid: no private IP address found")

// privateIPv4 获取私有 IPv4 地址
func privateIPv4() (net.IP, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}

	for _, addr := range addrs {
		ipnet, ok := addr.(*net.IPNet)
		if !ok || ipnet.IP.IsLoopback() {
			continue
		}

		ip := ipnet.IP.To4()
		if isPrivateIPv4(ip) {
			return ip, nil
		}
	}

	return nil, ErrNoPrivateAddress
}

// isPrivateIPv4 判断是否为私有 IPv4 地址
// 包括 RFC1918 私有地址和 RFC3927 链路本地地址
// 自动将 16 字节 IPv4-mapped IPv6 转换为 4 字节 IPv4 格式
func isPrivateIPv4(ip net.IP) bool {
	ip = ip.To4()
	if ip == nil {
		return false
	}
	// 10.0.0.0/8
	// 172.16.0.0/12
	// 192.168.0.0/16
	// 169.254.0.0/16 (link-local)
	return ip[0] == 10 ||
		(ip[0] == 172 && ip[1] >= 16 && ip[1] < 32) ||
		(ip[0] == 192 && ip[1] == 168) ||
		(ip[0] == 169 && ip[1] == 254)
}
