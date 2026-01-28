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
		panic(err)
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
		panic(err)
	}

	// IPSet 自动合并重叠范围
	fmt.Println(len(set.Ranges()))

	addr := netip.MustParseAddr("10.0.0.75")
	fmt.Println(set.Contains(addr))
	// Output:
	// 2
	// true
}

func ExampleWireRangeFrom() {
	r, err := xnet.ParseRange("192.168.1.1-192.168.1.100")
	if err != nil {
		panic(err)
	}
	w := xnet.WireRangeFrom(r)
	data, err := json.Marshal(w)
	if err != nil {
		panic(err)
	}
	fmt.Println(string(data))
	// Output:
	// {"s":"192.168.1.1","e":"192.168.1.100"}
}
