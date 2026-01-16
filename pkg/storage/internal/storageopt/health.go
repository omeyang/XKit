package storageopt

import "time"

// 健康检查相关常量。
const (
	// DefaultHealthTimeout 默认健康检查超时时间。
	// 用于 xmongo、xclickhouse、xetcd 等包。
	DefaultHealthTimeout = 5 * time.Second
)
