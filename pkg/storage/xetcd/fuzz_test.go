package xetcd

import (
	"context"
	"strings"
	"testing"

	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// FuzzEventType_String 模糊测试 EventType.String 方法。
// 验证该方法对任意 EventType 值都不会 panic。
func FuzzEventType_String(f *testing.F) {
	// 添加种子语料
	f.Add(int(EventPut))
	f.Add(int(EventDelete))
	f.Add(0)
	f.Add(-1)
	f.Add(100)
	f.Add(int(^uint(0) >> 1)) // max int

	f.Fuzz(func(t *testing.T, i int) {
		et := EventType(i)
		result := et.String()

		// 验证结果不为空
		if result == "" {
			t.Error("EventType.String() should never return empty string")
		}

		// 对于已知类型，验证返回值
		switch et {
		case EventPut:
			if result != "PUT" {
				t.Errorf("EventPut.String() = %q, want %q", result, "PUT")
			}
		case EventDelete:
			if result != "DELETE" {
				t.Errorf("EventDelete.String() = %q, want %q", result, "DELETE")
			}
		default:
			// 未知类型应该返回 UNKNOWN(x) 格式
			if len(result) < 7 || result[:7] != "UNKNOWN" {
				t.Errorf("Unknown EventType.String() = %q, should start with UNKNOWN", result)
			}
		}
	})
}

// FuzzConvertEvent 模糊测试 convertEvent 函数。
// 验证该函数对任意输入都不会 panic。
func FuzzConvertEvent(f *testing.F) {
	// 添加种子语料
	f.Add(int32(mvccpb.PUT), "key", "value", int64(1))
	f.Add(int32(mvccpb.DELETE), "deleted-key", "", int64(100))
	f.Add(int32(0), "", "", int64(0))
	f.Add(int32(99), "unknown", "unknown", int64(-1))

	f.Fuzz(func(t *testing.T, eventType int32, key, value string, revision int64) {
		etcdEvent := &clientv3.Event{
			Type: mvccpb.Event_EventType(eventType),
			Kv: &mvccpb.KeyValue{
				Key:         []byte(key),
				Value:       []byte(value),
				ModRevision: revision,
			},
		}

		// 不应该 panic
		event := convertEvent(etcdEvent)

		// 验证 Key 正确转换
		if event.Key != key {
			t.Errorf("convertEvent() Key = %q, want %q", event.Key, key)
		}

		// 验证 Revision 正确转换
		if event.Revision != revision {
			t.Errorf("convertEvent() Revision = %d, want %d", event.Revision, revision)
		}

		// 验证 DELETE 事件的 Value 为 nil
		if mvccpb.Event_EventType(eventType) == mvccpb.DELETE && event.Value != nil {
			t.Error("convertEvent() DELETE event should have nil Value")
		}
	})
}

// FuzzConfigValidate 模糊测试 Config.Validate 方法。
func FuzzConfigValidate(f *testing.F) {
	// 添加种子语料
	f.Add("")
	f.Add("localhost:2379")
	f.Add("host1:2379,host2:2379")
	f.Add("invalid")
	f.Add(":2379")
	f.Add("localhost:")

	f.Fuzz(func(t *testing.T, endpoint string) {
		cfg := &Config{}
		if endpoint != "" {
			cfg.Endpoints = []string{endpoint}
		}

		// Validate 不应该 panic
		err := cfg.Validate()

		// 空 endpoints 应该返回错误
		if len(cfg.Endpoints) == 0 && err != ErrNoEndpoints {
			t.Errorf("Validate() with empty endpoints = %v, want %v", err, ErrNoEndpoints)
		}

		// 非空 endpoints 验证规则：
		// - 必须包含 ":" 表示 host:port 格式
		// - 空字符串的 endpoint 应该返回错误
		if len(cfg.Endpoints) > 0 {
			ep := cfg.Endpoints[0]
			hasPort := strings.Contains(ep, ":")
			if ep == "" || !hasPort {
				// 无效格式应该返回错误
				if err == nil {
					t.Errorf("Validate() with invalid endpoint %q = nil, want error", ep)
				}
			} else {
				// 有效格式应该通过验证
				if err != nil {
					t.Errorf("Validate() with valid endpoint %q = %v, want nil", ep, err)
				}
			}
		}
	})
}

// FuzzBuildWatchOptions 模糊测试 buildWatchOptions 方法。
func FuzzBuildWatchOptions(f *testing.F) {
	// 添加种子语料
	f.Add(false, int64(0))
	f.Add(true, int64(0))
	f.Add(false, int64(100))
	f.Add(true, int64(100))
	f.Add(true, int64(-1))
	f.Add(false, int64(-100))

	f.Fuzz(func(t *testing.T, prefix bool, revision int64) {
		c := &Client{}
		opts := &watchOptions{
			prefix:   prefix,
			revision: revision,
		}

		// 不应该 panic
		result := c.buildWatchOptions(opts)

		// 验证选项数量
		expectedCount := 0
		if prefix {
			expectedCount++
		}
		if revision > 0 {
			expectedCount++
		}

		if len(result) != expectedCount {
			t.Errorf("buildWatchOptions() returned %d options, want %d (prefix=%v, revision=%d)",
				len(result), expectedCount, prefix, revision)
		}
	})
}

// FuzzDispatchEvents 模糊测试 dispatchEvents 方法。
func FuzzDispatchEvents(f *testing.F) {
	// 添加种子语料
	f.Add(0)
	f.Add(1)
	f.Add(10)
	f.Add(100)

	f.Fuzz(func(t *testing.T, eventCount int) {
		// 限制事件数量避免内存问题
		if eventCount < 0 {
			eventCount = 0
		}
		if eventCount > 1000 {
			eventCount = 1000
		}

		c := &Client{}
		ctx := context.Background()
		eventCh := make(chan Event, eventCount+1)

		events := make([]*clientv3.Event, eventCount)
		for i := 0; i < eventCount; i++ {
			events[i] = &clientv3.Event{
				Type: mvccpb.PUT,
				Kv: &mvccpb.KeyValue{
					Key:         []byte("key"),
					Value:       []byte("value"),
					ModRevision: int64(i),
				},
			}
		}

		// 不应该 panic
		_, result := c.dispatchEvents(ctx, events, eventCh)

		// 应该成功
		if !result {
			t.Error("dispatchEvents() should return true")
		}

		// 验证通道中的事件数量
		if len(eventCh) != eventCount {
			t.Errorf("dispatchEvents() sent %d events, want %d", len(eventCh), eventCount)
		}
	})
}

// FuzzWithRevision 模糊测试 WithRevision 选项函数。
func FuzzWithRevision(f *testing.F) {
	f.Add(int64(0))
	f.Add(int64(1))
	f.Add(int64(-1))
	f.Add(int64(9223372036854775807)) // max int64

	f.Fuzz(func(t *testing.T, rev int64) {
		opts := &watchOptions{}

		// 不应该 panic
		WithRevision(rev)(opts)

		// 验证值被正确设置
		if opts.revision != rev {
			t.Errorf("WithRevision(%d) set revision to %d", rev, opts.revision)
		}
	})
}

// FuzzIsKeyNotFound 模糊测试 IsKeyNotFound 函数。
func FuzzIsKeyNotFound(f *testing.F) {
	f.Add("")
	f.Add("some error")
	f.Add("xetcd: key not found")
	f.Add("wrapped: xetcd: key not found")

	f.Fuzz(func(t *testing.T, errMsg string) {
		var err error
		if errMsg != "" {
			err = &testError{msg: errMsg}
		}

		// 不应该 panic
		_ = IsKeyNotFound(err)
	})
}

// FuzzIsClientClosed 模糊测试 IsClientClosed 函数。
func FuzzIsClientClosed(f *testing.F) {
	f.Add("")
	f.Add("some error")
	f.Add("xetcd: client is closed")
	f.Add("wrapped: xetcd: client is closed")

	f.Fuzz(func(t *testing.T, errMsg string) {
		var err error
		if errMsg != "" {
			err = &testError{msg: errMsg}
		}

		// 不应该 panic
		_ = IsClientClosed(err)
	})
}

// testError 用于模糊测试的简单错误类型。
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}
