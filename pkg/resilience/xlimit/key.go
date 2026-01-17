package xlimit

import (
	"context"
	"strings"

	"github.com/omeyang/xkit/pkg/context/xtenant"
)

// 模板变量名常量
const (
	varTenantID = "${tenant_id}"
	varCallerID = "${caller_id}"
	varMethod   = "${method}"
	varPath     = "${path}"
	varResource = "${resource}"
)

// Key 限流键，包含用于构建 Redis 键的各个维度
type Key struct {
	// Tenant 租户 ID，用于多租户限流
	Tenant string

	// Caller 上游调用方 ID，用于调用方限流
	Caller string

	// Method HTTP/gRPC 方法，如 GET、POST
	Method string

	// Path HTTP/gRPC 路径，如 /v1/users
	Path string

	// Resource 业务自定义资源名，如 createOrder
	Resource string

	// Extra 额外的自定义维度
	Extra map[string]string
}

// KeyFromContext 从 context 自动提取租户信息构建 Key
//
// 自动提取以下信息：
//   - Tenant: 从 xtenant.TenantID(ctx) 获取
//
// 返回的 Key 可以通过链式调用添加其他字段：
//
//	key := xlimit.KeyFromContext(ctx).
//	    WithMethod(r.Method).
//	    WithPath(r.URL.Path)
func KeyFromContext(ctx context.Context) Key {
	return Key{
		Tenant: xtenant.TenantID(ctx),
	}
}

// Render 使用 Key 中的值渲染模板字符串
// 支持的变量：
//   - ${tenant_id}: Tenant 字段
//   - ${caller_id}: Caller 字段
//   - ${method}: Method 字段
//   - ${path}: Path 字段
//   - ${resource}: Resource 字段
//   - ${xxx}: Extra 中的自定义字段
//
// 使用 strings.Builder 优化性能，避免多次字符串分配
func (k Key) Render(template string) string {
	// 快速路径：如果没有变量占位符，直接返回
	if !strings.Contains(template, "${") {
		return template
	}

	var b strings.Builder
	b.Grow(len(template) + 32) // 预分配空间

	i := 0
	for i < len(template) {
		// 查找变量开始位置
		start := strings.Index(template[i:], "${")
		if start == -1 {
			b.WriteString(template[i:])
			break
		}
		start += i

		// 写入变量前的文本
		b.WriteString(template[i:start])

		// 查找变量结束位置
		end := strings.Index(template[start:], "}")
		if end == -1 {
			b.WriteString(template[start:])
			break
		}
		end += start + 1

		// 提取变量名并替换
		varName := template[start:end]
		value := k.resolveVar(varName)
		b.WriteString(value)

		i = end
	}

	return b.String()
}

// resolveVar 解析变量值
func (k Key) resolveVar(varName string) string {
	switch varName {
	case varTenantID:
		return k.Tenant
	case varCallerID:
		return k.Caller
	case varMethod:
		return k.Method
	case varPath:
		return k.Path
	case varResource:
		return k.Resource
	default:
		// 检查 Extra 中的自定义字段
		if k.Extra != nil {
			// 提取变量名（去掉 ${ 和 }）
			name := varName[2 : len(varName)-1]
			if value, ok := k.Extra[name]; ok {
				return value
			}
		}
		// 未找到的变量保持原样
		return varName
	}
}

// String 返回 Key 的字符串表示，用于日志和调试
func (k Key) String() string {
	var b strings.Builder
	b.Grow(64)

	first := true
	appendField := func(name, value string) {
		if value == "" {
			return
		}
		if !first {
			b.WriteByte(',')
		}
		first = false
		b.WriteString(name)
		b.WriteByte('=')
		b.WriteString(value)
	}

	appendField("tenant", k.Tenant)
	appendField("caller", k.Caller)
	appendField("method", k.Method)
	appendField("path", k.Path)
	appendField("resource", k.Resource)

	for name, value := range k.Extra {
		appendField(name, value)
	}

	return b.String()
}

// IsEmpty 检查 Key 是否为空（所有字段都未设置）
func (k Key) IsEmpty() bool {
	return k.Tenant == "" &&
		k.Caller == "" &&
		k.Method == "" &&
		k.Path == "" &&
		k.Resource == "" &&
		len(k.Extra) == 0
}

// WithTenant 返回设置了 Tenant 的新 Key
func (k Key) WithTenant(tenant string) Key {
	k.Tenant = tenant
	return k
}

// WithCaller 返回设置了 Caller 的新 Key
func (k Key) WithCaller(caller string) Key {
	k.Caller = caller
	return k
}

// WithMethod 返回设置了 Method 的新 Key
func (k Key) WithMethod(method string) Key {
	k.Method = method
	return k
}

// WithPath 返回设置了 Path 的新 Key
func (k Key) WithPath(path string) Key {
	k.Path = path
	return k
}

// WithResource 返回设置了 Resource 的新 Key
func (k Key) WithResource(resource string) Key {
	k.Resource = resource
	return k
}

// WithExtra 返回添加了自定义维度的新 Key
func (k Key) WithExtra(key, value string) Key {
	if k.Extra == nil {
		k.Extra = make(map[string]string)
	}
	k.Extra[key] = value
	return k
}
