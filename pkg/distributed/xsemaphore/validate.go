package xsemaphore

import (
	"fmt"
	"strings"
	"unicode"
)

// =============================================================================
// Key 前缀校验
// =============================================================================

// 禁止在 key 前缀中使用的字符
// - `{` 和 `}`: Redis Cluster hash tag 分隔符
//
// 原因：xsemaphore 使用 {resource} 作为 hash tag 确保同一资源的键在同一 slot。
// 如果 keyPrefix 包含 {}，会导致 hash tag 解析错误，例如：
//
//	keyPrefix = "app:{env}:"
//	最终 key = "app:{env}:{resource}:permits"
//	Redis 只识别第一个 {}，按 "env" 计算 slot，导致同一 resource 的键可能分到不同 slot
const invalidKeyPrefixChars = "{}"

// validateKeyPrefix 校验 key 前缀
//
// keyPrefix 不能包含 `{` 或 `}`，否则会破坏 Redis Cluster hash tag 机制。
// 返回 nil 表示校验通过
func validateKeyPrefix(prefix string) error {
	if idx := strings.IndexAny(prefix, invalidKeyPrefixChars); idx >= 0 {
		return fmt.Errorf("%w: key prefix cannot contain '%c' (found at position %d), "+
			"it would break Redis Cluster hash tag mechanism",
			ErrInvalidKeyPrefix, prefix[idx], idx)
	}
	return nil
}

// =============================================================================
// 资源名校验
// =============================================================================

// MaxResourceLength 资源名称的最大长度
const MaxResourceLength = 256

// 禁止在资源名中使用的字符
// - `{` 和 `}`: Redis Cluster hash tag 分隔符，会影响 slot 计算
// - `:`: Redis key 命名约定中的分隔符，可能导致 key 解析混乱
const invalidResourceChars = "{}:"

// validateResource 校验资源名称
//
// 资源名不能：
//   - 为空字符串
//   - 超过 MaxResourceLength (256) 字符
//   - 包含特殊字符 `{`, `}`, `:` （会破坏 Redis key 结构和 hash tag）
//   - 包含空白字符（空格、换行、制表符等）
//
// 返回 nil 表示校验通过
func validateResource(resource string) error {
	if resource == "" {
		return fmt.Errorf("%w: resource name cannot be empty", ErrInvalidResource)
	}

	if len(resource) > MaxResourceLength {
		return fmt.Errorf("%w: resource name exceeds max length %d (got %d)",
			ErrInvalidResource, MaxResourceLength, len(resource))
	}

	if idx := strings.IndexAny(resource, invalidResourceChars); idx >= 0 {
		return fmt.Errorf("%w: resource name cannot contain '%c' (found at position %d)",
			ErrInvalidResource, resource[idx], idx)
	}

	for i, r := range resource {
		if unicode.IsSpace(r) {
			return fmt.Errorf("%w: resource name cannot contain whitespace at position %d",
				ErrInvalidResource, i)
		}
	}

	return nil
}

// =============================================================================
// 租户 ID 校验
// =============================================================================

// MaxTenantIDLength 租户 ID 的最大长度
const MaxTenantIDLength = 256

// invalidTenantIDChars 禁止在租户 ID 中使用的字符
// 与 resource 一致：`{`, `}`, `:` 会破坏 Redis key 结构和 hash tag
const invalidTenantIDChars = "{}:"

// validateTenantID 校验租户 ID
//
// 空字符串是合法的（表示不使用租户配额），非空时：
//   - 不能超过 MaxTenantIDLength (256) 字符
//   - 不能包含特殊字符 `{`, `}`, `:` （会破坏 Redis key 结构和 hash tag）
//   - 不能包含空白字符
//
// 返回 nil 表示校验通过
func validateTenantID(tenantID string) error {
	if tenantID == "" {
		return nil // 空字符串合法，表示不使用租户配额
	}

	if len(tenantID) > MaxTenantIDLength {
		return fmt.Errorf("%w: tenant ID exceeds max length %d (got %d)",
			ErrInvalidTenantID, MaxTenantIDLength, len(tenantID))
	}

	if idx := strings.IndexAny(tenantID, invalidTenantIDChars); idx >= 0 {
		return fmt.Errorf("%w: tenant ID cannot contain '%c' (found at position %d)",
			ErrInvalidTenantID, tenantID[idx], idx)
	}

	for i, r := range tenantID {
		if unicode.IsSpace(r) {
			return fmt.Errorf("%w: tenant ID cannot contain whitespace at position %d",
				ErrInvalidTenantID, i)
		}
	}

	return nil
}
