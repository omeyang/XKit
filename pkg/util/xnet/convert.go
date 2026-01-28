package xnet

import (
	"encoding/binary"
	"math/big"
	"net/netip"
)

// Version 表示 IP 协议版本。
type Version uint8

const (
	// V0 表示无效或未知的 IP 版本。
	V0 Version = 0
	// V4 表示 IPv4。
	V4 Version = 4
	// V6 表示 IPv6。
	V6 Version = 6
)

// String 返回版本的字符串表示。
func (v Version) String() string {
	switch v {
	case V4:
		return "IPv4"
	case V6:
		return "IPv6"
	default:
		return "unknown"
	}
}

// AddrVersion 返回 addr 的 IP 版本（V4 或 V6）。
// IPv4-mapped IPv6 地址视为 V4。
// 无效地址返回 V0。
func AddrVersion(addr netip.Addr) Version {
	if addr.Is4() || addr.Is4In6() {
		return V4
	}
	if addr.IsValid() {
		return V6
	}
	return V0
}

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
