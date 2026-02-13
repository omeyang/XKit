package xcron

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// ============================================================================
// NewK8sLocker Tests
// ============================================================================

func TestNewK8sLocker(t *testing.T) {
	t.Run("with custom client", func(t *testing.T) {
		fakeClient := fake.NewSimpleClientset()
		locker, err := NewK8sLocker(K8sLockerOptions{
			Client:    fakeClient,
			Namespace: "test-ns",
			Identity:  "test-pod",
			Prefix:    "test-",
		})

		require.NoError(t, err)
		require.NotNil(t, locker)
		assert.Equal(t, "test-ns", locker.Namespace())
		assert.Equal(t, "test-pod", locker.Identity())
		assert.Equal(t, fakeClient, locker.Client())
	})

	t.Run("with default values", func(t *testing.T) {
		fakeClient := fake.NewSimpleClientset()
		locker, err := NewK8sLocker(K8sLockerOptions{
			Client: fakeClient,
		})

		require.NoError(t, err)
		require.NotNil(t, locker)
		// 默认值应该被设置
		assert.NotEmpty(t, locker.Namespace())
		assert.NotEmpty(t, locker.Identity())
	})

	t.Run("without client in non-cluster env returns error", func(t *testing.T) {
		_, err := NewK8sLocker(K8sLockerOptions{})

		// 在非集群环境中应该返回错误
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "in-cluster config")
	})
}

// ============================================================================
// K8sLocker TryLock Tests
// ============================================================================

func TestK8sLocker_TryLock(t *testing.T) {
	ctx := context.Background()

	t.Run("creates lease when not exists", func(t *testing.T) {
		fakeClient := fake.NewSimpleClientset()
		locker, err := NewK8sLocker(K8sLockerOptions{
			Client:    fakeClient,
			Namespace: "default",
			Identity:  "pod-1",
			Prefix:    "xcron-",
		})
		require.NoError(t, err)

		handle, err := locker.TryLock(ctx, "test-job", 5*time.Minute)

		assert.NoError(t, err)
		assert.NotNil(t, handle)

		// 验证 Lease 被创建
		lease, err := fakeClient.CoordinationV1().Leases("default").Get(ctx, "xcron-test-job", metav1.GetOptions{})
		require.NoError(t, err)
		// HolderIdentity 现在是 "pod-1:uuid" 格式
		assert.True(t, strings.HasPrefix(*lease.Spec.HolderIdentity, "pod-1:"))
	})

	t.Run("fails when lease held by another instance", func(t *testing.T) {
		fakeClient := fake.NewSimpleClientset()

		// 预先创建一个被其他实例持有的 Lease
		now := metav1.NewMicroTime(time.Now())
		duration := int32(300)
		holder := "other-pod:some-uuid"
		existingLease := &coordinationv1.Lease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "xcron-test-job",
				Namespace: "default",
			},
			Spec: coordinationv1.LeaseSpec{
				HolderIdentity:       &holder,
				LeaseDurationSeconds: &duration,
				AcquireTime:          &now,
				RenewTime:            &now,
			},
		}
		_, err := fakeClient.CoordinationV1().Leases("default").Create(ctx, existingLease, metav1.CreateOptions{})
		require.NoError(t, err)

		locker, err := NewK8sLocker(K8sLockerOptions{
			Client:    fakeClient,
			Namespace: "default",
			Identity:  "pod-1",
			Prefix:    "xcron-",
		})
		require.NoError(t, err)

		handle, err := locker.TryLock(ctx, "test-job", 5*time.Minute)

		assert.NoError(t, err)
		assert.Nil(t, handle) // 应该获取失败
	})

	t.Run("acquires expired lease", func(t *testing.T) {
		fakeClient := fake.NewSimpleClientset()

		// 创建一个已过期的 Lease
		pastTime := metav1.NewMicroTime(time.Now().Add(-10 * time.Minute))
		duration := int32(60) // 1分钟，已经过期
		holder := "other-pod:old-uuid"
		existingLease := &coordinationv1.Lease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "xcron-test-job",
				Namespace: "default",
			},
			Spec: coordinationv1.LeaseSpec{
				HolderIdentity:       &holder,
				LeaseDurationSeconds: &duration,
				AcquireTime:          &pastTime,
				RenewTime:            &pastTime,
			},
		}
		_, err := fakeClient.CoordinationV1().Leases("default").Create(ctx, existingLease, metav1.CreateOptions{})
		require.NoError(t, err)

		locker, err := NewK8sLocker(K8sLockerOptions{
			Client:    fakeClient,
			Namespace: "default",
			Identity:  "pod-1",
			Prefix:    "xcron-",
		})
		require.NoError(t, err)

		handle, err := locker.TryLock(ctx, "test-job", 5*time.Minute)

		assert.NoError(t, err)
		assert.NotNil(t, handle) // 过期的锁可以被获取
	})

	t.Run("prevents reentry on own non-expired lease", func(t *testing.T) {
		fakeClient := fake.NewSimpleClientset()
		locker, err := NewK8sLocker(K8sLockerOptions{
			Client:    fakeClient,
			Namespace: "default",
			Identity:  "pod-1",
			Prefix:    "xcron-",
		})
		require.NoError(t, err)

		// 第一次获取
		handle1, err := locker.TryLock(ctx, "test-job", 5*time.Minute)
		require.NoError(t, err)
		require.NotNil(t, handle1)

		// 第二次获取（同一个实例，但会生成不同的 token）
		// 设计决策：即使是同一实例也不允许重入，防止重叠调度导致的并发问题
		// 场景：任务 A 执行中，任务 B（同 key）被调度，B 获得"重入"锁后先完成并释放，
		//       此时 A 仍在执行但已无锁保护，其他实例可获取锁并发执行
		handle2, err := locker.TryLock(ctx, "test-job", 5*time.Minute)

		assert.NoError(t, err)
		assert.Nil(t, handle2) // 不允许重入
	})
}

// ============================================================================
// K8sLocker Unlock Tests
// ============================================================================

func TestK8sLockHandle_Unlock(t *testing.T) {
	ctx := context.Background()

	t.Run("releases held lock", func(t *testing.T) {
		fakeClient := fake.NewSimpleClientset()
		locker, err := NewK8sLocker(K8sLockerOptions{
			Client:    fakeClient,
			Namespace: "default",
			Identity:  "pod-1",
			Prefix:    "xcron-",
		})
		require.NoError(t, err)

		// 先获取锁
		handle, err := locker.TryLock(ctx, "test-job", 5*time.Minute)
		require.NoError(t, err)
		require.NotNil(t, handle)

		// 释放锁
		err = handle.Unlock(ctx)

		assert.NoError(t, err)

		// 验证 Lease 的 HolderIdentity 已清除
		lease, err := fakeClient.CoordinationV1().Leases("default").Get(ctx, "xcron-test-job", metav1.GetOptions{})
		require.NoError(t, err)
		assert.Nil(t, lease.Spec.HolderIdentity)
	})

	t.Run("returns not held when lease owned by another token", func(t *testing.T) {
		fakeClient := fake.NewSimpleClientset()

		locker, err := NewK8sLocker(K8sLockerOptions{
			Client:    fakeClient,
			Namespace: "default",
			Identity:  "pod-1",
			Prefix:    "xcron-",
		})
		require.NoError(t, err)

		// 获取锁
		handle, err := locker.TryLock(ctx, "test-job", 5*time.Minute)
		require.NoError(t, err)
		require.NotNil(t, handle)

		// 直接修改 Lease 的 HolderIdentity 为其他值（模拟被其他实例抢占）
		lease, err := fakeClient.CoordinationV1().Leases("default").Get(ctx, "xcron-test-job", metav1.GetOptions{})
		require.NoError(t, err)
		newHolder := "other-pod:different-uuid"
		lease.Spec.HolderIdentity = &newHolder
		_, err = fakeClient.CoordinationV1().Leases("default").Update(ctx, lease, metav1.UpdateOptions{})
		require.NoError(t, err)

		// 尝试用旧 handle 释放，应该失败
		err = handle.Unlock(ctx)

		assert.Equal(t, ErrLockNotHeld, err)
	})
}

// ============================================================================
// K8sLocker Renew Tests
// ============================================================================

func TestK8sLockHandle_Renew(t *testing.T) {
	ctx := context.Background()

	t.Run("renews held lock", func(t *testing.T) {
		fakeClient := fake.NewSimpleClientset()
		locker, err := NewK8sLocker(K8sLockerOptions{
			Client:    fakeClient,
			Namespace: "default",
			Identity:  "pod-1",
			Prefix:    "xcron-",
		})
		require.NoError(t, err)

		// 先获取锁
		handle, err := locker.TryLock(ctx, "test-job", 5*time.Minute)
		require.NoError(t, err)
		require.NotNil(t, handle)

		// 等待一小段时间
		time.Sleep(10 * time.Millisecond)

		// 续期
		err = handle.Renew(ctx, 10*time.Minute)

		assert.NoError(t, err)

		// 验证 LeaseDuration 已更新
		lease, err := fakeClient.CoordinationV1().Leases("default").Get(ctx, "xcron-test-job", metav1.GetOptions{})
		require.NoError(t, err)
		assert.Equal(t, int32(600), *lease.Spec.LeaseDurationSeconds)
	})

	t.Run("returns not held when lease owned by another token", func(t *testing.T) {
		fakeClient := fake.NewSimpleClientset()

		locker, err := NewK8sLocker(K8sLockerOptions{
			Client:    fakeClient,
			Namespace: "default",
			Identity:  "pod-1",
			Prefix:    "xcron-",
		})
		require.NoError(t, err)

		// 获取锁
		handle, err := locker.TryLock(ctx, "test-job", 5*time.Minute)
		require.NoError(t, err)
		require.NotNil(t, handle)

		// 直接修改 Lease 的 HolderIdentity 为其他值（模拟被其他实例抢占）
		lease, err := fakeClient.CoordinationV1().Leases("default").Get(ctx, "xcron-test-job", metav1.GetOptions{})
		require.NoError(t, err)
		newHolder := "other-pod:different-uuid"
		lease.Spec.HolderIdentity = &newHolder
		_, err = fakeClient.CoordinationV1().Leases("default").Update(ctx, lease, metav1.UpdateOptions{})
		require.NoError(t, err)

		// 尝试用旧 handle 续期，应该失败
		err = handle.Renew(ctx, 10*time.Minute)

		assert.Equal(t, ErrLockNotHeld, err)
	})
}

// ============================================================================
// K8sLocker Helper Function Tests
// ============================================================================

func TestK8sLocker_isLeaseExpired(t *testing.T) {
	fakeClient := fake.NewSimpleClientset()
	locker, _ := NewK8sLocker(K8sLockerOptions{
		Client:    fakeClient,
		Namespace: "default",
		Identity:  "pod-1",
	})

	t.Run("expired lease", func(t *testing.T) {
		pastTime := metav1.NewMicroTime(time.Now().Add(-10 * time.Minute))
		duration := int32(60) // 1分钟
		lease := &coordinationv1.Lease{
			Spec: coordinationv1.LeaseSpec{
				RenewTime:            &pastTime,
				LeaseDurationSeconds: &duration,
			},
		}

		assert.True(t, locker.isLeaseExpired(lease))
	})

	t.Run("not expired lease", func(t *testing.T) {
		now := metav1.NewMicroTime(time.Now())
		duration := int32(300) // 5分钟
		lease := &coordinationv1.Lease{
			Spec: coordinationv1.LeaseSpec{
				RenewTime:            &now,
				LeaseDurationSeconds: &duration,
			},
		}

		assert.False(t, locker.isLeaseExpired(lease))
	})

	t.Run("nil renewTime is expired", func(t *testing.T) {
		duration := int32(300)
		lease := &coordinationv1.Lease{
			Spec: coordinationv1.LeaseSpec{
				RenewTime:            nil,
				LeaseDurationSeconds: &duration,
			},
		}

		assert.True(t, locker.isLeaseExpired(lease))
	})

	t.Run("nil duration is expired", func(t *testing.T) {
		now := metav1.NewMicroTime(time.Now())
		lease := &coordinationv1.Lease{
			Spec: coordinationv1.LeaseSpec{
				RenewTime:            &now,
				LeaseDurationSeconds: nil,
			},
		}

		assert.True(t, locker.isLeaseExpired(lease))
	})
}

func TestK8sLocker_canAcquire(t *testing.T) {
	fakeClient := fake.NewSimpleClientset()
	locker, _ := NewK8sLocker(K8sLockerOptions{
		Client:    fakeClient,
		Namespace: "default",
		Identity:  "pod-1",
	})

	t.Run("nil holder", func(t *testing.T) {
		lease := &coordinationv1.Lease{
			Spec: coordinationv1.LeaseSpec{
				HolderIdentity: nil,
			},
		}

		assert.True(t, locker.canAcquire(lease))
	})

	t.Run("empty holder", func(t *testing.T) {
		empty := ""
		lease := &coordinationv1.Lease{
			Spec: coordinationv1.LeaseSpec{
				HolderIdentity: &empty,
			},
		}

		assert.True(t, locker.canAcquire(lease))
	})

	t.Run("own holder not expired", func(t *testing.T) {
		// 新设计中，即使是自己的 holder（包含相同 identity 前缀）也不允许重入
		// 防止重叠调度导致并发执行
		self := "pod-1:some-uuid"
		now := metav1.NewMicroTime(time.Now())
		duration := int32(300)
		lease := &coordinationv1.Lease{
			Spec: coordinationv1.LeaseSpec{
				HolderIdentity:       &self,
				RenewTime:            &now,
				LeaseDurationSeconds: &duration,
			},
		}

		// 不允许重入，必须等待过期
		assert.False(t, locker.canAcquire(lease))
	})

	t.Run("other holder not expired", func(t *testing.T) {
		other := "other-pod"
		now := metav1.NewMicroTime(time.Now())
		duration := int32(300)
		lease := &coordinationv1.Lease{
			Spec: coordinationv1.LeaseSpec{
				HolderIdentity:       &other,
				RenewTime:            &now,
				LeaseDurationSeconds: &duration,
			},
		}

		assert.False(t, locker.canAcquire(lease))
	})

	t.Run("other holder but expired", func(t *testing.T) {
		other := "other-pod"
		pastTime := metav1.NewMicroTime(time.Now().Add(-10 * time.Minute))
		duration := int32(60)
		lease := &coordinationv1.Lease{
			Spec: coordinationv1.LeaseSpec{
				HolderIdentity:       &other,
				RenewTime:            &pastTime,
				LeaseDurationSeconds: &duration,
			},
		}

		assert.True(t, locker.canAcquire(lease))
	})
}

func TestK8sLocker_leaseName(t *testing.T) {
	fakeClient := fake.NewSimpleClientset()
	locker, _ := NewK8sLocker(K8sLockerOptions{
		Client:    fakeClient,
		Namespace: "default",
		Identity:  "pod-1",
		Prefix:    "xcron-",
	})

	t.Run("clean key unchanged", func(t *testing.T) {
		result := locker.leaseName("simple-job")
		assert.Equal(t, "xcron-simple-job", result)
	})

	t.Run("sanitized keys get hash suffix", func(t *testing.T) {
		// 需要字符替换的 key 添加 hash 后缀确保唯一性
		keys := []string{"job.with.dots", "JOB_WITH_UPPER", "job/with/slashes"}
		for _, key := range keys {
			t.Run(key, func(t *testing.T) {
				result := locker.leaseName(key)
				assert.True(t, len(result) <= 63, "lease name exceeds 63 chars")
				assert.Regexp(t, `^xcron-[a-z0-9-]+-[a-f0-9]{8}$`, result)
			})
		}
	})

	t.Run("different keys produce different lease names", func(t *testing.T) {
		// 确保不同 key 不会碰撞
		r1 := locker.leaseName("my.job")
		r2 := locker.leaseName("my/job")
		assert.NotEqual(t, r1, r2, "different keys should produce different lease names")
	})
}

// ============================================================================
// Integration Tests
// ============================================================================

func TestK8sLocker_FullCycle(t *testing.T) {
	ctx := context.Background()
	fakeClient := fake.NewSimpleClientset()

	locker, err := NewK8sLocker(K8sLockerOptions{
		Client:    fakeClient,
		Namespace: "default",
		Identity:  "pod-1",
		Prefix:    "xcron-",
	})
	require.NoError(t, err)

	// 1. 获取锁
	handle, err := locker.TryLock(ctx, "my-job", 5*time.Minute)
	require.NoError(t, err)
	require.NotNil(t, handle)

	// 2. 续期
	err = handle.Renew(ctx, 10*time.Minute)
	require.NoError(t, err)

	// 3. 释放锁
	err = handle.Unlock(ctx)
	require.NoError(t, err)

	// 4. 另一个实例可以获取
	locker2, _ := NewK8sLocker(K8sLockerOptions{
		Client:    fakeClient,
		Namespace: "default",
		Identity:  "pod-2",
		Prefix:    "xcron-",
	})

	handle2, err := locker2.TryLock(ctx, "my-job", 5*time.Minute)
	assert.NoError(t, err)
	assert.NotNil(t, handle2)
}
