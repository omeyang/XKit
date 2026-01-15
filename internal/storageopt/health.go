package storageopt

import (
	"context"
	"time"
)

// 健康检查相关常量。
const (
	// DefaultHealthTimeout 默认健康检查超时时间。
	// 用于 xmongo、xclickhouse、xetcd 等包。
	DefaultHealthTimeout = 5 * time.Second
)

// HealthContext 创建带健康检查超时的 context。
// 如果 timeout <= 0，返回原始 context 和空的 cancel 函数。
//
// 使用示例：
//
//	ctx, cancel := storageopt.HealthContext(ctx, timeout)
//	defer cancel()
func HealthContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, timeout)
}
