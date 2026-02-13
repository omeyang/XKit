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
// 方式一：使用 Retryer（推荐用于需要接口抽象和自定义策略的场景）
//
//	r := xretry.NewRetryer(
//	    xretry.WithRetryPolicy(xretry.NewFixedRetry(3)),
//	    xretry.WithBackoffPolicy(xretry.NewExponentialBackoff()),
//	)
//	err := r.Do(ctx, func(ctx context.Context) error { ... })
//
// Retryer 的回调函数签名为 func(ctx context.Context) error，可直接感知 context。
// 如需 mock 重试执行器，可使用 Executor 接口作为函数参数类型。
//
// 方式二：直接使用 Do 函数（推荐用于简单场景）
//
//	err := xretry.Do(ctx, func() error { ... },
//	    xretry.Attempts(3), xretry.Delay(100*time.Millisecond))
//
// Do 函数的回调签名为 func() error，不接收 context 参数。
// 如需在回调中使用 context，通过闭包捕获即可。
//
// 注意：Do 函数使用 retry-go 默认延迟策略（含随机抖动）。
// 若需精确的零延迟重试，请同时设置 Delay(0) 和 MaxJitter(0)。
//
// # 选择指南
//
// 两种方式均使用 avast/retry-go/v5 作为底层引擎：
//   - Retryer: 通过 RetryPolicy/BackoffPolicy 接口抽象，支持自定义策略和 mock
//   - Do/DoWithData: 直接暴露 retry-go 选项，API 更简洁，适合一次性使用
//
// # 错误分类
//
//   - NewPermanentError(err)：标记为永久性错误（不应重试）
//   - NewTemporaryError(err)：标记为临时性错误（应该重试）
//   - Unrecoverable(err)：retry-go 风格的不可恢复错误
//   - context.Canceled / context.DeadlineExceeded：默认不可重试（fail-fast）
//
// 错误检查函数：
//   - IsRetryable(err)：是否可重试（context 取消返回 false）
//   - IsPermanent(err)：是否为显式标记的永久性错误（仅 PermanentError 或
//     Retryable()==false 的自定义类型；context 取消返回 false）
//
// 详细用法参见各函数文档和 example_test.go。
//
// # 性能
//
// 退避策略使用 crypto/rand 生成抖动随机数，确保安全随机性。
// 单次 NextDelay 调用耗时约 50-100ns，对于重试场景（通常每秒最多几次）
// 此性能开销完全可接受。如需禁用抖动以获得确定性行为，可使用 WithJitter(0)。
//
// Retryer.Do 每次调用会重建 retry-go 选项切片（约 440 B/op, 13 allocs/op），
// 对于重试场景此开销完全可接受。
//
// [avast/retry-go/v5]: https://github.com/avast/retry-go
package xretry
