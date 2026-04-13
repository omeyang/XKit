package xetcd

// 测试辅助：暴露 InformerStore 的内部写方法，仅供本包测试使用。
// 生产代码通过 Informer.Run 间接驱动 set/remove/replace。

// SetForTest 直接设置缓存项。
func (s *InformerStore) SetForTest(key string, value []byte, rev int64) { s.set(key, value, rev) }

// RemoveForTest 直接删除缓存项。
func (s *InformerStore) RemoveForTest(key string, rev int64) { s.remove(key, rev) }
