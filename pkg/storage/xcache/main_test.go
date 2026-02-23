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
		// 设计决策: go-redis v9.17 的 tryDial 重连退避使用 time.Sleep(time.Second)
		// （pool.go:439），goroutine 暂停时栈顶为 time.Sleep，无法用 IgnoreTopFunction
		// 精确匹配 tryDial（上方已添加但不覆盖 Sleep 状态）。
		// 已知风险: 此规则过于宽泛，可能屏蔽非 go-redis 的 goroutine 泄漏。
		// 缓解措施: 上方已添加 go-redis 具体函数签名（tryDial, cleanupLoop），
		// 此规则仅覆盖 tryDial 内 time.Sleep 退避状态的 goroutine。
		// goleak 不支持基于调用栈的条件过滤（仅 IgnoreTopFunction/IgnoreAnyFunction）。
		// TODO: 当 go-redis 改用 context.WithTimeout 替代 time.Sleep 后，移除此规则。
		goleak.IgnoreTopFunction("time.Sleep"),
	)
}
