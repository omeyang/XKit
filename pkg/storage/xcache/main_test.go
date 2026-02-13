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
		goleak.IgnoreTopFunction("time.Sleep"),
	)
}
