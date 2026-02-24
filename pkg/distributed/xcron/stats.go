package xcron

import (
	"sync"
	"sync/atomic"
	"time"
)

// Stats 提供任务执行统计信息。
//
// 线程安全，可在任务执行期间安全读取。
// 统计数据包括执行次数、成功/失败次数、执行时长等。
//
// 用法：
//
//	scheduler := xcron.New()
//	scheduler.AddFunc("@every 1m", task, xcron.WithName("my-task"))
//	scheduler.Start()
//
//	// 获取统计信息
//	stats := scheduler.Stats()
//	fmt.Printf("总执行次数: %d\n", stats.TotalExecutions())
//	fmt.Printf("成功次数: %d\n", stats.SuccessCount())
//	fmt.Printf("失败次数: %d\n", stats.FailureCount())
type Stats struct {
	totalExecutions atomic.Int64
	successCount    atomic.Int64
	failureCount    atomic.Int64
	skipCount       atomic.Int64 // 因锁获取失败跳过的次数

	mu           sync.RWMutex
	lastExecTime time.Time     // 最后执行时间
	lastDuration time.Duration // 最后执行耗时
	lastError    error         // 最后一次错误

	// 执行时长统计
	totalDuration atomic.Int64 // 纳秒
	minDuration   atomic.Int64 // 纳秒
	maxDuration   atomic.Int64 // 纳秒

	// 每个任务的统计
	jobStats sync.Map // map[string]*JobStats
}

// JobStats 单个任务的执行统计。
// 设计决策: 字段与 Stats 高度重复，但不抽取公共基类。
// 两者职责不同（全局聚合 vs 单任务粒度），且 Stats 使用 sync.Map 管理 JobStats，
// 抽象反而增加间接层次，降低可读性。
type JobStats struct {
	Name            string
	totalExecutions atomic.Int64
	successCount    atomic.Int64
	failureCount    atomic.Int64
	skipCount       atomic.Int64

	mu           sync.RWMutex
	lastExecTime time.Time
	lastDuration time.Duration
	lastError    error

	totalDuration atomic.Int64
	minDuration   atomic.Int64
	maxDuration   atomic.Int64
}

// newStats 创建新的统计实例。
func newStats() *Stats {
	s := &Stats{}
	// 初始化最小值为最大值，以便首次执行时正确更新
	s.minDuration.Store(int64(1<<63 - 1))
	return s
}

// TotalExecutions 返回总执行次数。
func (s *Stats) TotalExecutions() int64 {
	return s.totalExecutions.Load()
}

// SuccessCount 返回成功执行次数。
func (s *Stats) SuccessCount() int64 {
	return s.successCount.Load()
}

// FailureCount 返回失败执行次数。
func (s *Stats) FailureCount() int64 {
	return s.failureCount.Load()
}

// SkipCount 返回因锁获取失败跳过的次数。
func (s *Stats) SkipCount() int64 {
	return s.skipCount.Load()
}

// LastExecTime 返回最后一次执行时间。
func (s *Stats) LastExecTime() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastExecTime
}

// LastDuration 返回最后一次执行耗时。
func (s *Stats) LastDuration() time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastDuration
}

// LastError 返回最后一次执行错误（nil 表示成功）。
func (s *Stats) LastError() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastError
}

// AvgDuration 返回平均执行耗时。
func (s *Stats) AvgDuration() time.Duration {
	total := s.totalExecutions.Load()
	if total == 0 {
		return 0
	}
	return time.Duration(s.totalDuration.Load() / total)
}

// MinDuration 返回最小执行耗时。
func (s *Stats) MinDuration() time.Duration {
	min := s.minDuration.Load()
	if min == int64(1<<63-1) {
		return 0 // 尚未执行
	}
	return time.Duration(min)
}

// MaxDuration 返回最大执行耗时。
func (s *Stats) MaxDuration() time.Duration {
	return time.Duration(s.maxDuration.Load())
}

// SuccessRate 返回成功率（0-1）。
func (s *Stats) SuccessRate() float64 {
	total := s.totalExecutions.Load()
	if total == 0 {
		return 0
	}
	return float64(s.successCount.Load()) / float64(total)
}

// JobStats 返回指定任务的统计信息。
func (s *Stats) JobStats(name string) *JobStats {
	if v, ok := s.jobStats.Load(name); ok {
		if js, ok := v.(*JobStats); ok {
			return js
		}
	}
	return nil
}

// AllJobStats 返回所有任务的统计信息。
func (s *Stats) AllJobStats() map[string]*JobStats {
	result := make(map[string]*JobStats)
	s.jobStats.Range(func(key, value any) bool {
		if name, ok := key.(string); ok {
			if js, ok := value.(*JobStats); ok {
				result[name] = js
			}
		}
		return true
	})
	return result
}

// recordExecution 记录一次执行。
func (s *Stats) recordExecution(name string, duration time.Duration, err error) {
	now := time.Now()
	durationNs := int64(duration)

	// 更新全局统计
	s.totalExecutions.Add(1)
	s.totalDuration.Add(durationNs)

	if err != nil {
		s.failureCount.Add(1)
	} else {
		s.successCount.Add(1)
	}

	// 更新最小值（CAS 循环）
	for {
		old := s.minDuration.Load()
		if durationNs >= old {
			break
		}
		if s.minDuration.CompareAndSwap(old, durationNs) {
			break
		}
	}

	// 更新最大值（CAS 循环）
	for {
		old := s.maxDuration.Load()
		if durationNs <= old {
			break
		}
		if s.maxDuration.CompareAndSwap(old, durationNs) {
			break
		}
	}

	s.mu.Lock()
	s.lastExecTime = now
	s.lastDuration = duration
	s.lastError = err
	s.mu.Unlock()

	// 更新任务级统计
	if name != "" {
		js := s.getOrCreateJobStats(name)
		js.recordExecution(duration, err)
	}
}

// recordSkip 记录一次跳过（锁获取失败）。
func (s *Stats) recordSkip(name string) {
	s.skipCount.Add(1)

	if name != "" {
		js := s.getOrCreateJobStats(name)
		js.skipCount.Add(1)
	}
}

// getOrCreateJobStats 获取或创建任务统计。
func (s *Stats) getOrCreateJobStats(name string) *JobStats {
	if v, ok := s.jobStats.Load(name); ok {
		if js, ok := v.(*JobStats); ok {
			return js
		}
	}

	js := &JobStats{Name: name}
	js.minDuration.Store(int64(1<<63 - 1))

	actual, _ := s.jobStats.LoadOrStore(name, js)
	if result, ok := actual.(*JobStats); ok {
		return result
	}
	// 理论上不会走到这里，但返回新创建的 js 作为 fallback
	return js
}

// JobStats 方法

// TotalExecutions 返回任务总执行次数。
func (js *JobStats) TotalExecutions() int64 {
	return js.totalExecutions.Load()
}

// SuccessCount 返回任务成功执行次数。
func (js *JobStats) SuccessCount() int64 {
	return js.successCount.Load()
}

// FailureCount 返回任务失败执行次数。
func (js *JobStats) FailureCount() int64 {
	return js.failureCount.Load()
}

// SkipCount 返回任务跳过次数。
func (js *JobStats) SkipCount() int64 {
	return js.skipCount.Load()
}

// LastExecTime 返回任务最后执行时间。
func (js *JobStats) LastExecTime() time.Time {
	js.mu.RLock()
	defer js.mu.RUnlock()
	return js.lastExecTime
}

// LastDuration 返回任务最后执行耗时。
func (js *JobStats) LastDuration() time.Duration {
	js.mu.RLock()
	defer js.mu.RUnlock()
	return js.lastDuration
}

// LastError 返回任务最后执行错误。
func (js *JobStats) LastError() error {
	js.mu.RLock()
	defer js.mu.RUnlock()
	return js.lastError
}

// AvgDuration 返回任务平均执行耗时。
func (js *JobStats) AvgDuration() time.Duration {
	total := js.totalExecutions.Load()
	if total == 0 {
		return 0
	}
	return time.Duration(js.totalDuration.Load() / total)
}

// MinDuration 返回任务最小执行耗时。
func (js *JobStats) MinDuration() time.Duration {
	min := js.minDuration.Load()
	if min == int64(1<<63-1) {
		return 0
	}
	return time.Duration(min)
}

// MaxDuration 返回任务最大执行耗时。
func (js *JobStats) MaxDuration() time.Duration {
	return time.Duration(js.maxDuration.Load())
}

// SuccessRate 返回任务成功率。
func (js *JobStats) SuccessRate() float64 {
	total := js.totalExecutions.Load()
	if total == 0 {
		return 0
	}
	return float64(js.successCount.Load()) / float64(total)
}

// recordExecution 记录任务执行。
func (js *JobStats) recordExecution(duration time.Duration, err error) {
	now := time.Now()
	durationNs := int64(duration)

	js.totalExecutions.Add(1)
	js.totalDuration.Add(durationNs)

	if err != nil {
		js.failureCount.Add(1)
	} else {
		js.successCount.Add(1)
	}

	// 更新最小值
	for {
		old := js.minDuration.Load()
		if durationNs >= old {
			break
		}
		if js.minDuration.CompareAndSwap(old, durationNs) {
			break
		}
	}

	// 更新最大值
	for {
		old := js.maxDuration.Load()
		if durationNs <= old {
			break
		}
		if js.maxDuration.CompareAndSwap(old, durationNs) {
			break
		}
	}

	js.mu.Lock()
	js.lastExecTime = now
	js.lastDuration = duration
	js.lastError = err
	js.mu.Unlock()
}

// StatsSnapshot 统计快照，用于序列化。
type StatsSnapshot struct {
	TotalExecutions int64                        `json:"total_executions"`
	SuccessCount    int64                        `json:"success_count"`
	FailureCount    int64                        `json:"failure_count"`
	SkipCount       int64                        `json:"skip_count"`
	SuccessRate     float64                      `json:"success_rate"`
	LastExecTime    time.Time                    `json:"last_exec_time,omitempty"`
	LastDuration    time.Duration                `json:"last_duration"`
	LastError       string                       `json:"last_error,omitempty"`
	AvgDuration     time.Duration                `json:"avg_duration"`
	MinDuration     time.Duration                `json:"min_duration"`
	MaxDuration     time.Duration                `json:"max_duration"`
	Jobs            map[string]*JobStatsSnapshot `json:"jobs,omitempty"`
}

// JobStatsSnapshot 任务统计快照。
type JobStatsSnapshot struct {
	Name            string        `json:"name"`
	TotalExecutions int64         `json:"total_executions"`
	SuccessCount    int64         `json:"success_count"`
	FailureCount    int64         `json:"failure_count"`
	SkipCount       int64         `json:"skip_count"`
	SuccessRate     float64       `json:"success_rate"`
	LastExecTime    time.Time     `json:"last_exec_time,omitempty"`
	LastDuration    time.Duration `json:"last_duration"`
	LastError       string        `json:"last_error,omitempty"`
	AvgDuration     time.Duration `json:"avg_duration"`
	MinDuration     time.Duration `json:"min_duration"`
	MaxDuration     time.Duration `json:"max_duration"`
}

// Snapshot 返回统计快照。
func (s *Stats) Snapshot() *StatsSnapshot {
	snap := &StatsSnapshot{
		TotalExecutions: s.TotalExecutions(),
		SuccessCount:    s.SuccessCount(),
		FailureCount:    s.FailureCount(),
		SkipCount:       s.SkipCount(),
		SuccessRate:     s.SuccessRate(),
		LastExecTime:    s.LastExecTime(),
		LastDuration:    s.LastDuration(),
		AvgDuration:     s.AvgDuration(),
		MinDuration:     s.MinDuration(),
		MaxDuration:     s.MaxDuration(),
		Jobs:            make(map[string]*JobStatsSnapshot),
	}

	if err := s.LastError(); err != nil {
		snap.LastError = err.Error()
	}

	for name, js := range s.AllJobStats() {
		snap.Jobs[name] = js.Snapshot()
	}

	return snap
}

// Snapshot 返回任务统计快照。
func (js *JobStats) Snapshot() *JobStatsSnapshot {
	snap := &JobStatsSnapshot{
		Name:            js.Name,
		TotalExecutions: js.TotalExecutions(),
		SuccessCount:    js.SuccessCount(),
		FailureCount:    js.FailureCount(),
		SkipCount:       js.SkipCount(),
		SuccessRate:     js.SuccessRate(),
		LastExecTime:    js.LastExecTime(),
		LastDuration:    js.LastDuration(),
		AvgDuration:     js.AvgDuration(),
		MinDuration:     js.MinDuration(),
		MaxDuration:     js.MaxDuration(),
	}

	if err := js.LastError(); err != nil {
		snap.LastError = err.Error()
	}

	return snap
}
