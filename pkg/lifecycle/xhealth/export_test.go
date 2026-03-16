package xhealth

// IsShutdown 返回 shutdown 状态（仅测试可用）。
func (h *Health) IsShutdown() bool {
	return h.shutdown.Load()
}

// CheckEntries 返回指定端点的检查项数量（仅测试可用）。
func (h *Health) CheckEntries(ep int) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.checks[ep])
}

// ReadyCh 返回 readyCh 通道（仅测试可用）。
func (h *Health) ReadyCh() <-chan struct{} {
	return h.readyCh
}
