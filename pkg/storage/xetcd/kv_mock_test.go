package xetcd

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/mock/gomock"
)

// newTestClient 创建用于测试的 Client，注入 mock etcdClient。
func newTestClient(t *testing.T, mockClient *MocketcdClient) *Client {
	t.Helper()
	return &Client{
		client: mockClient,
		config: &Config{Endpoints: []string{"localhost:2379"}},
	}
}

// TestKV_Get_Success 测试 Get 成功获取值。
func TestKV_Get_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := NewMocketcdClient(ctrl)
	c := newTestClient(t, mockClient)

	ctx := context.Background()
	key := "test-key"
	value := []byte("test-value")

	mockClient.EXPECT().
		Get(ctx, key).
		Return(&clientv3.GetResponse{
			Kvs: []*mvccpb.KeyValue{
				{Key: []byte(key), Value: value, ModRevision: 100},
			},
		}, nil)

	result, err := c.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	if string(result) != string(value) {
		t.Errorf("Get() = %q, want %q", result, value)
	}
}

// TestKV_Get_NotFound 测试 Get 键不存在。
func TestKV_Get_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := NewMocketcdClient(ctrl)
	c := newTestClient(t, mockClient)

	ctx := context.Background()
	key := "non-existent-key"

	mockClient.EXPECT().
		Get(ctx, key).
		Return(&clientv3.GetResponse{Kvs: nil}, nil)

	_, err := c.Get(ctx, key)
	if !errors.Is(err, ErrKeyNotFound) {
		t.Errorf("Get() error = %v, want %v", err, ErrKeyNotFound)
	}
}

// TestKV_Get_Error 测试 Get 发生错误。
func TestKV_Get_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := NewMocketcdClient(ctrl)
	c := newTestClient(t, mockClient)

	ctx := context.Background()
	key := "test-key"
	expectedErr := errors.New("connection refused")

	mockClient.EXPECT().
		Get(ctx, key).
		Return(nil, expectedErr)

	_, err := c.Get(ctx, key)
	if err == nil {
		t.Fatal("Get() error = nil, want error")
	}
	if !errors.Is(err, expectedErr) {
		t.Logf("Get() error = %v (wrapped)", err)
	}
}

// TestKV_GetWithRevision_Success 测试 GetWithRevision 成功。
func TestKV_GetWithRevision_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := NewMocketcdClient(ctrl)
	c := newTestClient(t, mockClient)

	ctx := context.Background()
	key := "test-key"
	value := []byte("test-value")
	revision := int64(12345)

	mockClient.EXPECT().
		Get(ctx, key).
		Return(&clientv3.GetResponse{
			Kvs: []*mvccpb.KeyValue{
				{Key: []byte(key), Value: value, ModRevision: revision},
			},
		}, nil)

	result, rev, err := c.GetWithRevision(ctx, key)
	if err != nil {
		t.Fatalf("GetWithRevision() error = %v, want nil", err)
	}
	if string(result) != string(value) {
		t.Errorf("GetWithRevision() value = %q, want %q", result, value)
	}
	if rev != revision {
		t.Errorf("GetWithRevision() revision = %d, want %d", rev, revision)
	}
}

// TestKV_Put_Success 测试 Put 成功写入。
func TestKV_Put_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := NewMocketcdClient(ctrl)
	c := newTestClient(t, mockClient)

	ctx := context.Background()
	key := "test-key"
	value := []byte("test-value")

	mockClient.EXPECT().
		Put(ctx, key, string(value)).
		Return(&clientv3.PutResponse{}, nil)

	err := c.Put(ctx, key, value)
	if err != nil {
		t.Fatalf("Put() error = %v, want nil", err)
	}
}

// TestKV_Put_Error 测试 Put 发生错误。
func TestKV_Put_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := NewMocketcdClient(ctrl)
	c := newTestClient(t, mockClient)

	ctx := context.Background()
	key := "test-key"
	value := []byte("test-value")
	expectedErr := errors.New("disk full")

	mockClient.EXPECT().
		Put(ctx, key, string(value)).
		Return(nil, expectedErr)

	err := c.Put(ctx, key, value)
	if err == nil {
		t.Fatal("Put() error = nil, want error")
	}
}

// TestKV_PutWithTTL_Success 测试 PutWithTTL 成功写入。
func TestKV_PutWithTTL_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := NewMocketcdClient(ctrl)
	c := newTestClient(t, mockClient)

	ctx := context.Background()
	key := "test-key"
	value := []byte("test-value")
	ttl := 10 * time.Second
	leaseID := clientv3.LeaseID(123456)

	mockClient.EXPECT().
		Grant(ctx, int64(10)).
		Return(&clientv3.LeaseGrantResponse{ID: leaseID}, nil)

	mockClient.EXPECT().
		Put(ctx, key, string(value), gomock.Any()).
		Return(&clientv3.PutResponse{}, nil)

	err := c.PutWithTTL(ctx, key, value, ttl)
	if err != nil {
		t.Fatalf("PutWithTTL() error = %v, want nil", err)
	}
}

// TestKV_PutWithTTL_ZeroTTL 测试 PutWithTTL TTL 为 0 时降级为普通 Put。
func TestKV_PutWithTTL_ZeroTTL(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := NewMocketcdClient(ctrl)
	c := newTestClient(t, mockClient)

	ctx := context.Background()
	key := "test-key"
	value := []byte("test-value")

	// TTL <= 0 时降级为普通 Put
	mockClient.EXPECT().
		Put(ctx, key, string(value)).
		Return(&clientv3.PutResponse{}, nil)

	err := c.PutWithTTL(ctx, key, value, 0)
	if err != nil {
		t.Fatalf("PutWithTTL() error = %v, want nil", err)
	}
}

// TestKV_PutWithTTL_NegativeTTL 测试 PutWithTTL 负数 TTL。
func TestKV_PutWithTTL_NegativeTTL(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := NewMocketcdClient(ctrl)
	c := newTestClient(t, mockClient)

	ctx := context.Background()
	key := "test-key"
	value := []byte("test-value")

	// TTL <= 0 时降级为普通 Put
	mockClient.EXPECT().
		Put(ctx, key, string(value)).
		Return(&clientv3.PutResponse{}, nil)

	err := c.PutWithTTL(ctx, key, value, -5*time.Second)
	if err != nil {
		t.Fatalf("PutWithTTL() error = %v, want nil", err)
	}
}

// TestKV_PutWithTTL_GrantError 测试 PutWithTTL 创建租约失败。
func TestKV_PutWithTTL_GrantError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := NewMocketcdClient(ctrl)
	c := newTestClient(t, mockClient)

	ctx := context.Background()
	key := "test-key"
	value := []byte("test-value")
	ttl := 10 * time.Second
	expectedErr := errors.New("lease grant failed")

	mockClient.EXPECT().
		Grant(ctx, int64(10)).
		Return(nil, expectedErr)

	err := c.PutWithTTL(ctx, key, value, ttl)
	if err == nil {
		t.Fatal("PutWithTTL() error = nil, want error")
	}
}

// TestKV_PutWithTTL_PutError 测试 PutWithTTL 写入失败。
func TestKV_PutWithTTL_PutError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := NewMocketcdClient(ctrl)
	c := newTestClient(t, mockClient)

	ctx := context.Background()
	key := "test-key"
	value := []byte("test-value")
	ttl := 10 * time.Second
	leaseID := clientv3.LeaseID(123456)
	expectedErr := errors.New("put with lease failed")

	mockClient.EXPECT().
		Grant(ctx, int64(10)).
		Return(&clientv3.LeaseGrantResponse{ID: leaseID}, nil)

	mockClient.EXPECT().
		Put(ctx, key, string(value), gomock.Any()).
		Return(nil, expectedErr)

	// Put 失败后会调用 Revoke 撤销租约（使用 Background context）
	mockClient.EXPECT().
		Revoke(gomock.Any(), leaseID).
		Return(&clientv3.LeaseRevokeResponse{}, nil)

	err := c.PutWithTTL(ctx, key, value, ttl)
	if err == nil {
		t.Fatal("PutWithTTL() error = nil, want error")
	}
}

// TestKV_PutWithTTL_SmallTTL 测试 PutWithTTL 小于 1 秒的 TTL 被转换为 1 秒。
func TestKV_PutWithTTL_SmallTTL(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := NewMocketcdClient(ctrl)
	c := newTestClient(t, mockClient)

	ctx := context.Background()
	key := "test-key"
	value := []byte("test-value")
	ttl := 500 * time.Millisecond // 0.5 秒，会被转换为 1 秒
	leaseID := clientv3.LeaseID(123456)

	// 500ms 会被转换为 1 秒
	mockClient.EXPECT().
		Grant(ctx, int64(1)).
		Return(&clientv3.LeaseGrantResponse{ID: leaseID}, nil)

	mockClient.EXPECT().
		Put(ctx, key, string(value), gomock.Any()).
		Return(&clientv3.PutResponse{}, nil)

	err := c.PutWithTTL(ctx, key, value, ttl)
	if err != nil {
		t.Fatalf("PutWithTTL() error = %v, want nil", err)
	}
}

// TestKV_Delete_Success 测试 Delete 成功删除。
func TestKV_Delete_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := NewMocketcdClient(ctrl)
	c := newTestClient(t, mockClient)

	ctx := context.Background()
	key := "test-key"

	mockClient.EXPECT().
		Delete(ctx, key).
		Return(&clientv3.DeleteResponse{Deleted: 1}, nil)

	err := c.Delete(ctx, key)
	if err != nil {
		t.Fatalf("Delete() error = %v, want nil", err)
	}
}

// TestKV_Delete_NotExist 测试 Delete 删除不存在的键（不报错）。
func TestKV_Delete_NotExist(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := NewMocketcdClient(ctrl)
	c := newTestClient(t, mockClient)

	ctx := context.Background()
	key := "non-existent-key"

	mockClient.EXPECT().
		Delete(ctx, key).
		Return(&clientv3.DeleteResponse{Deleted: 0}, nil)

	err := c.Delete(ctx, key)
	if err != nil {
		t.Fatalf("Delete() error = %v, want nil (not found should not be error)", err)
	}
}

// TestKV_Delete_Error 测试 Delete 发生错误。
func TestKV_Delete_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := NewMocketcdClient(ctrl)
	c := newTestClient(t, mockClient)

	ctx := context.Background()
	key := "test-key"
	expectedErr := errors.New("connection refused")

	mockClient.EXPECT().
		Delete(ctx, key).
		Return(nil, expectedErr)

	err := c.Delete(ctx, key)
	if err == nil {
		t.Fatal("Delete() error = nil, want error")
	}
}

// TestKV_DeleteWithPrefix_Success 测试 DeleteWithPrefix 成功删除。
func TestKV_DeleteWithPrefix_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := NewMocketcdClient(ctrl)
	c := newTestClient(t, mockClient)

	ctx := context.Background()
	prefix := "/app/config/"

	mockClient.EXPECT().
		Delete(ctx, prefix, gomock.Any()).
		Return(&clientv3.DeleteResponse{Deleted: 5}, nil)

	deleted, err := c.DeleteWithPrefix(ctx, prefix)
	if err != nil {
		t.Fatalf("DeleteWithPrefix() error = %v, want nil", err)
	}
	if deleted != 5 {
		t.Errorf("DeleteWithPrefix() deleted = %d, want 5", deleted)
	}
}

// TestKV_DeleteWithPrefix_Error 测试 DeleteWithPrefix 发生错误。
func TestKV_DeleteWithPrefix_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := NewMocketcdClient(ctrl)
	c := newTestClient(t, mockClient)

	ctx := context.Background()
	prefix := "/app/config/"
	expectedErr := errors.New("permission denied")

	mockClient.EXPECT().
		Delete(ctx, prefix, gomock.Any()).
		Return(nil, expectedErr)

	_, err := c.DeleteWithPrefix(ctx, prefix)
	if err == nil {
		t.Fatal("DeleteWithPrefix() error = nil, want error")
	}
}

// TestKV_List_Success 测试 List 成功列出。
func TestKV_List_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := NewMocketcdClient(ctrl)
	c := newTestClient(t, mockClient)

	ctx := context.Background()
	prefix := "/app/config/"

	mockClient.EXPECT().
		Get(ctx, prefix, gomock.Any()).
		Return(&clientv3.GetResponse{
			Kvs: []*mvccpb.KeyValue{
				{Key: []byte("/app/config/key1"), Value: []byte("value1")},
				{Key: []byte("/app/config/key2"), Value: []byte("value2")},
			},
		}, nil)

	result, err := c.List(ctx, prefix)
	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}
	if len(result) != 2 {
		t.Errorf("List() len = %d, want 2", len(result))
	}
	if string(result["/app/config/key1"]) != "value1" {
		t.Errorf("List()[/app/config/key1] = %q, want %q", result["/app/config/key1"], "value1")
	}
}

// TestKV_List_Empty 测试 List 空结果。
func TestKV_List_Empty(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := NewMocketcdClient(ctrl)
	c := newTestClient(t, mockClient)

	ctx := context.Background()
	prefix := "/empty/"

	mockClient.EXPECT().
		Get(ctx, prefix, gomock.Any()).
		Return(&clientv3.GetResponse{Kvs: nil}, nil)

	result, err := c.List(ctx, prefix)
	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}
	if len(result) != 0 {
		t.Errorf("List() len = %d, want 0", len(result))
	}
}

// TestKV_List_Error 测试 List 发生错误。
func TestKV_List_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := NewMocketcdClient(ctrl)
	c := newTestClient(t, mockClient)

	ctx := context.Background()
	prefix := "/app/config/"
	expectedErr := errors.New("timeout")

	mockClient.EXPECT().
		Get(ctx, prefix, gomock.Any()).
		Return(nil, expectedErr)

	_, err := c.List(ctx, prefix)
	if err == nil {
		t.Fatal("List() error = nil, want error")
	}
}

// TestKV_ListKeys_Success 测试 ListKeys 成功列出键名。
func TestKV_ListKeys_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := NewMocketcdClient(ctrl)
	c := newTestClient(t, mockClient)

	ctx := context.Background()
	prefix := "/app/"

	mockClient.EXPECT().
		Get(ctx, prefix, gomock.Any(), gomock.Any()).
		Return(&clientv3.GetResponse{
			Kvs: []*mvccpb.KeyValue{
				{Key: []byte("/app/key1")},
				{Key: []byte("/app/key2")},
				{Key: []byte("/app/key3")},
			},
		}, nil)

	keys, err := c.ListKeys(ctx, prefix)
	if err != nil {
		t.Fatalf("ListKeys() error = %v, want nil", err)
	}
	if len(keys) != 3 {
		t.Errorf("ListKeys() len = %d, want 3", len(keys))
	}
}

// TestKV_ListKeys_Error 测试 ListKeys 发生错误。
func TestKV_ListKeys_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := NewMocketcdClient(ctrl)
	c := newTestClient(t, mockClient)

	ctx := context.Background()
	prefix := "/app/"
	expectedErr := errors.New("connection refused")

	mockClient.EXPECT().
		Get(ctx, prefix, gomock.Any(), gomock.Any()).
		Return(nil, expectedErr)

	_, err := c.ListKeys(ctx, prefix)
	if err == nil {
		t.Fatal("ListKeys() error = nil, want error")
	}
}

// TestKV_Exists_True 测试 Exists 键存在。
func TestKV_Exists_True(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := NewMocketcdClient(ctrl)
	c := newTestClient(t, mockClient)

	ctx := context.Background()
	key := "test-key"

	mockClient.EXPECT().
		Get(ctx, key, gomock.Any()).
		Return(&clientv3.GetResponse{Count: 1}, nil)

	exists, err := c.Exists(ctx, key)
	if err != nil {
		t.Fatalf("Exists() error = %v, want nil", err)
	}
	if !exists {
		t.Error("Exists() = false, want true")
	}
}

// TestKV_Exists_False 测试 Exists 键不存在。
func TestKV_Exists_False(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := NewMocketcdClient(ctrl)
	c := newTestClient(t, mockClient)

	ctx := context.Background()
	key := "non-existent-key"

	mockClient.EXPECT().
		Get(ctx, key, gomock.Any()).
		Return(&clientv3.GetResponse{Count: 0}, nil)

	exists, err := c.Exists(ctx, key)
	if err != nil {
		t.Fatalf("Exists() error = %v, want nil", err)
	}
	if exists {
		t.Error("Exists() = true, want false")
	}
}

// TestKV_Exists_Error 测试 Exists 发生错误。
func TestKV_Exists_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := NewMocketcdClient(ctrl)
	c := newTestClient(t, mockClient)

	ctx := context.Background()
	key := "test-key"
	expectedErr := errors.New("timeout")

	mockClient.EXPECT().
		Get(ctx, key, gomock.Any()).
		Return(nil, expectedErr)

	_, err := c.Exists(ctx, key)
	if err == nil {
		t.Fatal("Exists() error = nil, want error")
	}
}

// TestKV_Count_Success 测试 Count 成功计数。
func TestKV_Count_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := NewMocketcdClient(ctrl)
	c := newTestClient(t, mockClient)

	ctx := context.Background()
	prefix := "/app/"

	mockClient.EXPECT().
		Get(ctx, prefix, gomock.Any(), gomock.Any()).
		Return(&clientv3.GetResponse{Count: 42}, nil)

	count, err := c.Count(ctx, prefix)
	if err != nil {
		t.Fatalf("Count() error = %v, want nil", err)
	}
	if count != 42 {
		t.Errorf("Count() = %d, want 42", count)
	}
}

// TestKV_Count_Zero 测试 Count 返回 0。
func TestKV_Count_Zero(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := NewMocketcdClient(ctrl)
	c := newTestClient(t, mockClient)

	ctx := context.Background()
	prefix := "/empty/"

	mockClient.EXPECT().
		Get(ctx, prefix, gomock.Any(), gomock.Any()).
		Return(&clientv3.GetResponse{Count: 0}, nil)

	count, err := c.Count(ctx, prefix)
	if err != nil {
		t.Fatalf("Count() error = %v, want nil", err)
	}
	if count != 0 {
		t.Errorf("Count() = %d, want 0", count)
	}
}

// TestKV_Count_Error 测试 Count 发生错误。
func TestKV_Count_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := NewMocketcdClient(ctrl)
	c := newTestClient(t, mockClient)

	ctx := context.Background()
	prefix := "/app/"
	expectedErr := errors.New("connection refused")

	mockClient.EXPECT().
		Get(ctx, prefix, gomock.Any(), gomock.Any()).
		Return(nil, expectedErr)

	_, err := c.Count(ctx, prefix)
	if err == nil {
		t.Fatal("Count() error = nil, want error")
	}
}

// TestClient_RawClient 测试 RawClient 返回 nil（使用 mock 时）。
func TestClient_RawClient(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := NewMocketcdClient(ctrl)
	c := newTestClient(t, mockClient)

	// 使用 mock 时 rawClient 为 nil
	if c.RawClient() != nil {
		t.Error("RawClient() should be nil for mock-based client")
	}
}

// TestClient_Close_Success 测试 Close 成功关闭。
func TestClient_Close_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := NewMocketcdClient(ctrl)
	c := newTestClient(t, mockClient)

	mockClient.EXPECT().
		Close().
		Return(nil)

	err := c.Close()
	if err != nil {
		t.Fatalf("Close() error = %v, want nil", err)
	}

	// 验证已标记为关闭
	if !c.isClosed() {
		t.Error("isClosed() = false after Close(), want true")
	}

	// 第二次关闭不应该调用底层 Close
	err = c.Close()
	if err != nil {
		t.Fatalf("second Close() error = %v, want nil", err)
	}
}

// TestClient_Close_Error 测试 Close 发生错误。
func TestClient_Close_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := NewMocketcdClient(ctrl)
	c := newTestClient(t, mockClient)

	expectedErr := errors.New("close failed")
	mockClient.EXPECT().
		Close().
		Return(expectedErr)

	err := c.Close()
	if !errors.Is(err, expectedErr) {
		t.Errorf("Close() error = %v, want %v", err, expectedErr)
	}
}
