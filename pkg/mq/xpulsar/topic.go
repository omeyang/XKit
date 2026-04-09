package xpulsar

import (
	"fmt"
	"strings"
)

// =============================================================================
// Topic 类型
// =============================================================================

// Topic 表示一个 Pulsar topic 的结构化信息。
//
// Pulsar topic URI 格式: {persistent|non-persistent}://tenant/namespace/topic
//
// 通过 ParseTopic 从 URI 解析，或通过 NewTopic 从字段构造（默认 persistent）。
type Topic struct {
	// Persistent 表示是否为持久化 topic。true 对应 "persistent://"，false 对应 "non-persistent://"。
	Persistent bool

	// Tenant 租户名称。仅允许字母、数字、连字符和下划线，不能以非字母数字字符开头。
	Tenant string

	// Namespace 命名空间。命名规则同 Tenant。
	Namespace string

	// Name topic 名称。允许字母、数字、连字符、下划线和点号，不能以非字母数字字符开头。
	Name string
}

// URI 方式与 scheme 前缀
const (
	persistentScheme    = "persistent://"
	nonPersistentScheme = "non-persistent://"
)

// ParseTopic 从 Pulsar topic URI 解析 Topic。
//
// 支持格式:
//   - persistent://tenant/namespace/topic
//   - non-persistent://tenant/namespace/topic
func ParseTopic(raw string) (Topic, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Topic{}, ErrEmptyTopicURI
	}

	persistent, path, ok := cutScheme(raw)
	if !ok {
		return Topic{}, fmt.Errorf("%w: %q", ErrInvalidTopicScheme, raw)
	}

	parts := strings.Split(path, "/")
	if len(parts) != 3 {
		return Topic{}, fmt.Errorf("%w: expected tenant/namespace/topic, got %q", ErrInvalidTopicFormat, path)
	}

	topic := Topic{
		Persistent: persistent,
		Tenant:     parts[0],
		Namespace:  parts[1],
		Name:       parts[2],
	}

	if err := topic.Validate(); err != nil {
		return Topic{}, err
	}
	return topic, nil
}

// NewTopic 从字段构造 Topic，默认为 persistent。
// 所有字段会经过 Validate 校验。
func NewTopic(tenant, namespace, name string) (Topic, error) {
	topic := Topic{
		Persistent: true,
		Tenant:     tenant,
		Namespace:  namespace,
		Name:       name,
	}
	if err := topic.Validate(); err != nil {
		return Topic{}, err
	}
	return topic, nil
}

// URI 返回标准的 Pulsar topic URI。
func (t Topic) URI() string {
	scheme := persistentScheme
	if !t.Persistent {
		scheme = nonPersistentScheme
	}
	return scheme + t.Tenant + "/" + t.Namespace + "/" + t.Name
}

// String 实现 fmt.Stringer，返回值与 URI() 一致。
//
// 注意: 零值 Topic 返回 "non-persistent:////"，在日志中可见但语义无效。
// 调用方若需区分零值，应先调用 IsZero() 判断。
func (t Topic) String() string {
	return t.URI()
}

// IsZero 返回 Topic 是否为零值（tenant/namespace/name 三者皆空）。
//
// 设计决策: 仅检查命名字段，不考虑 Persistent 布尔字段。
// 因此 Topic{Persistent: true} 也被视为零值——没有任何命名的 Topic
// 无法构成有效 URI，Persistent 标志单独存在无意义。
func (t Topic) IsZero() bool {
	return t.Tenant == "" && t.Namespace == "" && t.Name == ""
}

// Validate 校验 Topic 各字段是否满足 Pulsar 命名规则。
func (t Topic) Validate() error {
	if err := validateIdentifier(t.Tenant, "tenant", ErrInvalidTenant); err != nil {
		return err
	}
	if err := validateIdentifier(t.Namespace, "namespace", ErrInvalidNamespace); err != nil {
		return err
	}
	if err := validateTopicName(t.Name); err != nil {
		return err
	}
	return nil
}

// =============================================================================
// 内部辅助
// =============================================================================

// cutScheme 解析 URI scheme，返回 (isPersistent, path, ok)。
func cutScheme(raw string) (bool, string, bool) {
	if strings.HasPrefix(raw, persistentScheme) {
		return true, raw[len(persistentScheme):], true
	}
	if strings.HasPrefix(raw, nonPersistentScheme) {
		return false, raw[len(nonPersistentScheme):], true
	}
	return false, "", false
}

// validateIdentifier 校验 tenant/namespace 命名:
// 非空、以字母或数字开头、仅包含字母/数字/连字符/下划线。
func validateIdentifier(value, field string, sentinel error) error {
	if value == "" {
		return fmt.Errorf("%w: %s must not be empty", sentinel, field)
	}
	if !isAlphanumeric(value[0]) {
		return fmt.Errorf("%w: %s must start with alphanumeric, got %q", sentinel, field, value)
	}
	for i := 1; i < len(value); i++ {
		if !isIdentifierChar(value[i]) {
			return fmt.Errorf("%w: %s contains invalid character %q", sentinel, field, value[i:i+1])
		}
	}
	return nil
}

// validateTopicName 校验 topic 名称:
// 非空、以字母或数字开头、仅包含字母/数字/连字符/下划线/点号。
func validateTopicName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: name must not be empty", ErrInvalidTopicName)
	}
	if !isAlphanumeric(name[0]) {
		return fmt.Errorf("%w: name must start with alphanumeric, got %q", ErrInvalidTopicName, name)
	}
	for i := 1; i < len(name); i++ {
		if !isTopicNameChar(name[i]) {
			return fmt.Errorf("%w: name contains invalid character %q", ErrInvalidTopicName, name[i:i+1])
		}
	}
	return nil
}

func isAlphanumeric(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

func isIdentifierChar(c byte) bool {
	return isAlphanumeric(c) || c == '-' || c == '_'
}

func isTopicNameChar(c byte) bool {
	return isIdentifierChar(c) || c == '.'
}
