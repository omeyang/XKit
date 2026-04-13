package xmac

import (
	"fmt"
	"net"
)

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
//
// 设计决策: 使用手动逐字节循环而非 bytes.Compare，避免切片化导致的逃逸分析开销，
// 使比较操作保持在栈上完成（零堆分配）。
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
	if a == broadcastAddr() {
		return Addr{}, ErrOverflow
	}
	// 利用 uint8 自然回绕：从最低字节加 1，回绕到 0 时进位到上一字节
	next := a
	for i := 5; i >= 0; i-- {
		next.bytes[i]++
		if next.bytes[i] != 0 {
			return next, nil // 无进位，完成
		}
	}
	// 此分支不可达：广播地址在函数入口已拦截
	return Addr{}, ErrOverflow
}

// Prev 返回前一个 MAC 地址（当前地址 -1）。
// 如果 a 是 00:00:00:00:00:00，返回 [ErrUnderflow]。
func (a Addr) Prev() (Addr, error) {
	if a == (Addr{}) {
		return Addr{}, ErrUnderflow
	}
	// 从最低字节减 1：若该字节非 0 则直接减完成；若为 0 则置为 0xff 并向上借位
	prev := a
	for i := 5; i >= 0; i-- {
		if prev.bytes[i] != 0 {
			prev.bytes[i]--
			return prev, nil // 无借位，完成
		}
		prev.bytes[i] = 0xff // 借位继续
	}
	// 此分支不可达：零地址在函数入口已拦截
	return Addr{}, ErrUnderflow
}

// GoString 实现 [fmt.GoStringer]，返回 Go 语法表示。
// 用于 %#v 格式化，输出形如 xmac.MustParse("aa:bb:cc:dd:ee:ff")。
// 无效地址返回 xmac.Addr{}。
func (a Addr) GoString() string {
	if !a.IsValid() {
		return "xmac.Addr{}"
	}
	return fmt.Sprintf("xmac.MustParse(%q)", a.String())
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
