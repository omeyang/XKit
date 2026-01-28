package xnet

import (
	"net/netip"
	"testing"

	"go4.org/netipx"
)

// =============================================================================
// FullIP 格式化模糊测试
// =============================================================================

func FuzzFullIPRoundTrip(f *testing.F) {
	f.Add("192.168.1.1")
	f.Add("10.0.0.1")
	f.Add("0.0.0.0")
	f.Add("255.255.255.255")
	f.Add("::1")
	f.Add("2001:db8::1")
	f.Add("::ffff:192.168.1.1")

	f.Fuzz(func(t *testing.T, s string) {
		addr, err := netip.ParseAddr(s)
		if err != nil {
			return
		}
		full := FormatFullIPAddr(addr)
		if full == "" {
			t.Fatalf("FormatFullIPAddr returned empty for valid addr %q", s)
		}
		restored, err := ParseFullIP(full)
		if err != nil {
			t.Fatalf("ParseFullIP(%q) failed: %v (from %q)", full, err, s)
		}
		// Unmap for comparison since FormatFullIPAddr unmaps IPv4-mapped
		expected := addr
		if expected.Is4In6() {
			expected = expected.Unmap()
		}
		if expected.Compare(restored) != 0 {
			t.Errorf("round-trip mismatch: %q → %q → %q (expected %q)", s, full, restored, expected)
		}
	})
}

// =============================================================================
// BigInt 转换模糊测试
// =============================================================================

func FuzzBigIntRoundTrip(f *testing.F) {
	f.Add("0.0.0.0")
	f.Add("192.168.1.1")
	f.Add("255.255.255.255")
	f.Add("::1")
	f.Add("2001:db8::1")
	f.Add("ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff")

	f.Fuzz(func(t *testing.T, s string) {
		addr, err := netip.ParseAddr(s)
		if err != nil {
			return
		}
		ver := AddrVersion(addr)
		if ver == V0 {
			return
		}
		bi := AddrToBigInt(addr)
		restored, err := AddrFromBigInt(bi, ver)
		if err != nil {
			t.Fatalf("AddrFromBigInt failed for %q (ver=%v, bigint=%v): %v", s, ver, bi, err)
		}
		expected := addr
		if expected.Is4In6() {
			expected = expected.Unmap()
		}
		if expected.Compare(restored) != 0 {
			t.Errorf("BigInt round-trip mismatch: %q → %v → %q", s, bi, restored)
		}
	})
}

// =============================================================================
// 范围解析模糊测试
// =============================================================================

func FuzzParseRange(f *testing.F) {
	f.Add("192.168.1.1")
	f.Add("192.168.1.0/24")
	f.Add("192.168.1.0/255.255.255.0")
	f.Add("10.0.0.1-10.0.0.100")
	f.Add("")
	f.Add("invalid")
	f.Add("::1/128")
	f.Add("  192.168.1.0/24  ")

	f.Fuzz(func(t *testing.T, s string) {
		r, err := ParseRange(s)
		if err != nil {
			return
		}
		// Valid range should have From <= To
		if r.From().Compare(r.To()) > 0 {
			t.Errorf("range From > To: %s", s)
		}
		// From should be contained
		if !r.Contains(r.From()) {
			t.Errorf("range does not contain its From: %s", s)
		}
		// To should be contained
		if !r.Contains(r.To()) {
			t.Errorf("range does not contain its To: %s", s)
		}
	})
}

// =============================================================================
// WireRange 序列化模糊测试
// =============================================================================

func FuzzWireRangeRoundTrip(f *testing.F) {
	f.Add("192.168.1.1", "192.168.1.100")
	f.Add("10.0.0.1", "10.0.0.1")
	f.Add("::1", "::ff")
	f.Add("0.0.0.0", "255.255.255.255")

	f.Fuzz(func(t *testing.T, start, end string) {
		startAddr, err := netip.ParseAddr(start)
		if err != nil {
			return
		}
		endAddr, err := netip.ParseAddr(end)
		if err != nil {
			return
		}

		// 版本必须相同
		if AddrVersion(startAddr) != AddrVersion(endAddr) {
			return
		}

		// start <= end
		if startAddr.Compare(endAddr) > 0 {
			return
		}

		// 创建 IPRange
		r := netipx.IPRangeFrom(startAddr, endAddr)
		if !r.IsValid() {
			return
		}

		// WireRange 往返测试
		w := WireRangeFrom(r)
		restored, err := w.ToIPRange()
		if err != nil {
			t.Fatalf("WireRange.ToIPRange failed: %v", err)
		}

		if r.From().Compare(restored.From()) != 0 || r.To().Compare(restored.To()) != 0 {
			t.Errorf("WireRange round-trip mismatch: %v → %v → %v", r, w, restored)
		}
	})
}

// =============================================================================
// RangeContainsV4 模糊测试
// =============================================================================

func FuzzRangeContainsV4(f *testing.F) {
	f.Add("192.168.1.1", "192.168.1.100", "192.168.1.50")
	f.Add("10.0.0.0", "10.255.255.255", "10.128.0.1")
	f.Add("0.0.0.0", "255.255.255.255", "128.0.0.1")

	f.Fuzz(func(t *testing.T, fromStr, toStr, addrStr string) {
		from, err := netip.ParseAddr(fromStr)
		if err != nil || !from.Is4() {
			return
		}
		to, err := netip.ParseAddr(toStr)
		if err != nil || !to.Is4() {
			return
		}
		addr, err := netip.ParseAddr(addrStr)
		if err != nil || !addr.Is4() {
			return
		}

		// from <= to
		if from.Compare(to) > 0 {
			return
		}

		// 比较 RangeContainsV4 和 IPRange.Contains 的结果
		r := netipx.IPRangeFrom(from, to)
		expected := r.Contains(addr)
		got := RangeContainsV4(from, to, addr)

		if expected != got {
			t.Errorf("RangeContainsV4(%s, %s, %s) = %v, want %v", from, to, addr, got, expected)
		}
	})
}

// =============================================================================
// IPSetFromRanges 模糊测试
// =============================================================================

func FuzzIPSetFromRanges(f *testing.F) {
	f.Add("192.168.1.1", "192.168.1.100", "10.0.0.1", "10.0.0.50")

	f.Fuzz(func(t *testing.T, from1, to1, from2, to2 string) {
		addr1, err := netip.ParseAddr(from1)
		if err != nil {
			return
		}
		addr2, err := netip.ParseAddr(to1)
		if err != nil {
			return
		}
		addr3, err := netip.ParseAddr(from2)
		if err != nil {
			return
		}
		addr4, err := netip.ParseAddr(to2)
		if err != nil {
			return
		}

		// 确保每个范围的 from <= to 且版本相同
		if addr1.Compare(addr2) > 0 || AddrVersion(addr1) != AddrVersion(addr2) {
			return
		}
		if addr3.Compare(addr4) > 0 || AddrVersion(addr3) != AddrVersion(addr4) {
			return
		}

		r1 := netipx.IPRangeFrom(addr1, addr2)
		r2 := netipx.IPRangeFrom(addr3, addr4)

		if !r1.IsValid() || !r2.IsValid() {
			return
		}

		ranges := []netipx.IPRange{r1, r2}
		set, err := IPSetFromRanges(ranges)
		if err != nil {
			t.Fatalf("IPSetFromRanges failed: %v", err)
		}

		// 验证原始范围的端点都在 set 中
		if !set.Contains(r1.From()) {
			t.Errorf("set should contain r1.From(): %v", r1.From())
		}
		if !set.Contains(r1.To()) {
			t.Errorf("set should contain r1.To(): %v", r1.To())
		}
		if !set.Contains(r2.From()) {
			t.Errorf("set should contain r2.From(): %v", r2.From())
		}
		if !set.Contains(r2.To()) {
			t.Errorf("set should contain r2.To(): %v", r2.To())
		}
	})
}

// =============================================================================
// MergeRanges 模糊测试
// =============================================================================

func FuzzMergeRanges(f *testing.F) {
	f.Add("10.0.0.1", "10.0.0.100", "10.0.0.50", "10.0.0.150")

	f.Fuzz(func(t *testing.T, from1, to1, from2, to2 string) {
		addr1, err := netip.ParseAddr(from1)
		if err != nil {
			return
		}
		addr2, err := netip.ParseAddr(to1)
		if err != nil {
			return
		}
		addr3, err := netip.ParseAddr(from2)
		if err != nil {
			return
		}
		addr4, err := netip.ParseAddr(to2)
		if err != nil {
			return
		}

		// 确保每个范围的 from <= to 且版本相同
		if addr1.Compare(addr2) > 0 || AddrVersion(addr1) != AddrVersion(addr2) {
			return
		}
		if addr3.Compare(addr4) > 0 || AddrVersion(addr3) != AddrVersion(addr4) {
			return
		}

		r1 := netipx.IPRangeFrom(addr1, addr2)
		r2 := netipx.IPRangeFrom(addr3, addr4)

		if !r1.IsValid() || !r2.IsValid() {
			return
		}

		ranges := []netipx.IPRange{r1, r2}
		merged, err := MergeRanges(ranges)
		if err != nil {
			t.Fatalf("MergeRanges failed: %v", err)
		}

		if merged == nil {
			t.Fatal("MergeRanges returned nil for valid input")
		}

		// 验证合并结果：所有原始端点都应该被包含
		set, _ := IPSetFromRanges(merged)
		if !set.Contains(r1.From()) || !set.Contains(r1.To()) {
			t.Errorf("merged ranges should contain all points from r1")
		}
		if !set.Contains(r2.From()) || !set.Contains(r2.To()) {
			t.Errorf("merged ranges should contain all points from r2")
		}
	})
}

// =============================================================================
// Version 模糊测试
// =============================================================================

func FuzzAddrVersion(f *testing.F) {
	f.Add("192.168.1.1")
	f.Add("::1")
	f.Add("::ffff:192.168.1.1")
	f.Add("2001:db8::1")

	f.Fuzz(func(t *testing.T, s string) {
		addr, err := netip.ParseAddr(s)
		if err != nil {
			return
		}

		ver := AddrVersion(addr)

		// 验证版本与 netip.Addr 方法一致
		if addr.Is4() || addr.Is4In6() {
			if ver != V4 {
				t.Errorf("AddrVersion(%s) = %v, want V4", s, ver)
			}
		} else if addr.Is6() {
			if ver != V6 {
				t.Errorf("AddrVersion(%s) = %v, want V6", s, ver)
			}
		}
	})
}

// =============================================================================
// uint32 转换模糊测试
// =============================================================================

func FuzzUint32RoundTrip(f *testing.F) {
	f.Add(uint32(0))
	f.Add(uint32(0xC0A80101)) // 192.168.1.1
	f.Add(uint32(0xFFFFFFFF))
	f.Add(uint32(0x0A000001)) // 10.0.0.1

	f.Fuzz(func(t *testing.T, v uint32) {
		addr := AddrFromUint32(v)
		if !addr.Is4() {
			t.Errorf("AddrFromUint32(%d) should return IPv4, got %v", v, addr)
		}

		restored, ok := AddrToUint32(addr)
		if !ok {
			t.Errorf("AddrToUint32(%v) returned false", addr)
		}
		if restored != v {
			t.Errorf("uint32 round-trip mismatch: %d → %v → %d", v, addr, restored)
		}
	})
}

// =============================================================================
// MapToIPv6/UnmapToIPv4 模糊测试
// =============================================================================

func FuzzMapUnmapRoundTrip(f *testing.F) {
	f.Add("192.168.1.1")
	f.Add("10.0.0.1")
	f.Add("0.0.0.0")
	f.Add("255.255.255.255")

	f.Fuzz(func(t *testing.T, s string) {
		addr, err := netip.ParseAddr(s)
		if err != nil {
			return
		}
		// 只测试 IPv4，因为 IPv6 → map → unmap 不保证等价
		if !addr.Is4() {
			return
		}

		mapped := MapToIPv6(addr)
		if !mapped.Is4In6() {
			t.Errorf("MapToIPv6(%s) should be IPv4-mapped, got %v", s, mapped)
		}

		unmapped := UnmapToIPv4(mapped)
		if addr.Compare(unmapped) != 0 {
			t.Errorf("map/unmap round-trip mismatch: %s → %s → %s", addr, mapped, unmapped)
		}
	})
}

// =============================================================================
// AddrAdd 模糊测试
// =============================================================================

func FuzzAddrAdd(f *testing.F) {
	f.Add("192.168.1.100", int64(1))
	f.Add("192.168.1.100", int64(-1))
	f.Add("10.0.0.1", int64(100))
	f.Add("::1", int64(1))

	f.Fuzz(func(t *testing.T, s string, delta int64) {
		addr, err := netip.ParseAddr(s)
		if err != nil {
			return
		}

		result, err := AddrAdd(addr, delta)
		if err != nil {
			// 溢出是允许的
			return
		}

		// 验证结果有效
		if !result.IsValid() {
			t.Errorf("AddrAdd(%s, %d) returned invalid address", s, delta)
		}

		// 验证版本不变
		if AddrVersion(addr) != AddrVersion(result) {
			t.Errorf("AddrAdd(%s, %d) changed version: %v → %v", s, delta, AddrVersion(addr), AddrVersion(result))
		}
	})
}

// =============================================================================
// RangeSize 模糊测试
// =============================================================================

func FuzzRangeSize(f *testing.F) {
	f.Add("192.168.1.1", "192.168.1.100")
	f.Add("10.0.0.0", "10.0.0.255")
	f.Add("::1", "::ff")

	f.Fuzz(func(t *testing.T, fromStr, toStr string) {
		from, err := netip.ParseAddr(fromStr)
		if err != nil {
			return
		}
		to, err := netip.ParseAddr(toStr)
		if err != nil {
			return
		}

		// 版本必须相同
		if AddrVersion(from) != AddrVersion(to) {
			return
		}

		// from <= to
		if from.Compare(to) > 0 {
			return
		}

		r := netipx.IPRangeFrom(from, to)
		if !r.IsValid() {
			return
		}

		size := RangeSize(r)
		if size == nil {
			t.Errorf("RangeSize returned nil for valid range %s-%s", from, to)
			return
		}

		// 大小应该 >= 1
		if size.Sign() < 1 {
			t.Errorf("RangeSize(%s-%s) = %v, want >= 1", from, to, size)
		}
	})
}
