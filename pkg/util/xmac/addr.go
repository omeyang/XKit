package xmac

import "net"

// Addr 表示 48 位 MAC 地址（EUI-48/MAC-48）。
//
// Addr 是不可变值类型：
//   - 零值表示无效地址，IsValid() 返回 false
//   - 可直接比较（==）和用作 map key
//   - 并发安全，无需加锁
//
// 使用 [Parse] 或 [MustParse] 创建有效地址：
//
//	addr, err := xmac.Parse("aa:bb:cc:dd:ee:ff")
//	addr := xmac.MustParse("aa:bb:cc:dd:ee:ff")
type Addr struct {
	// 使用固定大小数组而非切片：
	// 1. 值语义，可比较，可作为 map key
	// 2. 栈分配，零堆开销
	// 3. 编译期大小检查
	bytes [6]byte
}

// AddrFrom6 从 6 字节数组创建 MAC 地址。
func AddrFrom6(b [6]byte) Addr {
	return Addr{bytes: b}
}

// Bytes 返回 MAC 地址的字节表示（长度始终为 6）。
// 返回副本，修改不影响原值。
func (a Addr) Bytes() [6]byte {
	return a.bytes
}

// IsValid 报告 a 是否为有效的非零 MAC 地址。
// 零值 Addr{} 返回 false。
func (a Addr) IsValid() bool {
	return a != Addr{}
}

// Compare 比较两个 MAC 地址的字节顺序。
// 返回值：-1 (a < b), 0 (a == b), 1 (a > b)。
// 按网络字节序（大端）比较。
func (a Addr) Compare(b Addr) int {
	for i := range 6 {
		if a.bytes[i] < b.bytes[i] {
			return -1
		}
		if a.bytes[i] > b.bytes[i] {
			return 1
		}
	}
	return 0
}

// Next 返回下一个 MAC 地址（当前地址 +1）。
// 如果 a 是 ff:ff:ff:ff:ff:ff，返回 [ErrOverflow]。
func (a Addr) Next() (Addr, error) {
	if a == Broadcast {
		return Addr{}, ErrOverflow
	}
	var next Addr
	carry := uint16(1)
	for i := 5; i >= 0; i-- {
		sum := uint16(a.bytes[i]) + carry
		next.bytes[i] = byte(sum)
		carry = sum >> 8
	}
	return next, nil
}

// Prev 返回前一个 MAC 地址（当前地址 -1）。
// 如果 a 是 00:00:00:00:00:00，返回 [ErrUnderflow]。
func (a Addr) Prev() (Addr, error) {
	if a == (Addr{}) {
		return Addr{}, ErrUnderflow
	}
	var prev Addr
	borrow := uint16(1)
	for i := 5; i >= 0; i-- {
		// 使用 uint16 避免下溢：当 a.bytes[i] == 0 且 borrow == 1 时，
		// 0 - 1 在 uint16 中会得到 0xFFFF，取低 8 位得 0xFF（正确的借位结果）
		diff := uint16(a.bytes[i]) - borrow
		prev.bytes[i] = byte(diff)
		// 如果发生借位（原值小于借位值），设置下一轮借位
		if a.bytes[i] < byte(borrow) {
			borrow = 1
		} else {
			borrow = 0
		}
	}
	return prev, nil
}

// HardwareAddr 返回 [net.HardwareAddr] 表示。
// 返回副本，修改不影响原值。
// 无效地址返回 nil。
func (a Addr) HardwareAddr() net.HardwareAddr {
	if !a.IsValid() {
		return nil
	}
	hw := make(net.HardwareAddr, 6)
	copy(hw, a.bytes[:])
	return hw
}
