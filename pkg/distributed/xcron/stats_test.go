package xcron

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStats_NewStats(t *testing.T) {
	stats := newStats()
	require.NotNil(t, stats)
	assert.Equal(t, int64(0), stats.TotalExecutions())
	assert.Equal(t, int64(0), stats.SuccessCount())
	assert.Equal(t, int64(0), stats.FailureCount())
	assert.Equal(t, int64(0), stats.SkipCount())
	assert.Equal(t, time.Duration(0), stats.MinDuration())
	assert.Equal(t, time.Duration(0), stats.MaxDuration())
	assert.Equal(t, time.Duration(0), stats.AvgDuration())
	assert.Equal(t, float64(0), stats.SuccessRate())
}

func TestStats_RecordExecution(t *testing.T) {
	stats := newStats()

	// 记录成功执行
	stats.recordExecution("test-job", 100*time.Millisecond, nil)

	assert.Equal(t, int64(1), stats.TotalExecutions())
	assert.Equal(t, int64(1), stats.SuccessCount())
	assert.Equal(t, int64(0), stats.FailureCount())
	assert.Equal(t, float64(1), stats.SuccessRate())
	assert.Equal(t, 100*time.Millisecond, stats.MinDuration())
	assert.Equal(t, 100*time.Millisecond, stats.MaxDuration())
	assert.Equal(t, 100*time.Millisecond, stats.AvgDuration())
	assert.Equal(t, 100*time.Millisecond, stats.LastDuration())
	assert.NoError(t, stats.LastError())

	// 记录失败执行
	testErr := errors.New("test error")
	stats.recordExecution("test-job", 200*time.Millisecond, testErr)

	assert.Equal(t, int64(2), stats.TotalExecutions())
	assert.Equal(t, int64(1), stats.SuccessCount())
	assert.Equal(t, int64(1), stats.FailureCount())
	assert.Equal(t, float64(0.5), stats.SuccessRate())
	assert.Equal(t, 100*time.Millisecond, stats.MinDuration())
	assert.Equal(t, 200*time.Millisecond, stats.MaxDuration())
	assert.Equal(t, 150*time.Millisecond, stats.AvgDuration())
	assert.Equal(t, 200*time.Millisecond, stats.LastDuration())
	assert.Equal(t, testErr, stats.LastError())
}

func TestStats_RecordSkip(t *testing.T) {
	stats := newStats()

	stats.recordSkip("test-job")
	stats.recordSkip("test-job")

	assert.Equal(t, int64(0), stats.TotalExecutions())
	assert.Equal(t, int64(2), stats.SkipCount())
}

func TestStats_JobStats(t *testing.T) {
	stats := newStats()

	// 记录不同任务的执行
	stats.recordExecution("job-a", 100*time.Millisecond, nil)
	stats.recordExecution("job-a", 200*time.Millisecond, nil)
	stats.recordExecution("job-b", 50*time.Millisecond, errors.New("error"))
	stats.recordSkip("job-b")

	// 验证任务级统计
	jobA := stats.JobStats("job-a")
	require.NotNil(t, jobA)
	assert.Equal(t, "job-a", jobA.Name)
	assert.Equal(t, int64(2), jobA.TotalExecutions())
	assert.Equal(t, int64(2), jobA.SuccessCount())
	assert.Equal(t, int64(0), jobA.FailureCount())
	assert.Equal(t, float64(1), jobA.SuccessRate())
	assert.Equal(t, 100*time.Millisecond, jobA.MinDuration())
	assert.Equal(t, 200*time.Millisecond, jobA.MaxDuration())

	jobB := stats.JobStats("job-b")
	require.NotNil(t, jobB)
	assert.Equal(t, "job-b", jobB.Name)
	assert.Equal(t, int64(1), jobB.TotalExecutions())
	assert.Equal(t, int64(0), jobB.SuccessCount())
	assert.Equal(t, int64(1), jobB.FailureCount())
	assert.Equal(t, int64(1), jobB.SkipCount())
	assert.Equal(t, float64(0), jobB.SuccessRate())

	// 验证不存在的任务
	assert.Nil(t, stats.JobStats("nonexistent"))
}

func TestStats_AllJobStats(t *testing.T) {
	stats := newStats()

	stats.recordExecution("job-a", 100*time.Millisecond, nil)
	stats.recordExecution("job-b", 50*time.Millisecond, nil)
	stats.recordExecution("job-c", 150*time.Millisecond, nil)

	all := stats.AllJobStats()
	assert.Len(t, all, 3)
	assert.Contains(t, all, "job-a")
	assert.Contains(t, all, "job-b")
	assert.Contains(t, all, "job-c")
}

func TestStats_Snapshot(t *testing.T) {
	stats := newStats()

	testErr := errors.New("snapshot test error")
	stats.recordExecution("job-a", 100*time.Millisecond, nil)
	stats.recordExecution("job-b", 200*time.Millisecond, testErr)
	stats.recordSkip("job-a")

	snap := stats.Snapshot()
	require.NotNil(t, snap)

	assert.Equal(t, int64(2), snap.TotalExecutions)
	assert.Equal(t, int64(1), snap.SuccessCount)
	assert.Equal(t, int64(1), snap.FailureCount)
	assert.Equal(t, int64(1), snap.SkipCount)
	assert.Equal(t, float64(0.5), snap.SuccessRate)
	assert.Equal(t, "snapshot test error", snap.LastError)
	assert.Len(t, snap.Jobs, 2)

	jobASnap := snap.Jobs["job-a"]
	require.NotNil(t, jobASnap)
	assert.Equal(t, "job-a", jobASnap.Name)
	assert.Equal(t, int64(1), jobASnap.TotalExecutions)
	assert.Equal(t, int64(1), jobASnap.SkipCount)
}

func TestStats_Concurrent(t *testing.T) {
	stats := newStats()
	var wg sync.WaitGroup

	// 并发记录
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			var err error
			if n%3 == 0 {
				err = errors.New("error")
			}
			stats.recordExecution("concurrent-job", time.Duration(n)*time.Millisecond, err)
		}(i)
	}

	// 并发跳过
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			stats.recordSkip("concurrent-job")
		}()
	}

	wg.Wait()

	assert.Equal(t, int64(100), stats.TotalExecutions())
	assert.Equal(t, int64(50), stats.SkipCount())
	assert.Equal(t, int64(100), stats.SuccessCount()+stats.FailureCount())
}

func TestJobStats_Snapshot(t *testing.T) {
	stats := newStats()

	testErr := errors.New("job snapshot error")
	stats.recordExecution("my-job", 100*time.Millisecond, nil)
	stats.recordExecution("my-job", 50*time.Millisecond, testErr)
	stats.recordSkip("my-job")

	js := stats.JobStats("my-job")
	require.NotNil(t, js)

	snap := js.Snapshot()
	assert.Equal(t, "my-job", snap.Name)
	assert.Equal(t, int64(2), snap.TotalExecutions)
	assert.Equal(t, int64(1), snap.SuccessCount)
	assert.Equal(t, int64(1), snap.FailureCount)
	assert.Equal(t, int64(1), snap.SkipCount)
	assert.Equal(t, float64(0.5), snap.SuccessRate)
	assert.Equal(t, 50*time.Millisecond, snap.MinDuration)
	assert.Equal(t, 100*time.Millisecond, snap.MaxDuration)
	assert.Equal(t, "job snapshot error", snap.LastError)
}

func TestScheduler_Stats(t *testing.T) {
	scheduler := New()
	require.NotNil(t, scheduler)

	stats := scheduler.Stats()
	require.NotNil(t, stats)
	assert.Equal(t, int64(0), stats.TotalExecutions())
}

func TestScheduler_StatsIntegration(t *testing.T) {
	scheduler := New(WithSeconds()) // 启用秒级精度以加快测试
	var successCount, failureCount atomic.Int64

	// 添加一个会成功的任务
	_, err := scheduler.AddFunc("*/1 * * * * *", func(ctx context.Context) error {
		successCount.Add(1)
		return nil
	}, WithName("success-job"))
	require.NoError(t, err)

	// 添加一个会失败的任务
	_, err = scheduler.AddFunc("*/1 * * * * *", func(ctx context.Context) error {
		failureCount.Add(1)
		return errors.New("intentional error")
	}, WithName("failure-job"))
	require.NoError(t, err)

	scheduler.Start()
	defer scheduler.Stop()

	// 等待足够的执行（等待至少2秒确保任务执行）
	time.Sleep(2500 * time.Millisecond)

	// 验证统计
	stats := scheduler.Stats()
	assert.GreaterOrEqual(t, stats.TotalExecutions(), int64(2))
	assert.GreaterOrEqual(t, stats.SuccessCount(), int64(1))
	assert.GreaterOrEqual(t, stats.FailureCount(), int64(1))

	// 验证任务级统计
	successStats := stats.JobStats("success-job")
	require.NotNil(t, successStats)
	assert.GreaterOrEqual(t, successStats.SuccessCount(), int64(1))
	assert.Equal(t, int64(0), successStats.FailureCount())

	failureStats := stats.JobStats("failure-job")
	require.NotNil(t, failureStats)
	assert.GreaterOrEqual(t, failureStats.FailureCount(), int64(1))
}
