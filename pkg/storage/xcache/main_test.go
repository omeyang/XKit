package xcache

import (
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m,
		// go-redis v9.17 内部 goroutine：连接池 tryDial 和 circuit breaker cleanupLoop
		goleak.IgnoreTopFunction("github.com/redis/go-redis/v9/internal/pool.(*ConnPool).tryDial"),
		goleak.IgnoreTopFunction("github.com/redis/go-redis/v9/maintnotifications.(*CircuitBreakerManager).cleanupLoop"),
		// 设计决策: go-redis v9.17 的部分内部 goroutine（如连接池健康检查、重连退避）
		// 在 Client.Close() 后可能仍处于 time.Sleep 状态，导致 goleak 误报。
		// 已知风险: time.Sleep 规则过于宽泛，可能屏蔽非 go-redis 的 goroutine 泄漏。
		// 缓解措施: 上方已添加 go-redis 具体函数签名（tryDial, cleanupLoop），
		// 此规则仅覆盖无法精确匹配的退避重连 goroutine。
		// TODO: 当 go-redis 提供更精确的 goroutine 清理机制后，移除此规则。
		goleak.IgnoreTopFunction("time.Sleep"),
	)
}
