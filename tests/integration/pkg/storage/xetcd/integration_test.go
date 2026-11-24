//go:build integration

package xetcd

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// 集成测试需要真实的 etcd 服务。
// 运行方式: go test -tags=integration -v ./pkg/storage/xetcd/...
//
// 环境变量:
//   - ETCD_ENDPOINTS: etcd 端点（逗号分隔），默认 "localhost:2379"
//   - ETCD_USERNAME: etcd 用户名（可选）
//   - ETCD_PASSWORD: etcd 密码（可选）

func getTestEndpoints() []string {
	endpoints := os.Getenv("ETCD_ENDPOINTS")
	if endpoints == "" {
		return []string{"localhost:2379"}
	}
	return strings.Split(endpoints, ",")
}

func getTestConfig() *Config {
	return &Config{
		Endpoints:   getTestEndpoints(),
		Username:    os.Getenv("ETCD_USERNAME"),
		Password:    os.Getenv("ETCD_PASSWORD"),
		DialTimeout: 5 * time.Second,
	}
}

func createTestClient(t *testing.T) *Client {
	t.Helper()
	client, err := NewClient(getTestConfig(), WithHealthCheck(true, 5*time.Second))
	if err != nil {
		t.Skipf("Skipping integration test: cannot connect to etcd: %v", err)
	}
	t.Cleanup(func() {
		_ = client.Close(context.Background())
	})
	return client
}

// TestIntegration_NewClient 测试客户端创建。
func TestIntegration_NewClient(t *testing.T) {
	client := createTestClient(t)

	if client == nil {
		t.Fatal("NewClient returned nil")
	}

	if client.RawClient() == nil {
		t.Error("RawClient() should not return nil")
	}
}

// TestIntegration_PutGet 测试基本的 Put 和 Get 操作。
func TestIntegration_PutGet(t *testing.T) {
	client := createTestClient(t)
	ctx := context.Background()

	key := "xetcd-test/put-get-" + time.Now().Format("20060102150405")
	value := []byte("test-value")

	// Put
	if err := client.Put(ctx, key, value); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	// 清理
	t.Cleanup(func() {
		_ = client.Delete(context.Background(), key)
	})

	// Get
	got, err := client.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if string(got) != string(value) {
		t.Errorf("Get() = %q, want %q", got, value)
	}
}

// TestIntegration_GetWithRevision 测试带版本号的 Get 操作。
func TestIntegration_GetWithRevision(t *testing.T) {
	client := createTestClient(t)
	ctx := context.Background()

	key := "xetcd-test/get-revision-" + time.Now().Format("20060102150405")
	value := []byte("test-value")

	// Put
	if err := client.Put(ctx, key, value); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	// 清理
	t.Cleanup(func() {
		_ = client.Delete(context.Background(), key)
	})

	// GetWithRevision
	got, rev, err := client.GetWithRevision(ctx, key)
	if err != nil {
		t.Fatalf("GetWithRevision() error = %v", err)
	}
	if string(got) != string(value) {
		t.Errorf("GetWithRevision() value = %q, want %q", got, value)
	}
	if rev <= 0 {
		t.Errorf("GetWithRevision() revision = %d, want > 0", rev)
	}
}

// TestIntegration_GetNotFound 测试 Get 不存在的键。
func TestIntegration_GetNotFound(t *testing.T) {
	client := createTestClient(t)
	ctx := context.Background()

	key := "xetcd-test/not-found-" + time.Now().Format("20060102150405.000000")

	_, err := client.Get(ctx, key)
	if err != ErrKeyNotFound {
		t.Errorf("Get() non-existent key error = %v, want %v", err, ErrKeyNotFound)
	}

	if !IsKeyNotFound(err) {
		t.Error("IsKeyNotFound() should return true")
	}
}

// TestIntegration_PutWithTTL 测试带 TTL 的 Put 操作。
func TestIntegration_PutWithTTL(t *testing.T) {
	client := createTestClient(t)
	ctx := context.Background()

	key := "xetcd-test/ttl-" + time.Now().Format("20060102150405")
	value := []byte("ttl-value")
	ttl := 2 * time.Second

	// PutWithTTL
	if err := client.PutWithTTL(ctx, key, value, ttl); err != nil {
		t.Fatalf("PutWithTTL() error = %v", err)
	}

	// 验证键存在
	got, err := client.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get() after PutWithTTL error = %v", err)
	}
	if string(got) != string(value) {
		t.Errorf("Get() = %q, want %q", got, value)
	}

	// 等待 TTL 过期
	time.Sleep(ttl + time.Second)

	// 验证键已被删除
	_, err = client.Get(ctx, key)
	if !IsKeyNotFound(err) {
		t.Errorf("Get() after TTL expiry error = %v, want ErrKeyNotFound", err)
	}
}

// TestIntegration_Delete 测试 Delete 操作。
func TestIntegration_Delete(t *testing.T) {
	client := createTestClient(t)
	ctx := context.Background()

	key := "xetcd-test/delete-" + time.Now().Format("20060102150405")
	value := []byte("delete-value")

	// Put
	if err := client.Put(ctx, key, value); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	// Delete
	if err := client.Delete(ctx, key); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// 验证键不存在
	_, err := client.Get(ctx, key)
	if !IsKeyNotFound(err) {
		t.Errorf("Get() after Delete error = %v, want ErrKeyNotFound", err)
	}
}

// TestIntegration_DeleteWithPrefix 测试前缀删除操作。
func TestIntegration_DeleteWithPrefix(t *testing.T) {
	client := createTestClient(t)
	ctx := context.Background()

	prefix := "xetcd-test/prefix-delete-" + time.Now().Format("20060102150405") + "/"

	// 创建多个键
	keys := []string{
		prefix + "key1",
		prefix + "key2",
		prefix + "key3",
	}

	for _, key := range keys {
		if err := client.Put(ctx, key, []byte("value")); err != nil {
			t.Fatalf("Put(%s) error = %v", key, err)
		}
	}

	// DeleteWithPrefix
	deleted, err := client.DeleteWithPrefix(ctx, prefix)
	if err != nil {
		t.Fatalf("DeleteWithPrefix() error = %v", err)
	}
	if deleted != int64(len(keys)) {
		t.Errorf("DeleteWithPrefix() deleted = %d, want %d", deleted, len(keys))
	}

	// 验证所有键都被删除
	for _, key := range keys {
		_, err := client.Get(ctx, key)
		if !IsKeyNotFound(err) {
			t.Errorf("Get(%s) after DeleteWithPrefix error = %v, want ErrKeyNotFound", key, err)
		}
	}
}

// TestIntegration_List 测试列表操作。
func TestIntegration_List(t *testing.T) {
	client := createTestClient(t)
	ctx := context.Background()

	prefix := "xetcd-test/list-" + time.Now().Format("20060102150405") + "/"

	// 创建多个键
	testData := map[string][]byte{
		prefix + "key1": []byte("value1"),
		prefix + "key2": []byte("value2"),
		prefix + "key3": []byte("value3"),
	}

	for key, value := range testData {
		if err := client.Put(ctx, key, value); err != nil {
			t.Fatalf("Put(%s) error = %v", key, err)
		}
	}

	// 清理
	t.Cleanup(func() {
		_, _ = client.DeleteWithPrefix(context.Background(), prefix)
	})

	// List
	result, err := client.List(ctx, prefix)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(result) != len(testData) {
		t.Errorf("List() returned %d items, want %d", len(result), len(testData))
	}

	for key, expectedValue := range testData {
		if gotValue, ok := result[key]; !ok {
			t.Errorf("List() missing key %s", key)
		} else if string(gotValue) != string(expectedValue) {
			t.Errorf("List()[%s] = %q, want %q", key, gotValue, expectedValue)
		}
	}
}

// TestIntegration_ListKeys 测试仅列出键名。
func TestIntegration_ListKeys(t *testing.T) {
	client := createTestClient(t)
	ctx := context.Background()

	prefix := "xetcd-test/list-keys-" + time.Now().Format("20060102150405") + "/"

	// 创建多个键
	keys := []string{
		prefix + "key1",
		prefix + "key2",
		prefix + "key3",
	}

	for _, key := range keys {
		if err := client.Put(ctx, key, []byte("value")); err != nil {
			t.Fatalf("Put(%s) error = %v", key, err)
		}
	}

	// 清理
	t.Cleanup(func() {
		_, _ = client.DeleteWithPrefix(context.Background(), prefix)
	})

	// ListKeys
	result, err := client.ListKeys(ctx, prefix)
	if err != nil {
		t.Fatalf("ListKeys() error = %v", err)
	}
	if len(result) != len(keys) {
		t.Errorf("ListKeys() returned %d keys, want %d", len(result), len(keys))
	}
}

// TestIntegration_Exists 测试键存在性检查。
func TestIntegration_Exists(t *testing.T) {
	client := createTestClient(t)
	ctx := context.Background()

	key := "xetcd-test/exists-" + time.Now().Format("20060102150405")

	// 检查不存在的键
	exists, err := client.Exists(ctx, key)
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}
	if exists {
		t.Error("Exists() should return false for non-existent key")
	}

	// 创建键
	if err := client.Put(ctx, key, []byte("value")); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	// 清理
	t.Cleanup(func() {
		_ = client.Delete(context.Background(), key)
	})

	// 检查存在的键
	exists, err = client.Exists(ctx, key)
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}
	if !exists {
		t.Error("Exists() should return true for existing key")
	}
}

// TestIntegration_Count 测试键数量统计。
func TestIntegration_Count(t *testing.T) {
	client := createTestClient(t)
	ctx := context.Background()

	prefix := "xetcd-test/count-" + time.Now().Format("20060102150405") + "/"

	// 创建多个键
	count := 5
	for i := 0; i < count; i++ {
		key := prefix + string(rune('a'+i))
		if err := client.Put(ctx, key, []byte("value")); err != nil {
			t.Fatalf("Put(%s) error = %v", key, err)
		}
	}

	// 清理
	t.Cleanup(func() {
		_, _ = client.DeleteWithPrefix(context.Background(), prefix)
	})

	// Count
	result, err := client.Count(ctx, prefix)
	if err != nil {
		t.Fatalf("Count() error = %v", err)
	}
	if result != int64(count) {
		t.Errorf("Count() = %d, want %d", result, count)
	}
}

// TestIntegration_Watch 测试 Watch 功能。
func TestIntegration_Watch(t *testing.T) {
	client := createTestClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	key := "xetcd-test/watch-" + time.Now().Format("20060102150405")

	// 启动 Watch
	eventCh, err := client.Watch(ctx, key)
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}

	// 在后台写入数据
	go func() {
		time.Sleep(100 * time.Millisecond)
		_ = client.Put(context.Background(), key, []byte("watch-value"))
		time.Sleep(100 * time.Millisecond)
		_ = client.Delete(context.Background(), key)
	}()

	// 接收事件
	var events []Event
	timeout := time.After(5 * time.Second)

	for len(events) < 2 {
		select {
		case event, ok := <-eventCh:
			if !ok {
				t.Fatal("Watch channel closed unexpectedly")
			}
			events = append(events, event)
		case <-timeout:
			t.Fatalf("Watch timeout, received %d events, want 2", len(events))
		}
	}

	// 验证事件
	if len(events) < 2 {
		t.Fatalf("Watch received %d events, want >= 2", len(events))
	}

	if events[0].Type != EventPut {
		t.Errorf("First event type = %v, want EventPut", events[0].Type)
	}
	if events[1].Type != EventDelete {
		t.Errorf("Second event type = %v, want EventDelete", events[1].Type)
	}
}

// TestIntegration_WatchWithPrefix 测试前缀 Watch。
func TestIntegration_WatchWithPrefix(t *testing.T) {
	client := createTestClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	prefix := "xetcd-test/watch-prefix-" + time.Now().Format("20060102150405") + "/"

	// 启动带前缀的 Watch
	eventCh, err := client.Watch(ctx, prefix, WithPrefix())
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}

	// 在后台写入多个键
	go func() {
		time.Sleep(100 * time.Millisecond)
		_ = client.Put(context.Background(), prefix+"key1", []byte("value1"))
		time.Sleep(50 * time.Millisecond)
		_ = client.Put(context.Background(), prefix+"key2", []byte("value2"))
		time.Sleep(50 * time.Millisecond)
		_, _ = client.DeleteWithPrefix(context.Background(), prefix)
	}()

	// 接收事件
	var events []Event
	timeout := time.After(5 * time.Second)

	for len(events) < 4 {
		select {
		case event, ok := <-eventCh:
			if !ok {
				break
			}
			events = append(events, event)
		case <-timeout:
			break
		}
	}

	// 应该收到至少 4 个事件（2 个 PUT + 2 个 DELETE）
	if len(events) < 4 {
		t.Logf("Received %d events (expected >= 4)", len(events))
	}
}

// TestIntegration_ClientClose 测试客户端关闭。
func TestIntegration_ClientClose(t *testing.T) {
	client, err := NewClient(getTestConfig())
	if err != nil {
		t.Skipf("Skipping: cannot connect to etcd: %v", err)
	}

	// 关闭客户端
	if err := client.Close(context.Background()); err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// 验证客户端已关闭
	if !client.isClosed() {
		t.Error("isClosed() should return true after Close()")
	}

	// 操作应该返回 ErrClientClosed
	_, err = client.Get(context.Background(), "test")
	if err != ErrClientClosed {
		t.Errorf("Get() after Close() error = %v, want %v", err, ErrClientClosed)
	}
}
