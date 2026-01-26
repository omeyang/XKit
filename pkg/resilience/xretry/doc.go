// Package xretry 提供通用的重试策略和退避策略接口及实现。
//
// # 设计理念
//
// xretry 采用接口驱动设计：
//   - RetryPolicy：定义是否应该重试
//   - BackoffPolicy：定义重试间隔时间
//
// 底层使用 [avast/retry-go/v5] 实现重试逻辑。
//
// # 重试策略
//
// 内置三种重试策略：
//   - FixedRetryPolicy：固定次数重试
//   - AlwaysRetryPolicy：无限重试（慎用）
//   - NeverRetryPolicy：永不重试
//
// # 退避策略
//
// 内置四种退避策略：
//   - FixedBackoff：固定延迟
//   - ExponentialBackoff：指数退避（带抖动）
//   - LinearBackoff：线性退避
//   - NoBackoff：无延迟
//
// # 使用方式
//
// 方式一：使用 Retryer（推荐用于需要接口抽象的场景）
//
//	retryer := xretry.NewRetryer(
//	    xretry.WithRetryPolicy(xretry.NewFixedRetry(3)),
//	    xretry.WithBackoffPolicy(xretry.NewExponentialBackoff()),
//	)
//	err := retryer.Do(ctx, func(ctx context.Context) error {
//	    return doSomething()
//	})
//
// 方式二：直接使用 retry-go 风格（推荐用于简单场景）
//
//	err := xretry.Do(ctx, func() error {
//	    return doSomething()
//	}, xretry.Attempts(3), xretry.Delay(100*time.Millisecond))
//
// # 错误分类
//
//   - NewPermanentError(err)：标记为永久性错误（不应重试）
//   - NewTemporaryError(err)：标记为临时性错误（应该重试）
//   - Unrecoverable(err)：retry-go 风格的不可恢复错误
//
// 详细用法参见各函数文档和 example_test.go。
//
// # 性能
//
// 退避策略使用 crypto/rand 生成抖动随机数，确保安全随机性。
// 单次 NextDelay 调用耗时约 50-100ns，对于重试场景（通常每秒最多几次）
// 此性能开销完全可接受。如需禁用抖动以获得确定性行为，可使用 WithJitter(0)。
//
// [avast/retry-go/v5]: https://github.com/avast/retry-go
package xretry
