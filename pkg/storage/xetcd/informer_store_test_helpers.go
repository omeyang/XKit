package xetcd

// SetForTest 测试专用：直接写入 key=value，等价于 etcd 推送了一条
// 该 key 的事件。生产代码请使用 Informer 通过 etcd Watch 驱动 Store；
// 直接调用本方法只在以下场景适用：
//   - 单元测试：跳过 etcd 真实拉起，预置缓存初态。
//   - 模糊测试：以确定性输入构造特定缓存状态。
//
// 命名以 ForTest 后缀做硬约束，向调用方明示其测试边界属性，避免被
// 业务代码误用。函数本体仅是 set 的薄封装，无额外语义。
func (s *InformerStore) SetForTest(key string, value []byte, rev int64) {
	s.set(key, value, rev)
}

// RemoveForTest 测试专用：直接删除 key，等价于 etcd 推送了一条 DELETE
// 事件。语义、约束同 [InformerStore.SetForTest]。
func (s *InformerStore) RemoveForTest(key string, rev int64) {
	s.remove(key, rev)
}

// ReplaceForTest 测试专用：原子替换整个缓存。等价于一次 re-list 全量同步。
// 用于在测试中一次性切换 Store 状态，避免逐 key SetForTest 累积。
func (s *InformerStore) ReplaceForTest(items map[string][]byte, rev int64) {
	s.replace(items, rev)
}
