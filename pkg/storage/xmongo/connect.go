package xmongo

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// cleanupDisconnectTimeout 为 Connect 失败清理分支的独立超时。
// 故意与调用方 ctx 解耦：caller 的 ctx 可能已被取消，但连接池必须被可靠释放。
const cleanupDisconnectTimeout = 5 * time.Second

// Connect 是一个便捷函数，从 URI 创建 Mongo 实例。
//
// 用户通过 configFn 回调直接操作 mongo driver 原生的 options.ClientOptions，
// xmongo 不做任何截流或重复包装。configFn 为 nil 时使用默认配置。
//
// opts 参数仅控制 xmongo 增值功能（慢查询检测、超时兜底、OTel 等），
// 与 New() 的 opts 完全一致。
//
// 用法:
//
//	m, err := xmongo.Connect(ctx, "mongodb://localhost:27017",
//	    func(opts *options.ClientOptions) {
//	        opts.SetCompressors([]string{"zstd", "snappy"})
//	        opts.SetMaxPoolSize(50)
//	    },
//	    xmongo.WithSlowQueryThreshold(200*time.Millisecond),
//	    xmongo.WithObserver(observer),
//	)
//
// 设计决策: ctx 参数当前仅用于 nil 守卫（mongo.Connect v2 不接受 context）。
// 保留 ctx 以与项目惯例一致，便于未来扩展（如初始 Ping 检查）。
func Connect(ctx context.Context, uri string, configFn func(*options.ClientOptions), opts ...Option) (Mongo, error) {
	if ctx == nil {
		return nil, ErrNilContext
	}
	if uri = strings.TrimSpace(uri); uri == "" {
		return nil, ErrEmptyURI
	}

	clientOpts := options.Client().ApplyURI(uri)
	if configFn != nil {
		configFn(clientOpts)
	}

	client, err := mongo.Connect(clientOpts)
	if err != nil {
		return nil, fmt.Errorf("xmongo connect: %w", err)
	}

	// New() 失败时必须断开底层 client，避免泄漏已建立的连接池。
	// 如 SlowQueryDetector 创建失败（参数越界）会走到这里。
	//
	// 设计决策: 清理分支使用独立的 context.Background()+超时，
	// 而非复用 caller ctx。caller ctx 可能已取消（如 Connect 调用处已有超时），
	// 此时若透传给 Disconnect，驱动会提前中止清理，导致底层连接池泄漏。
	m, err := New(client, opts...)
	if err != nil {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), cleanupDisconnectTimeout)
		defer cancel()
		//nolint:errcheck,gosec // cleanup path: 原始错误优先于清理错误
		client.Disconnect(cleanupCtx)
		return nil, err
	}
	return m, nil
}
