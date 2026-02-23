package xmac_test

import (
	"encoding/json"
	"fmt"

	"github.com/omeyang/xkit/pkg/util/xmac"
)

func ExampleParse() {
	// 支持多种格式
	formats := []string{
		"aa:bb:cc:dd:ee:ff", // 冒号格式
		"AA-BB-CC-DD-EE-FF", // 短线格式（大写）
		"aabb.ccdd.eeff",    // 点格式（Cisco 风格）
		"AABBCCDDEEFF",      // 无分隔符
	}

	for _, s := range formats {
		addr, err := xmac.Parse(s)
		if err != nil {
			fmt.Printf("Parse(%q) error: %v\n", s, err)
			continue
		}
		fmt.Printf("Parse(%q) = %s\n", s, addr)
	}

	// Output:
	// Parse("aa:bb:cc:dd:ee:ff") = aa:bb:cc:dd:ee:ff
	// Parse("AA-BB-CC-DD-EE-FF") = aa:bb:cc:dd:ee:ff
	// Parse("aabb.ccdd.eeff") = aa:bb:cc:dd:ee:ff
	// Parse("AABBCCDDEEFF") = aa:bb:cc:dd:ee:ff
}

func ExampleAddr_FormatString() {
	addr := xmac.MustParse("aa:bb:cc:dd:ee:ff")

	fmt.Println("Colon:", addr.FormatString(xmac.FormatColon))
	fmt.Println("Dash:", addr.FormatString(xmac.FormatDash))
	fmt.Println("Dot:", addr.FormatString(xmac.FormatDot))
	fmt.Println("Bare:", addr.FormatString(xmac.FormatBare))
	fmt.Println("ColonUpper:", addr.FormatString(xmac.FormatColonUpper))
	fmt.Println("DashUpper:", addr.FormatString(xmac.FormatDashUpper))
	fmt.Println("DotUpper:", addr.FormatString(xmac.FormatDotUpper))
	fmt.Println("BareUpper:", addr.FormatString(xmac.FormatBareUpper))

	// Output:
	// Colon: aa:bb:cc:dd:ee:ff
	// Dash: aa-bb-cc-dd-ee-ff
	// Dot: aabb.ccdd.eeff
	// Bare: aabbccddeeff
	// ColonUpper: AA:BB:CC:DD:EE:FF
	// DashUpper: AA-BB-CC-DD-EE-FF
	// DotUpper: AABB.CCDD.EEFF
	// BareUpper: AABBCCDDEEFF
}

func ExampleAddr_IsUsable() {
	// 业务场景：验证 MAC 地址是否可用于资产识别

	testCases := []string{
		"aa:bb:cc:dd:ee:ff", // 正常地址
		"",                  // 空字符串
		"00:00:00:00:00:00", // 全零地址
		"ff:ff:ff:ff:ff:ff", // 广播地址
	}

	for _, s := range testCases {
		addr, err := xmac.Parse(s)
		if err != nil {
			fmt.Printf("%q: parse error\n", s)
			continue
		}

		if addr.IsUsable() {
			fmt.Printf("%q: usable\n", s)
		} else {
			fmt.Printf("%q: not usable\n", s)
		}
	}

	// Output:
	// "aa:bb:cc:dd:ee:ff": usable
	// "": parse error
	// "00:00:00:00:00:00": not usable
	// "ff:ff:ff:ff:ff:ff": not usable
}

func ExampleAddr_IsUnicast() {
	unicast := xmac.MustParse("00:11:22:33:44:55")
	multicast := xmac.MustParse("01:00:5e:00:00:01")

	fmt.Println("Unicast:", unicast.IsUnicast(), unicast.IsMulticast())
	fmt.Println("Multicast:", multicast.IsUnicast(), multicast.IsMulticast())

	// Output:
	// Unicast: true false
	// Multicast: false true
}

func ExampleAddr_MarshalJSON() {
	type Asset struct {
		ID  int       `json:"id"`
		MAC xmac.Addr `json:"mac"`
	}

	// 序列化
	asset := Asset{ID: 1, MAC: xmac.MustParse("aa:bb:cc:dd:ee:ff")}
	data, err := json.Marshal(asset)
	if err != nil {
		fmt.Println("Marshal error:", err)
		return
	}
	fmt.Println("Marshal:", string(data))

	// 反序列化
	var decoded Asset
	if err := json.Unmarshal(data, &decoded); err != nil {
		fmt.Println("Unmarshal error:", err)
		return
	}
	fmt.Println("Unmarshal:", decoded.MAC)

	// Output:
	// Marshal: {"id":1,"mac":"aa:bb:cc:dd:ee:ff"}
	// Unmarshal: aa:bb:cc:dd:ee:ff
}

func ExampleAddr_OUI() {
	addr := xmac.MustParse("00:1a:2b:3c:4d:5e")

	oui := addr.OUI()
	nic := addr.NIC()

	fmt.Printf("OUI: %02x:%02x:%02x\n", oui[0], oui[1], oui[2])
	fmt.Printf("NIC: %02x:%02x:%02x\n", nic[0], nic[1], nic[2])

	// Output:
	// OUI: 00:1a:2b
	// NIC: 3c:4d:5e
}

func ExampleAddr_Next() {
	addr := xmac.MustParse("00:00:00:00:00:fe")

	for range 3 {
		fmt.Println(addr)
		next, err := addr.Next()
		if err != nil {
			fmt.Println("overflow!")
			break
		}
		addr = next
	}

	// Output:
	// 00:00:00:00:00:fe
	// 00:00:00:00:00:ff
	// 00:00:00:00:01:00
}
