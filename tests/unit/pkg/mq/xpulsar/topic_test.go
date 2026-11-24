package xpulsar

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// ParseTopic 测试
// =============================================================================

func TestParseTopic(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     string
		want    Topic
		wantErr error
	}{
		{
			name: "persistent topic",
			raw:  "persistent://my-tenant/my-ns/my-topic",
			want: Topic{Persistent: true, Tenant: "my-tenant", Namespace: "my-ns", Name: "my-topic"},
		},
		{
			name: "non-persistent topic",
			raw:  "non-persistent://public/default/events",
			want: Topic{Persistent: false, Tenant: "public", Namespace: "default", Name: "events"},
		},
		{
			name: "topic with dots",
			raw:  "persistent://tenant/ns/topic.v2",
			want: Topic{Persistent: true, Tenant: "tenant", Namespace: "ns", Name: "topic.v2"},
		},
		{
			name: "topic with underscores and hyphens",
			raw:  "persistent://my_tenant/my-ns/my_topic-v2",
			want: Topic{Persistent: true, Tenant: "my_tenant", Namespace: "my-ns", Name: "my_topic-v2"},
		},
		{
			name: "leading and trailing whitespace",
			raw:  "  persistent://public/default/events  ",
			want: Topic{Persistent: true, Tenant: "public", Namespace: "default", Name: "events"},
		},
		{
			name:    "empty string",
			raw:     "",
			wantErr: ErrEmptyTopicURI,
		},
		{
			name:    "whitespace only",
			raw:     "   ",
			wantErr: ErrEmptyTopicURI,
		},
		{
			name:    "missing scheme",
			raw:     "my-tenant/my-ns/my-topic",
			wantErr: ErrInvalidTopicScheme,
		},
		{
			name:    "invalid scheme",
			raw:     "http://tenant/ns/topic",
			wantErr: ErrInvalidTopicScheme,
		},
		{
			name:    "missing parts",
			raw:     "persistent://tenant/ns",
			wantErr: ErrInvalidTopicFormat,
		},
		{
			name:    "too many parts",
			raw:     "persistent://tenant/ns/topic/extra",
			wantErr: ErrInvalidTopicFormat,
		},
		{
			name:    "empty tenant",
			raw:     "persistent:///ns/topic",
			wantErr: ErrInvalidTenant,
		},
		{
			name:    "empty namespace",
			raw:     "persistent://tenant//topic",
			wantErr: ErrInvalidNamespace,
		},
		{
			name:    "empty topic name",
			raw:     "persistent://tenant/ns/",
			wantErr: ErrInvalidTopicName,
		},
		{
			name:    "tenant with whitespace",
			raw:     "persistent://my tenant/ns/topic",
			wantErr: ErrInvalidTenant,
		},
		{
			name:    "namespace starts with hyphen",
			raw:     "persistent://tenant/-ns/topic",
			wantErr: ErrInvalidNamespace,
		},
		{
			name:    "topic name with special chars",
			raw:     "persistent://tenant/ns/topic@v2",
			wantErr: ErrInvalidTopicName,
		},
		{
			name:    "unicode tenant",
			raw:     "persistent://租户/ns/topic",
			wantErr: ErrInvalidTenant,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTopic(tt.raw)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Equal(t, Topic{}, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// =============================================================================
// NewTopic 测试
// =============================================================================

func TestNewTopic(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		tenant    string
		namespace string
		topicName string
		want      Topic
		wantErr   error
	}{
		{
			name:      "valid topic",
			tenant:    "public",
			namespace: "default",
			topicName: "events",
			want:      Topic{Persistent: true, Tenant: "public", Namespace: "default", Name: "events"},
		},
		{
			name:      "empty tenant",
			tenant:    "",
			namespace: "ns",
			topicName: "topic",
			wantErr:   ErrInvalidTenant,
		},
		{
			name:      "empty namespace",
			tenant:    "tenant",
			namespace: "",
			topicName: "topic",
			wantErr:   ErrInvalidNamespace,
		},
		{
			name:      "empty topic name",
			tenant:    "tenant",
			namespace: "ns",
			topicName: "",
			wantErr:   ErrInvalidTopicName,
		},
		{
			name:      "invalid tenant chars",
			tenant:    "ten ant",
			namespace: "ns",
			topicName: "topic",
			wantErr:   ErrInvalidTenant,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewTopic(tt.tenant, tt.namespace, tt.topicName)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Equal(t, Topic{}, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// =============================================================================
// Topic 方法测试
// =============================================================================

func TestTopic_URI(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		topic Topic
		want  string
	}{
		{
			name:  "persistent",
			topic: Topic{Persistent: true, Tenant: "public", Namespace: "default", Name: "events"},
			want:  "persistent://public/default/events",
		},
		{
			name:  "non-persistent",
			topic: Topic{Persistent: false, Tenant: "public", Namespace: "default", Name: "events"},
			want:  "non-persistent://public/default/events",
		},
		{
			name:  "zero value",
			topic: Topic{},
			want:  "non-persistent:////",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.topic.URI())
		})
	}
}

func TestTopic_String(t *testing.T) {
	t.Parallel()

	topic := Topic{Persistent: true, Tenant: "public", Namespace: "default", Name: "events"}
	assert.Equal(t, topic.URI(), topic.String())
}

func TestTopic_IsZero(t *testing.T) {
	t.Parallel()

	assert.True(t, Topic{}.IsZero())
	assert.False(t, Topic{Tenant: "t", Namespace: "n", Name: "n"}.IsZero())

	// 部分零值: 任一字段非空即为非零
	assert.False(t, Topic{Tenant: "t"}.IsZero())
	assert.False(t, Topic{Namespace: "n"}.IsZero())
	assert.False(t, Topic{Name: "n"}.IsZero())
}

func TestTopic_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		topic   Topic
		wantErr error
	}{
		{
			name:  "valid persistent",
			topic: Topic{Persistent: true, Tenant: "public", Namespace: "default", Name: "events"},
		},
		{
			name:  "valid non-persistent",
			topic: Topic{Persistent: false, Tenant: "t1", Namespace: "ns1", Name: "topic.v2"},
		},
		{
			name:    "empty tenant",
			topic:   Topic{Tenant: "", Namespace: "ns", Name: "topic"},
			wantErr: ErrInvalidTenant,
		},
		{
			name:    "empty namespace",
			topic:   Topic{Tenant: "t", Namespace: "", Name: "topic"},
			wantErr: ErrInvalidNamespace,
		},
		{
			name:    "empty name",
			topic:   Topic{Tenant: "t", Namespace: "ns", Name: ""},
			wantErr: ErrInvalidTopicName,
		},
		{
			name:    "tenant with invalid start",
			topic:   Topic{Tenant: "-bad", Namespace: "ns", Name: "topic"},
			wantErr: ErrInvalidTenant,
		},
		{
			name:    "topic name with slash",
			topic:   Topic{Tenant: "t", Namespace: "ns", Name: "a/b"},
			wantErr: ErrInvalidTopicName,
		},
		{
			name:    "topic name starts with dot",
			topic:   Topic{Tenant: "t", Namespace: "ns", Name: ".hidden"},
			wantErr: ErrInvalidTopicName,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.topic.Validate()
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

// =============================================================================
// 往返测试: ParseTopic → URI → ParseTopic
// =============================================================================

func TestTopic_RoundTrip(t *testing.T) {
	t.Parallel()

	uris := []string{
		"persistent://public/default/events",
		"non-persistent://my-tenant/my-ns/my-topic.v2",
		"persistent://t1/ns1/a_b-c",
	}

	for _, uri := range uris {
		t.Run(uri, func(t *testing.T) {
			topic, err := ParseTopic(uri)
			require.NoError(t, err)
			assert.Equal(t, uri, topic.URI())

			// 二次解析也一致
			topic2, err := ParseTopic(topic.URI())
			require.NoError(t, err)
			assert.Equal(t, topic, topic2)
		})
	}
}

// =============================================================================
// Fuzz 测试
// =============================================================================

// FuzzParseTopic 对 ParseTopic 进行模糊测试，验证:
//  1. 不会 panic（任意输入都返回 error 或 valid Topic）
//  2. 成功解析的 Topic 往返一致（ParseTopic → URI → ParseTopic）
//  3. Validate 与 ParseTopic 校验语义一致
func FuzzParseTopic(f *testing.F) {
	// 种子语料库：覆盖成功与失败路径
	seeds := []string{
		"persistent://public/default/events",
		"non-persistent://t/n/topic",
		"persistent://my-tenant/my-ns/my-topic.v2",
		"",
		"   ",
		"http://bad/scheme",
		"persistent://only/two",
		"persistent://a/b/c/d",
		"persistent:///n/t",
		"persistent://t//t",
		"persistent://t/n/",
		"persistent://-bad/n/t",
		"persistent://t/n/.hidden",
		"persistent://租户/ns/topic",
		"persistent://t/n/t@v2",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, raw string) {
		topic, err := ParseTopic(raw)
		if err != nil {
			// 失败路径：必须返回零值 Topic
			if !topic.IsZero() {
				t.Fatalf("ParseTopic(%q) returned error but non-zero topic: %+v", raw, topic)
			}
			return
		}

		// 成功路径：Validate 应与 ParseTopic 判定一致
		if vErr := topic.Validate(); vErr != nil {
			t.Fatalf("ParseTopic(%q) succeeded but Validate failed: %v", raw, vErr)
		}

		// 往返一致性：URI() 生成的字符串再次 ParseTopic 应得相同 Topic
		topic2, err2 := ParseTopic(topic.URI())
		if err2 != nil {
			t.Fatalf("round-trip ParseTopic(%q) failed: %v", topic.URI(), err2)
		}
		if topic != topic2 {
			t.Fatalf("round-trip mismatch: %+v != %+v", topic, topic2)
		}
	})
}

// FuzzValidateIdentifier 对 validateIdentifier 进行模糊测试，确保不 panic。
func FuzzValidateIdentifier(f *testing.F) {
	f.Add("valid")
	f.Add("")
	f.Add("-bad")
	f.Add("a.b")
	f.Add("a_b-c123")
	f.Add("租户")
	f.Add("a b")

	f.Fuzz(func(t *testing.T, value string) {
		// 仅验证不 panic；返回值不做断言。
		//nolint:errcheck // fuzz: error 值本身不是测试目标
		validateIdentifier(value, "test", ErrInvalidTenant)
	})
}
