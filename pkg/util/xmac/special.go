package xmac

// 预定义的特殊 MAC 地址。
var (
	// Zero 是全零地址 00:00:00:00:00:00。
	// 通常表示"未知"或"无效"。
	// 注意：与零值 Addr{} 相同。
	Zero = Addr{}

	// Broadcast 是广播地址 ff:ff:ff:ff:ff:ff。
	// 用于向局域网内所有设备发送数据。
	Broadcast = Addr{bytes: [6]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}}
)

// IsSpecial 报告 a 是否为特殊地址（零地址或广播地址）。
// 特殊地址通常不应用于资产识别等业务场景。
func (a Addr) IsSpecial() bool {
	return a.IsZero() || a.IsBroadcast()
}

// IsUsable 报告 a 是否可用于业务场景。
// 可用条件：有效且非特殊（非零、非广播）。
// 这是资产识别等业务场景的推荐检查方法。
//
// 使用示例：
//
//	addr, err := xmac.Parse(macStr)
//	if err != nil || !addr.IsUsable() {
//	    // 跳过无效或不可用的 MAC
//	    return nil
//	}
func (a Addr) IsUsable() bool {
	return a.IsValid() && !a.IsSpecial()
}
