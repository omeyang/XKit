package xetcd

import "sync"

// InformerStore 线程安全的 etcd key-value 本地缓存。
// 所有读方法返回数据拷贝，调用方可安全修改返回值。
type InformerStore struct {
	mu    sync.RWMutex
	items map[string][]byte
	rev   int64 // 最后同步的 etcd revision
}

// newInformerStore 构造空 Store。
func newInformerStore() *InformerStore {
	return &InformerStore{items: make(map[string][]byte)}
}

// NewInformerStore 导出构造器，便于测试与外部组合场景。
func NewInformerStore() *InformerStore { return newInformerStore() }

// Get 读取 key 对应值。不存在时 ok=false。返回独立拷贝。
func (s *InformerStore) Get(key string) ([]byte, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.items[key]
	if !ok {
		return nil, false
	}
	cp := make([]byte, len(v))
	copy(cp, v)
	return cp, true
}

// List 返回所有缓存项的独立拷贝。
func (s *InformerStore) List() map[string][]byte {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cp := make(map[string][]byte, len(s.items))
	for k, v := range s.items {
		val := make([]byte, len(v))
		copy(val, v)
		cp[k] = val
	}
	return cp
}

// Keys 返回所有缓存 key。
func (s *InformerStore) Keys() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	keys := make([]string, 0, len(s.items))
	for k := range s.items {
		keys = append(keys, k)
	}
	return keys
}

// Len 返回缓存项数量。
func (s *InformerStore) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.items)
}

// Rev 返回最后同步的 etcd revision。
func (s *InformerStore) Rev() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.rev
}

// set 写入单个 key（Informer 内部使用）。
// 设计决策：拷贝入参 value，避免调用方持有的底层数组（如 etcd 事件复用的 buffer
// 或 Handler 接收到的 slice）被外部修改后污染 Store 的只读视图契约。
func (s *InformerStore) set(key string, value []byte, rev int64) {
	cp := make([]byte, len(value))
	copy(cp, value)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[key] = cp
	if rev > s.rev {
		s.rev = rev
	}
}

// remove 删除单个 key（Informer 内部使用）。
func (s *InformerStore) remove(key string, rev int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, key)
	if rev > s.rev {
		s.rev = rev
	}
}

// replace 原子替换全部缓存内容（re-list 使用）。
// 设计决策：对 value 做深拷贝，防止调用方传入的 map（如 Informer.list 构造的临时 map）
// 底层字节数组被后续事件复用或外部修改。
func (s *InformerStore) replace(items map[string][]byte, rev int64) {
	cp := make(map[string][]byte, len(items))
	for k, v := range items {
		val := make([]byte, len(v))
		copy(val, v)
		cp[k] = val
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = cp
	s.rev = rev
}

