package xnet

import (
	"net/netip"
	"testing"
)

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
