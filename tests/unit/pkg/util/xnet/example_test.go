package xnet_test

import (
	"encoding/json"
	"fmt"
	"net/netip"

	"github.com/omeyang/xkit/pkg/util/xnet"
)

func ExampleFormatFullIPAddr() {
	addr := netip.MustParseAddr("192.168.1.1")
	fmt.Println(addr.String())
	fmt.Println(xnet.AddrVersion(addr))
	fmt.Println(xnet.FormatFullIPAddr(addr))
	// Output:
	// 192.168.1.1
	// IPv4
	// 192.168.001.001
}

func ExampleParseRange() {
	r, err := xnet.ParseRange("192.168.1.0/24")
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(r.From())
	fmt.Println(r.To())

	addr := netip.MustParseAddr("192.168.1.100")
	fmt.Println(r.Contains(addr))
	// Output:
	// 192.168.1.0
	// 192.168.1.255
	// true
}

func ExampleParseRanges() {
	set, err := xnet.ParseRanges([]string{
		"10.0.0.50-10.0.0.150",
		"10.0.0.1-10.0.0.100",
		"192.168.1.0/24",
	})
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	// IPSet 自动合并重叠范围
	fmt.Println(len(set.Ranges()))

	addr := netip.MustParseAddr("10.0.0.75")
	fmt.Println(set.Contains(addr))
	// Output:
	// 2
	// true
}

func ExampleClassify() {
	addr := netip.MustParseAddr("192.168.1.1")
	c := xnet.Classify(addr)
	fmt.Println(c.String())
	fmt.Println(c.IsPrivate)
	fmt.Println(c.IsRoutable)
	// Output:
	// private
	// true
	// true
}

func ExampleAddrAdd() {
	addr := netip.MustParseAddr("192.168.1.100")
	next, err := xnet.AddrAdd(addr, 1)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	prev, err := xnet.AddrAdd(addr, -1)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(next)
	fmt.Println(prev)
	// Output:
	// 192.168.1.101
	// 192.168.1.99
}

func ExampleMapToIPv6() {
	addr := netip.MustParseAddr("192.168.1.1")
	mapped := xnet.MapToIPv6(addr)
	fmt.Println(mapped)

	unmapped := xnet.UnmapToIPv4(mapped)
	fmt.Println(unmapped)
	// Output:
	// ::ffff:192.168.1.1
	// 192.168.1.1
}

func ExampleWireRange_IsZero() {
	var w xnet.WireRange
	fmt.Println(w.IsZero())

	w = xnet.WireRange{Start: "10.0.0.1", End: "10.0.0.100"}
	fmt.Println(w.IsZero())
	// Output:
	// true
	// false
}

func ExampleWireRangeFrom() {
	r, err := xnet.ParseRange("192.168.1.1-192.168.1.100")
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	w, err := xnet.WireRangeFrom(r)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	data, err := json.Marshal(w)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(string(data))
	// Output:
	// {"start":"192.168.1.1","end":"192.168.1.100"}
}
