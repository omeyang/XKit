package xkeylock

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/cespare/xxhash/v2"
)

// keyLockImpl 是 KeyLock 的分片实现。
type keyLockImpl struct {
	shards   []shard
	mask     uint64
	opts     *options
	closed   atomic.Bool
	keyCount atomic.Int64
	done     chan struct{}
}

type shard struct {
	mu      sync.Mutex
	entries map[string]*lockEntry
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

func newKeyLockImpl(opts *options) *keyLockImpl {
	shards := make([]shard, opts.shardCount)
	for i := range shards {
		shards[i].entries = make(map[string]*lockEntry)
	}
	// shardCount 为 uint 类型且已验证为 2 的幂，uint → uint64 为安全宽化。
	mask := uint64(opts.shardCount - 1)
	return &keyLockImpl{
		shards: shards,
		mask:   mask,
		opts:   opts,
		done:   make(chan struct{}),
	}
}

func (kl *keyLockImpl) getShard(key string) *shard {
	h := xxhash.Sum64String(key)
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
		panic("xkeylock: nil Context")
	}
	// 快速检查：ctx 已取消时避免进入 getOrCreate 造成不必要的锁竞争。
	if err := ctx.Err(); err != nil {
		return nil, err
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
		return &handle{kl: kl, key: key, entry: entry}, nil
	case <-ctx.Done(): // 超时或取消
		kl.releaseRef(key, entry)
		return nil, ctx.Err()
	case <-kl.done: // KeyLock 已关闭
		kl.releaseRef(key, entry)
		return nil, ErrClosed
	}
}

func (kl *keyLockImpl) TryAcquire(key string) (Handle, error) {
	if kl.closed.Load() {
		return nil, ErrClosed
	}
	entry, err := kl.getOrCreate(key)
	if err != nil {
		return nil, err
	}
	select {
	case entry.ch <- struct{}{}: // 获取成功
		return &handle{kl: kl, key: key, entry: entry}, nil
	default: // 锁被占用
		kl.releaseRef(key, entry)
		return nil, nil
	}
}

func (kl *keyLockImpl) Len() int {
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
	return nil
}

func (h *handle) Key() string {
	return h.key
}

// 编译期接口检查。
var (
	_ KeyLock = (*keyLockImpl)(nil)
	_ Handle  = (*handle)(nil)
)
