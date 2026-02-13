package xmac

import (
	"encoding/json"
	"testing"
)

// parseFuzzMAC 验证字节切片长度为 6 并解析为 MAC 地址。
// 返回 ok=false 表示输入无效（长度不为 6 或解析失败）。
func parseFuzzMAC(b []byte) (Addr, bool) {
	if len(b) != 6 {
		return Addr{}, false
	}

	addr, err := ParseBytes(b)
	if err != nil {
		return Addr{}, false
	}

	return addr, true
}

// parseTwoFuzzMACs 解析两个字节切片为 MAC 地址对。
func parseTwoFuzzMACs(b1, b2 []byte) (a, b Addr, ok bool) {
	a, ok = parseFuzzMAC(b1)
	if !ok {
		return a, b, false
	}

	b, ok = parseFuzzMAC(b2)
	if !ok {
		return a, b, false
	}

	return a, b, true
}

// assertCastExclusivity 验证 IsUnicast 和 IsMulticast 对有效地址互斥。
func assertCastExclusivity(t *testing.T, addr Addr) {
	t.Helper()

	if !addr.IsValid() {
		return
	}

	if addr.IsUnicast() && addr.IsMulticast() {
		t.Errorf("addr is both unicast and multicast: %v", addr)
	}
	if !addr.IsUnicast() && !addr.IsMulticast() {
		t.Errorf("addr is neither unicast nor multicast: %v", addr)
	}
}

// assertOUIReconstructed 验证 OUI+NIC 拼接等于原地址。
func assertOUIReconstructed(t *testing.T, addr Addr) {
	t.Helper()

	if !addr.IsValid() {
		return
	}

	oui := addr.OUI()
	nic := addr.NIC()
	reconstructed := AddrFrom6([6]byte{oui[0], oui[1], oui[2], nic[0], nic[1], nic[2]})
	if reconstructed != addr {
		t.Errorf("OUI+NIC reconstruction failed: %v -> OUI=%v NIC=%v -> %v",
			addr, oui, nic, reconstructed)
	}
}

// assertValidationConsistency 验证 MAC 地址的各属性之间的一致性。
func assertValidationConsistency(t *testing.T, addr Addr) {
	t.Helper()

	assertCastExclusivity(t, addr)

	// IsUsable == IsValid && !IsSpecial
	expectedUsable := addr.IsValid() && !addr.IsSpecial()
	if addr.IsUsable() != expectedUsable {
		t.Errorf("IsUsable inconsistent: IsValid=%v, IsSpecial=%v, IsUsable=%v",
			addr.IsValid(), addr.IsSpecial(), addr.IsUsable())
	}

	// IsSpecial == IsZero || IsBroadcast
	expectedSpecial := addr.IsZero() || addr.IsBroadcast()
	if addr.IsSpecial() != expectedSpecial {
		t.Errorf("IsSpecial inconsistent: IsZero=%v, IsBroadcast=%v, IsSpecial=%v",
			addr.IsZero(), addr.IsBroadcast(), addr.IsSpecial())
	}

	assertOUIReconstructed(t, addr)
}

func FuzzParse(f *testing.F) {
	// 添加种子语料
	seeds := []string{
		"aa:bb:cc:dd:ee:ff",
		"AA:BB:CC:DD:EE:FF",
		"aa-bb-cc-dd-ee-ff",
		"aabb.ccdd.eeff",
		"aabbccddeeff",
		"00:00:00:00:00:00",
		"ff:ff:ff:ff:ff:ff",
		"",
		"invalid",
		"aa:bb:cc",
		"aa:bb:cc:dd:ee:ff:00:11",
		"gg:hh:ii:jj:kk:ll",
		"  aa:bb:cc:dd:ee:ff  ",
	}

	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, s string) {
		addr, err := Parse(s)
		if err != nil {
			// 解析失败是预期的
			return
		}

		// 零地址是特殊情况：解析成功但 IsValid() 为 false，String() 返回空
		if !addr.IsValid() {
			if addr.String() != "" {
				t.Errorf("invalid addr.String() = %q, want empty", addr.String())
			}
			return
		}

		// 解析成功，验证往返一致性
		str := addr.String()
		addr2, err := Parse(str)
		if err != nil {
			t.Errorf("round-trip parse failed: %q -> %v -> %q: %v", s, addr, str, err)
			return
		}

		if addr != addr2 {
			t.Errorf("round-trip mismatch: %q -> %v -> %q -> %v", s, addr, str, addr2)
		}

		// 验证属性的一致性
		if addr.IsValid() != addr2.IsValid() {
			t.Errorf("IsValid mismatch after round-trip")
		}

		// 验证字节一致性
		if addr.Bytes() != addr2.Bytes() {
			t.Errorf("Bytes mismatch after round-trip")
		}
	})
}

func FuzzParseBytes(f *testing.F) {
	// 添加种子语料
	f.Add([]byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff})
	f.Add([]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
	f.Add([]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff})
	f.Add([]byte{})
	f.Add([]byte{0xaa, 0xbb, 0xcc})
	f.Add([]byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff, 0x00})

	f.Fuzz(func(t *testing.T, b []byte) {
		addr, err := ParseBytes(b)
		if err != nil {
			// 长度不为 6 时预期失败
			if len(b) != 6 {
				return
			}
			t.Errorf("ParseBytes(%v) unexpected error: %v", b, err)
			return
		}

		// 验证长度为 6
		if len(b) != 6 {
			t.Errorf("ParseBytes succeeded with len=%d", len(b))
			return
		}

		// 验证字节一致性
		addrBytes := addr.Bytes()
		if addrBytes != [6]byte(b) {
			t.Errorf("bytes mismatch: got %v, want %v", addrBytes, b)
		}
	})
}

// =============================================================================
// JSON 序列化往返测试
// =============================================================================

func FuzzMarshalUnmarshalJSON(f *testing.F) {
	// 添加种子语料
	seeds := []string{
		"aa:bb:cc:dd:ee:ff",
		"00:00:00:00:00:00",
		"ff:ff:ff:ff:ff:ff",
		"01:23:45:67:89:ab",
		"02:00:00:00:00:01", // LAA
		"01:00:5e:00:00:01", // multicast
	}

	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, s string) {
		addr, err := Parse(s)
		if err != nil {
			return
		}

		// Marshal
		data, err := json.Marshal(addr)
		if err != nil {
			t.Errorf("json.Marshal(%v) failed: %v", addr, err)
			return
		}

		// Unmarshal
		var addr2 Addr
		if err := json.Unmarshal(data, &addr2); err != nil {
			t.Errorf("json.Unmarshal(%s) failed: %v", data, err)
			return
		}

		// 验证往返一致性
		if addr != addr2 {
			t.Errorf("JSON round-trip mismatch: %v -> %s -> %v", addr, data, addr2)
		}

		// 验证属性一致性
		if addr.IsValid() != addr2.IsValid() {
			t.Errorf("IsValid mismatch after JSON round-trip")
		}
		if addr.IsUsable() != addr2.IsUsable() {
			t.Errorf("IsUsable mismatch after JSON round-trip")
		}
	})
}

// =============================================================================
// Text 编码往返测试
// =============================================================================

func FuzzMarshalUnmarshalText(f *testing.F) {
	seeds := []string{
		"aa:bb:cc:dd:ee:ff",
		"00:00:00:00:00:00",
		"ff:ff:ff:ff:ff:ff",
		"01:23:45:67:89:ab",
	}

	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, s string) {
		addr, err := Parse(s)
		if err != nil {
			return
		}

		// MarshalText
		text, err := addr.MarshalText()
		if err != nil {
			t.Errorf("MarshalText(%v) failed: %v", addr, err)
			return
		}

		// UnmarshalText
		var addr2 Addr
		if err := addr2.UnmarshalText(text); err != nil {
			t.Errorf("UnmarshalText(%s) failed: %v", text, err)
			return
		}

		// 验证往返一致性
		if addr != addr2 {
			t.Errorf("Text round-trip mismatch: %v -> %s -> %v", addr, text, addr2)
		}
	})
}

// =============================================================================
// SQL 接口往返测试
// =============================================================================

func FuzzScanValue(f *testing.F) {
	seeds := []string{
		"aa:bb:cc:dd:ee:ff",
		"00:00:00:00:00:00",
		"ff:ff:ff:ff:ff:ff",
		"01:23:45:67:89:ab",
	}

	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, s string) {
		addr, err := Parse(s)
		if err != nil {
			return
		}

		// Value
		val, err := addr.Value()
		if err != nil {
			t.Errorf("Value(%v) failed: %v", addr, err)
			return
		}

		// Scan
		var addr2 Addr
		if err := addr2.Scan(val); err != nil {
			t.Errorf("Scan(%v) failed: %v", val, err)
			return
		}

		// 验证往返一致性
		if addr != addr2 {
			t.Errorf("SQL round-trip mismatch: %v -> %v -> %v", addr, val, addr2)
		}
	})
}

// =============================================================================
// Next/Prev 互逆测试
// =============================================================================

func FuzzNextPrevInverse(f *testing.F) {
	// 添加种子语料（避开边界值）
	f.Add([]byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff})
	f.Add([]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x01})
	f.Add([]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xfe})
	f.Add([]byte{0x01, 0x23, 0x45, 0x67, 0x89, 0xab})
	f.Add([]byte{0x80, 0x00, 0x00, 0x00, 0x00, 0x00})

	f.Fuzz(func(t *testing.T, b []byte) {
		addr, ok := parseFuzzMAC(b)
		if !ok {
			return
		}

		// 跳过边界值：全零（Prev 会下溢）和广播（Next 会上溢）
		if addr.IsZero() || addr.IsBroadcast() {
			return
		}

		// 测试 Next().Prev() == addr
		next, err := addr.Next()
		if err != nil {
			return
		}
		back, err := next.Prev()
		if err != nil {
			t.Errorf("Next().Prev() failed: %v", err)
			return
		}
		if back != addr {
			t.Errorf("Next().Prev() mismatch: %v -> %v -> %v", addr, next, back)
		}

		// 测试 Prev().Next() == addr
		prev, err := addr.Prev()
		if err != nil {
			return
		}
		forward, err := prev.Next()
		if err != nil {
			t.Errorf("Prev().Next() failed: %v", err)
			return
		}
		if forward != addr {
			t.Errorf("Prev().Next() mismatch: %v -> %v -> %v", addr, prev, forward)
		}
	})
}

// =============================================================================
// 各格式往返测试
// =============================================================================

func FuzzFormatParseRoundTrip(f *testing.F) {
	f.Add([]byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff})
	f.Add([]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x01})
	f.Add([]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff})
	f.Add([]byte{0x01, 0x23, 0x45, 0x67, 0x89, 0xab})

	formats := []Format{
		FormatColon,
		FormatDash,
		FormatDot,
		FormatBare,
		FormatColonUpper,
		FormatDashUpper,
		FormatDotUpper,
		FormatBareUpper,
	}

	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) != 6 {
			return
		}

		addr, err := ParseBytes(b)
		if err != nil {
			return
		}

		// 跳过全零地址（String() 返回空）
		if !addr.IsValid() {
			return
		}

		for _, format := range formats {
			// 格式化
			str := addr.FormatString(format)
			if str == "" {
				t.Errorf("FormatString(%v, %v) returned empty", addr, format)
				continue
			}

			// 解析回来
			addr2, err := Parse(str)
			if err != nil {
				t.Errorf("Parse(%q) failed after FormatString: %v", str, err)
				continue
			}

			// 验证一致性
			if addr != addr2 {
				t.Errorf("Format round-trip mismatch (format=%v): %v -> %q -> %v", format, addr, str, addr2)
			}
		}
	})
}

// =============================================================================
// Compare 属性测试
// =============================================================================

func FuzzCompareProperties(f *testing.F) {
	// 添加成对的种子
	f.Add(
		[]byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff},
		[]byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0x00},
	)
	f.Add(
		[]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		[]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
	)
	f.Add(
		[]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06},
		[]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06},
	)

	f.Fuzz(func(t *testing.T, b1, b2 []byte) {
		a, b, ok := parseTwoFuzzMACs(b1, b2)
		if !ok {
			return
		}

		cmpAB := a.Compare(b)
		cmpBA := b.Compare(a)

		// 反对称性：Compare(a, b) == -Compare(b, a)
		if cmpAB != -cmpBA {
			t.Errorf("Compare antisymmetry violated: %v.Compare(%v)=%d, %v.Compare(%v)=%d",
				a, b, cmpAB, b, a, cmpBA)
		}

		// 自反性：Compare(a, a) == 0
		// 使用副本来测试自反性，避免 gocritic dupArg 警告
		aCopy := a
		cmpAA := a.Compare(aCopy)
		if cmpAA != 0 {
			t.Errorf("Compare reflexivity violated: %v.Compare(self)=%d", a, cmpAA)
		}

		// 相等性一致性
		if cmpAB == 0 && a != b {
			t.Errorf("Compare==0 but a!=b: %v, %v", a, b)
		}
		if a == b && cmpAB != 0 {
			t.Errorf("a==b but Compare!=0: %v, %v, cmp=%d", a, b, cmpAB)
		}
	})
}

// =============================================================================
// 验证方法一致性测试
// =============================================================================

func FuzzValidationConsistency(f *testing.F) {
	f.Add([]byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff})
	f.Add([]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
	f.Add([]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff})
	f.Add([]byte{0x01, 0x00, 0x5e, 0x00, 0x00, 0x01}) // multicast
	f.Add([]byte{0x02, 0x00, 0x00, 0x00, 0x00, 0x01}) // LAA

	f.Fuzz(func(t *testing.T, b []byte) {
		addr, ok := parseFuzzMAC(b)
		if !ok {
			return
		}

		assertValidationConsistency(t, addr)
	})
}

// =============================================================================
// HardwareAddr 转换往返测试
// =============================================================================

func FuzzHardwareAddrRoundTrip(f *testing.F) {
	f.Add([]byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff})
	f.Add([]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x01})
	f.Add([]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff})

	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) != 6 {
			return
		}

		addr, err := ParseBytes(b)
		if err != nil {
			return
		}

		// 跳过无效地址
		if !addr.IsValid() {
			return
		}

		// 转换为 HardwareAddr
		hw := addr.HardwareAddr()
		if hw == nil {
			t.Errorf("HardwareAddr() returned nil for valid addr: %v", addr)
			return
		}

		// 转换回来
		addr2, err := FromHardwareAddr(hw)
		if err != nil {
			t.Errorf("FromHardwareAddr(%v) failed: %v", hw, err)
			return
		}

		// 验证一致性
		if addr != addr2 {
			t.Errorf("HardwareAddr round-trip mismatch: %v -> %v -> %v", addr, hw, addr2)
		}
	})
}

// =============================================================================
// RangeReverse 一致性测试
// =============================================================================

func FuzzRangeReverseConsistency(f *testing.F) {
	// 添加成对的种子
	f.Add(
		[]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x01},
		[]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x0a},
	)
	f.Add(
		[]byte{0xaa, 0xbb, 0xcc, 0x00, 0x00, 0x00},
		[]byte{0xaa, 0xbb, 0xcc, 0x00, 0x00, 0x10},
	)
	f.Add(
		[]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0xfe},
		[]byte{0x00, 0x00, 0x00, 0x00, 0x01, 0x02},
	)

	f.Fuzz(func(t *testing.T, b1, b2 []byte) {
		from, to, ok := parseTwoFuzzMACs(b1, b2)
		if !ok {
			return
		}

		// 确保 from <= to
		if from.Compare(to) > 0 {
			from, to = to, from
		}

		// 限制范围大小以避免内存耗尽
		if RangeCount(from, to) > 1000 {
			return
		}

		// 正向收集
		var forward []Addr
		for addr := range Range(from, to) {
			forward = append(forward, addr)
		}

		// 反向收集
		var reverse []Addr
		for addr := range RangeReverse(from, to) {
			reverse = append(reverse, addr)
		}

		// 验证长度相同
		if len(forward) != len(reverse) {
			t.Errorf("length mismatch: forward=%d, reverse=%d", len(forward), len(reverse))
			return
		}

		// 验证反向后内容相同
		for i := range forward {
			j := len(forward) - 1 - i
			if forward[i] != reverse[j] {
				t.Errorf("content mismatch at %d: forward=%v, reverse[%d]=%v",
					i, forward[i], j, reverse[j])
				return
			}
		}
	})
}
