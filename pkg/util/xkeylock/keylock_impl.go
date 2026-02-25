package xkeylock

import (
	"context"
	"hash/maphash"
	"sync"
	"sync/atomic"
	"unsafe"
)

// hashSeed 是分片哈希的种子，进程级别唯一。
// 分片选择不需要跨进程确定性，maphash 足以胜任。
var hashSeed = maphash.MakeSeed()

// keyLockImpl 是 Locker 的分片实现。
type keyLockImpl struct {
	shards   []shard
	mask     uint64
	opts     options
	closed   atomic.Bool
	keyCount atomic.Int64
	done     chan struct{}

	// testHookAfterTryAcquireSend 在 TryAcquire 的 channel 发送成功后、closed 检查前调用。
	// 仅供测试使用，生产环境为 nil（零开销）。实例级字段避免并行测试的数据竞争。
	testHookAfterTryAcquireSend func()

	// testHookAfterDefaultReleaseRef 在 TryAcquire default 分支 releaseRef 后、closed 检查前调用。
	// 仅供测试使用，与 testHookAfterTryAcquireSend 配合完整覆盖 TryAcquire 的两条竞态分支。
	testHookAfterDefaultReleaseRef func()
}

// cacheLineSize 是目标架构的缓存行大小（字节）。
const cacheLineSize = 64

// shardPayload 包含 shard 的业务字段。独立类型使 unsafe.Sizeof 可用于
// 自动计算 cache line padding，消除硬编码字节数和跨架构兼容性问题。
type shardPayload struct {
	mu      sync.Mutex
	entries map[string]*lockEntry
}

type shard struct {
	shardPayload
	// 设计决策: 填充至 cache line 边界消除相邻 shard 之间的伪共享。
	// 使用 unsafe.Sizeof(shardPayload{}) 自动计算填充字节数，
	// 适配不同架构（amd64: 16B payload → 48B padding; 386: 12B → 52B）。
	// 编译期安全：若 shardPayload 增长超过 cacheLineSize，数组长度变负，编译即失败。
	_ [cacheLineSize - unsafe.Sizeof(shardPayload{})]byte // cache line padding
}

// lockEntry 表示一个 key 的锁条目。
// ch 是 size=1 的 channel，用作互斥量：
//   - 发送成功 = 获取锁
//   - 发送阻塞 = 锁被占用
//   - 接收 = 释放锁
type lockEntry struct {
	ch chan struct{}
	// refcnt 跟踪引用此条目的 goroutine 数量（持有者 + 等待者）。
	// 归零时条目从 map 中删除。
	refcnt atomic.Int32
}

// handle 实现 Handle 接口。
type handle struct {
	kl    *keyLockImpl
	key   string
	entry *lockEntry
	done  atomic.Bool
}

func newKeyLockImpl(opts options) *keyLockImpl {
	shards := make([]shard, opts.shardCount)
	for i := range shards {
		shards[i].entries = make(map[string]*lockEntry)
	}
	return &keyLockImpl{
		shards: shards,
		mask:   opts.shardMask, // validate() 已计算
		opts:   opts,
		done:   make(chan struct{}),
	}
}

func (kl *keyLockImpl) getShard(key string) *shard {
	h := maphash.String(hashSeed, key)
	return &kl.shards[h&kl.mask]
}

// getOrCreate 获取或创建 lockEntry，并增加引用计数。
func (kl *keyLockImpl) getOrCreate(key string) (*lockEntry, error) {
	s := kl.getShard(key)
	s.mu.Lock()
	defer s.mu.Unlock()

	if kl.closed.Load() {
		return nil, ErrClosed
	}

	e, ok := s.entries[key]
	if !ok {
		if kl.opts.maxKeys > 0 {
			// 使用 CAS 严格限制 key 数量，避免跨分片并发突破上限。
			for {
				cur := kl.keyCount.Load()
				if cur >= int64(kl.opts.maxKeys) {
					return nil, ErrMaxKeysExceeded
				}
				if kl.keyCount.CompareAndSwap(cur, cur+1) {
					break
				}
			}
		} else {
			kl.keyCount.Add(1)
		}
		e = &lockEntry{ch: make(chan struct{}, 1)}
		s.entries[key] = e
	}
	e.refcnt.Add(1)
	return e, nil
}

// releaseRef 减少引用计数，归零时从 map 删除。
func (kl *keyLockImpl) releaseRef(key string, entry *lockEntry) {
	s := kl.getShard(key)
	s.mu.Lock()
	defer s.mu.Unlock()

	if entry.refcnt.Add(-1) == 0 {
		delete(s.entries, key)
		kl.keyCount.Add(-1)
	}
}

func (kl *keyLockImpl) Acquire(ctx context.Context, key string) (Handle, error) {
	if ctx == nil {
		return nil, ErrNilContext
	}
	if key == "" {
		return nil, ErrInvalidKey
	}
	// 快速检查：已关闭时优先返回 ErrClosed，语义优先级高于 ctx 取消。
	if kl.closed.Load() {
		return nil, ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	entry, err := kl.getOrCreate(key)
	if err != nil {
		return nil, err
	}
	// 设计决策: 当 ctx.Done() 和 kl.done 同时就绪时，Go select 随机选取分支，
	// 因此阻塞路径下 ErrClosed 与 ctx.Err() 的返回不可控。快速路径（上方）
	// 保证 ErrClosed 优先，但阻塞等待后两者等价——调用方应同时处理这两种错误。
	select {
	case entry.ch <- struct{}{}: // 获取成功
		// 二次检查：封住 getOrCreate→select 之间的 Close 竞态窗口。
		// Close 设置 closed 后才关闭 done channel，所以此处 Load 能可靠观测到。
		if kl.closed.Load() {
			<-entry.ch
			kl.releaseRef(key, entry)
			return nil, ErrClosed
		}
		return &handle{kl: kl, key: key, entry: entry}, nil
	case <-ctx.Done(): // 超时或取消
		kl.releaseRef(key, entry)
		return nil, ctx.Err()
	case <-kl.done: // Locker 已关闭
		kl.releaseRef(key, entry)
		return nil, ErrClosed
	}
}

func (kl *keyLockImpl) TryAcquire(key string) (Handle, error) {
	if key == "" {
		return nil, ErrInvalidKey
	}
	if kl.closed.Load() {
		return nil, ErrClosed
	}
	entry, err := kl.getOrCreate(key)
	if err != nil {
		return nil, err
	}
	select {
	case entry.ch <- struct{}{}: // 获取成功
		if kl.testHookAfterTryAcquireSend != nil {
			kl.testHookAfterTryAcquireSend()
		}
		if kl.closed.Load() {
			<-entry.ch
			kl.releaseRef(key, entry)
			return nil, ErrClosed
		}
		return &handle{kl: kl, key: key, entry: entry}, nil
	default: // 锁被占用
		kl.releaseRef(key, entry)
		if kl.testHookAfterDefaultReleaseRef != nil {
			kl.testHookAfterDefaultReleaseRef()
		}
		// 二次检查：封住 getOrCreate→select 之间的 Close 竞态窗口。
		// 避免将"已关闭"误报为"锁被占用"。
		if kl.closed.Load() {
			return nil, ErrClosed
		}
		return nil, ErrLockOccupied
	}
}

func (kl *keyLockImpl) Len() int {
	// 防御性下界：正常情况下 keyCount 不会为负，
	// 但在 getOrCreate/releaseRef 并发路径中使用 max 作为安全网。
	return int(max(kl.keyCount.Load(), 0))
}

func (kl *keyLockImpl) Keys() []string {
	keys := make([]string, 0, max(kl.keyCount.Load(), 0))
	for i := range kl.shards {
		s := &kl.shards[i]
		s.mu.Lock()
		for k := range s.entries {
			keys = append(keys, k)
		}
		s.mu.Unlock()
	}
	return keys
}

func (kl *keyLockImpl) Close() error {
	if !kl.closed.CompareAndSwap(false, true) {
		return ErrClosed
	}
	close(kl.done)
	return nil
}

// handle 方法

func (h *handle) Unlock() error {
	if !h.done.CompareAndSwap(false, true) {
		return ErrLockNotHeld
	}
	<-h.entry.ch
	h.kl.releaseRef(h.key, h.entry)
	// 释放引用，防止长期持有 Handle 时阻止 GC 回收 keyLockImpl。
	h.kl = nil
	h.entry = nil
	return nil
}

func (h *handle) Key() string {
	return h.key
}

// 编译期接口检查。
var (
	_ Locker = (*keyLockImpl)(nil)
	_ Handle = (*handle)(nil)
)
