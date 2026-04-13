// Package xetcdtest 提供基于嵌入式 etcd 的测试辅助，语义对齐 xredismock：
// 启动进程内真实 etcd，返回真实 *clientv3.Client + cleanup。
// 适用：xetcd 客户端单测、Informer/Watch 集成测试、xelection/xdlock 选主测试、
// 依赖 etcd 的业务模块（调度 Pod allocator/leaderguard/routestore）。
//
// 已知限制：多个依赖本包的 package 并发 `go test`（`-p>1`）时，因 embed.Etcd
// 底层 WAL/端口初始化存在全局竞态，可能出现 "failed to create WAL" panic。
// 同一进程内 tryStart 已加 startMu + 重试缓解；跨进程建议用 `go test -p=1`
// 或将依赖嵌入式 etcd 的 package 纳入单独的 CI 阶段串行执行。
package xetcdtest

import (
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"runtime/debug"
	"sync"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/server/v3/embed"
	"go.uber.org/zap"
)

// removeDir 清理临时目录；失败仅记录，不中断调用方的主要错误路径。
func removeDir(dir string) {
	if err := os.RemoveAll(dir); err != nil {
		slog.Warn("xetcdtest: remove temp dir", "dir", dir, "err", err)
	}
}

// startTimeout 限制 embed.Etcd 启动等待；超时返回错误而非无限阻塞。
const startTimeout = 30 * time.Second

// maxStartAttempts 启动重试上限。`randomLocalURL()` 关闭探测 listener 到
// embed.StartEtcd 再次 Bind 之间存在竞态：并发测试（如 `go test -p=N`
// 多包同时启动嵌入式 etcd）可能抢占端口。`embed.StartEtcd` 在此情况下
// 会 panic（"failed to create WAL"），因此需 recover + 重试。
const maxStartAttempts = 5

// startMu 进程内串行化 embed.StartEtcd 的端口分配窗口，缩小同进程内并发
// 构造多实例时的竞态窗口。跨进程并发（多个 test binary 并行）仍需重试。
var startMu sync.Mutex

// Mock 封装一个进程内 embed.Etcd 实例及其客户端。
// 并发安全：Close 幂等。
type Mock struct {
	etcd   *embed.Etcd
	client *clientv3.Client
	dir    string

	mu     sync.Mutex
	closed bool
}

// New 启动一个监听随机端口的嵌入式 etcd，返回已连接的客户端。
// 数据目录使用临时目录，Close 时自动清理。
// 端口抢占导致启动失败会自动重试，重试次数上限 maxStartAttempts。
func New() (*Mock, error) {
	var lastErr error
	for attempt := 1; attempt <= maxStartAttempts; attempt++ {
		m, err := tryStart()
		if err == nil {
			return m, nil
		}
		lastErr = err
		// 指数退避等待下一次尝试，避开并发抢占窗口。
		time.Sleep(time.Duration(attempt) * 50 * time.Millisecond)
	}
	return nil, fmt.Errorf("xetcdtest: start after %d attempts: %w", maxStartAttempts, lastErr)
}

// tryStart 单次尝试启动嵌入式 etcd；失败（含 etcd 深层 panic）时清理资源返回错误。
func tryStart() (_ *Mock, retErr error) {
	dir, err := os.MkdirTemp("", "xetcdtest-")
	if err != nil {
		return nil, fmt.Errorf("xetcdtest: mkdir temp: %w", err)
	}

	startMu.Lock()
	unlocked := false
	unlock := func() {
		if !unlocked {
			startMu.Unlock()
			unlocked = true
		}
	}
	defer unlock()
	defer func() {
		if v := recover(); v != nil {
			removeDir(dir)
			// 附带堆栈，便于调试嵌入式 etcd 启动时深层 panic（如 WAL 竞态）。
			retErr = fmt.Errorf("xetcdtest: embed panic: %v\n%s", v, debug.Stack())
		}
	}()

	clientURL, err := randomLocalURL()
	if err != nil {
		removeDir(dir)
		return nil, err
	}
	peerURL, err := randomLocalURL()
	if err != nil {
		removeDir(dir)
		return nil, err
	}

	cfg := embed.NewConfig()
	cfg.Name = "xetcdtest"
	cfg.Dir = dir
	cfg.ListenClientUrls = []url.URL{*clientURL}
	cfg.AdvertiseClientUrls = []url.URL{*clientURL}
	cfg.ListenPeerUrls = []url.URL{*peerURL}
	cfg.AdvertisePeerUrls = []url.URL{*peerURL}
	cfg.InitialCluster = cfg.Name + "=" + peerURL.String()
	cfg.LogLevel = "error"
	cfg.Logger = "zap"
	cfg.ZapLoggerBuilder = embed.NewZapLoggerBuilder(zap.NewNop())

	e, err := embed.StartEtcd(cfg)
	unlock()
	if err != nil {
		removeDir(dir)
		return nil, fmt.Errorf("xetcdtest: start etcd: %w", err)
	}

	select {
	case <-e.Server.ReadyNotify():
	case <-time.After(startTimeout):
		e.Close()
		removeDir(dir)
		return nil, fmt.Errorf("xetcdtest: etcd not ready within %s", startTimeout)
	}

	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{clientURL.String()},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		e.Close()
		removeDir(dir)
		return nil, fmt.Errorf("xetcdtest: connect client: %w", err)
	}

	return &Mock{etcd: e, client: cli, dir: dir}, nil
}

// Client 返回连接到嵌入式 etcd 的客户端。
func (m *Mock) Client() *clientv3.Client { return m.client }

// Endpoints 返回客户端连接端点，便于传入依赖 endpoints 配置的上层模块。
func (m *Mock) Endpoints() []string { return m.client.Endpoints() }

// Etcd 返回底层 embed.Etcd，供需要高阶操作的测试使用（如触发 compaction）。
func (m *Mock) Etcd() *embed.Etcd { return m.etcd }

// Close 关闭客户端、停止 etcd、清理数据目录；幂等。
func (m *Mock) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return
	}
	m.closed = true

	if m.client != nil {
		if err := m.client.Close(); err != nil {
			slog.Warn("xetcdtest: close client", "err", err)
		}
	}
	if m.etcd != nil {
		m.etcd.Close()
	}
	if m.dir != "" {
		removeDir(m.dir)
	}
}

// NewClient 便捷构造：返回客户端和清理函数。
// 适合 `cli, cleanup, err := xetcdtest.NewClient(); defer cleanup()` 使用模式。
func NewClient() (*clientv3.Client, func(), error) {
	m, err := New()
	if err != nil {
		return nil, nil, err
	}
	return m.Client(), m.Close, nil
}

// randomLocalURL 通过监听 127.0.0.1:0 获取一个空闲端口，返回对应 http URL。
// 关闭 listener 后端口可能被其它进程抢占，这是 embed.Etcd 常见写法的已知竞态；
// 测试场景影响可忽略。
func randomLocalURL() (*url.URL, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("xetcdtest: pick port: %w", err)
	}
	addr := l.Addr().String()
	if err := l.Close(); err != nil {
		return nil, fmt.Errorf("xetcdtest: close probe listener: %w", err)
	}
	u, err := url.Parse("http://" + addr)
	if err != nil {
		return nil, fmt.Errorf("xetcdtest: parse url: %w", err)
	}
	return u, nil
}
