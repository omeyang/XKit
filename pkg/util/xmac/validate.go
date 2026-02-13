package xmac

// IsUnicast 报告 a 是否为单播地址。
// 单播地址的第一字节最低位（bit 0）为 0。
// 无效地址返回 false。
func (a Addr) IsUnicast() bool {
	return a.IsValid() && (a.bytes[0]&0x01) == 0
}

// IsMulticast 报告 a 是否为多播地址。
// 多播地址的第一字节最低位（bit 0）为 1。
// 广播地址也是一种特殊的多播地址。
// 无效地址返回 false。
func (a Addr) IsMulticast() bool {
	return a.IsValid() && (a.bytes[0]&0x01) == 1
}

// IsBroadcast 报告 a 是否为广播地址（ff:ff:ff:ff:ff:ff）。
func (a Addr) IsBroadcast() bool {
	return a == broadcastAddr()
}

// IsLocallyAdministered 报告 a 是否为本地管理地址（LAA）。
// LAA 的第一字节次低位（bit 1）为 1。
// 虚拟机、容器等通常使用 LAA。
// 无效地址返回 false。
func (a Addr) IsLocallyAdministered() bool {
	return a.IsValid() && (a.bytes[0]&0x02) == 0x02
}

// IsUniversallyAdministered 报告 a 是否为全球唯一地址（UAA）。
// UAA 的第一字节次低位（bit 1）为 0。
// 物理网卡出厂时分配的地址通常是 UAA。
// 无效地址返回 false。
func (a Addr) IsUniversallyAdministered() bool {
	return a.IsValid() && (a.bytes[0]&0x02) == 0
}

// IsZero 报告 a 是否为全零地址（00:00:00:00:00:00）。
// 注意：全零地址与无效地址（零值 Addr{}）相同。
func (a Addr) IsZero() bool {
	return a == Addr{}
}

// OUI 返回组织唯一标识符（Organizationally Unique Identifier）。
// OUI 是 MAC 地址的前 3 字节，由 IEEE 分配给设备制造商。
// 无效地址返回零值 [3]byte{}。
func (a Addr) OUI() [3]byte {
	if !a.IsValid() {
		return [3]byte{}
	}
	return [3]byte{a.bytes[0], a.bytes[1], a.bytes[2]}
}

// NIC 返回网络接口控制器标识（Network Interface Controller specific）。
// NIC 是 MAC 地址的后 3 字节，由制造商分配。
// 无效地址返回零值 [3]byte{}。
func (a Addr) NIC() [3]byte {
	if !a.IsValid() {
		return [3]byte{}
	}
	return [3]byte{a.bytes[3], a.bytes[4], a.bytes[5]}
}
