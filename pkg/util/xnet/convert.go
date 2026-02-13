package xnet

import (
	"encoding/binary"
	"fmt"
	"math/big"
	"net/netip"
)

// AddrFromUint32 从 IPv4 的 uint32 表示创建 [netip.Addr]。
// 使用网络字节序（大端）。
func AddrFromUint32(v uint32) netip.Addr {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], v)
	return netip.AddrFrom4(b)
}

// AddrToUint32 将 IPv4 地址转换为 uint32（网络字节序）。
// 非 IPv4 地址返回 (0, false)。
func AddrToUint32(addr netip.Addr) (uint32, bool) {
	if !addr.Is4() && !addr.Is4In6() {
		return 0, false
	}
	b := addr.Unmap().As4()
	return binary.BigEndian.Uint32(b[:]), true
}

// AddrFromBigInt 从 [*big.Int] 创建 [netip.Addr]。
// 需指定目标 IP 版本。
func AddrFromBigInt(v *big.Int, ver Version) (netip.Addr, error) {
	if v == nil {
		return netip.Addr{}, ErrInvalidBigInt
	}
	switch ver {
	case V4:
		if v.Sign() < 0 || v.BitLen() > 32 {
			return netip.Addr{}, ErrInvalidBigInt
		}
		// 使用字节方式构建，与 V6 路径一致，避免 uint64→uint32 类型收窄。
		var b [4]byte
		vBytes := v.Bytes()
		copy(b[4-len(vBytes):], vBytes)
		return netip.AddrFrom4(b), nil
	case V6:
		if v.Sign() < 0 || v.BitLen() > 128 {
			return netip.Addr{}, ErrInvalidBigInt
		}
		var b [16]byte
		vBytes := v.Bytes()
		copy(b[16-len(vBytes):], vBytes)
		return netip.AddrFrom16(b), nil
	default:
		return netip.Addr{}, ErrInvalidVersion
	}
}

// AddrToBigInt 将地址转换为 [*big.Int]。
// 无效地址返回零值 big.Int。
func AddrToBigInt(addr netip.Addr) *big.Int {
	if !addr.IsValid() {
		return new(big.Int)
	}
	if addr.Is4() || addr.Is4In6() {
		v, _ := AddrToUint32(addr)
		return new(big.Int).SetUint64(uint64(v))
	}
	b := addr.As16()
	return new(big.Int).SetBytes(b[:])
}

// MapToIPv6 将 IPv4 地址转换为 IPv4-mapped IPv6 地址。
// 例如：192.168.1.1 → ::ffff:192.168.1.1
// 如果已经是 IPv6 地址，原样返回。
// 无效地址返回零值。
func MapToIPv6(addr netip.Addr) netip.Addr {
	if !addr.IsValid() {
		return netip.Addr{}
	}
	if addr.Is4() {
		// netip.AddrFrom16 将 IPv4 转为 IPv4-mapped IPv6
		return netip.AddrFrom16(addr.As16())
	}
	return addr
}

// UnmapToIPv4 将 IPv4-mapped IPv6 地址转换为纯 IPv4 地址。
// 例如：::ffff:192.168.1.1 → 192.168.1.1
// 如果是纯 IPv4，原样返回。
// 如果是纯 IPv6（非映射），原样返回。
// 无效地址返回零值。
func UnmapToIPv4(addr netip.Addr) netip.Addr {
	if !addr.IsValid() {
		return netip.Addr{}
	}
	if addr.Is4In6() {
		return addr.Unmap()
	}
	return addr
}

// AddrAdd 对 IP 地址进行加法运算。
// delta 可以为负数表示减法。
// 溢出时返回错误。
//
// 对于 IPv4 地址，使用 uint64 快速路径（零分配）。
// 对于 IPv6 地址，使用 big.Int 运算。
//
// 注意：IPv4-mapped IPv6 地址（如 ::ffff:192.168.1.1）走 IPv4 快速路径，
// 返回结果为纯 IPv4 地址（如 192.168.1.2），不再是 IPv4-mapped 形式。
// 如需保留 IPv4-mapped 形式，请先调用 [UnmapToIPv4] 或对结果调用 [MapToIPv6]。
func AddrAdd(addr netip.Addr, delta int64) (netip.Addr, error) {
	if !addr.IsValid() {
		return netip.Addr{}, ErrInvalidAddress
	}

	// IPv4 快速路径：直接使用 uint64 运算，避免 big.Int 分配
	if addr.Is4() || addr.Is4In6() {
		v, _ := AddrToUint32(addr)
		v64 := uint64(v)
		var result uint64
		if delta >= 0 {
			// 加法：检查上溢
			d64 := uint64(delta)
			if d64 > uint64(^uint32(0))-v64 {
				return netip.Addr{}, fmt.Errorf("IPv4 address overflow (delta=%d): %w", delta, ErrOverflow)
			}
			result = v64 + d64
		} else {
			// 减法：检查下溢
			absDelta := uint64(-delta)
			if absDelta > v64 {
				return netip.Addr{}, fmt.Errorf("IPv4 address underflow (delta=%d): %w", delta, ErrOverflow)
			}
			result = v64 - absDelta
		}
		// 使用字节操作构建地址，避免 uint64->uint32 类型转换
		return addrFrom4Bytes(result), nil
	}

	// IPv6 路径：使用 big.Int
	bi := AddrToBigInt(addr)
	bi.Add(bi, big.NewInt(delta))

	result, err := AddrFromBigInt(bi, V6)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("IPv6 address overflow (delta=%d): %w: %w", delta, ErrOverflow, err)
	}
	return result, nil
}

// addrFrom4Bytes 从 uint64 的低 32 位构建 IPv4 地址。
// 使用字节操作避免 uint64->uint32 类型转换（避免 gosec G115）。
func addrFrom4Bytes(v uint64) netip.Addr {
	var b [4]byte
	b[0] = byte(v >> 24)
	b[1] = byte(v >> 16)
	b[2] = byte(v >> 8)
	b[3] = byte(v)
	return netip.AddrFrom4(b)
}
