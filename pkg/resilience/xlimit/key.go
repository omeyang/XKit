package xlimit

import (
	"strings"
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

// Render 使用 Key 中的值渲染模板字符串
// 支持的变量：
//   - ${tenant_id}: Tenant 字段
//   - ${caller_id}: Caller 字段
//   - ${method}: Method 字段
//   - ${path}: Path 字段
//   - ${resource}: Resource 字段
//   - ${xxx}: Extra 中的自定义字段
//
// 未找到的变量保持原样
func (k Key) Render(template string) string {
	result := template

	// 替换标准字段
	result = strings.ReplaceAll(result, "${tenant_id}", k.Tenant)
	result = strings.ReplaceAll(result, "${caller_id}", k.Caller)
	result = strings.ReplaceAll(result, "${method}", k.Method)
	result = strings.ReplaceAll(result, "${path}", k.Path)
	result = strings.ReplaceAll(result, "${resource}", k.Resource)

	// 替换 Extra 中的自定义字段
	for key, value := range k.Extra {
		result = strings.ReplaceAll(result, "${"+key+"}", value)
	}

	return result
}

// String 返回 Key 的字符串表示，用于日志和调试
func (k Key) String() string {
	var parts []string

	if k.Tenant != "" {
		parts = append(parts, "tenant="+k.Tenant)
	}
	if k.Caller != "" {
		parts = append(parts, "caller="+k.Caller)
	}
	if k.Method != "" {
		parts = append(parts, "method="+k.Method)
	}
	if k.Path != "" {
		parts = append(parts, "path="+k.Path)
	}
	if k.Resource != "" {
		parts = append(parts, "resource="+k.Resource)
	}

	return strings.Join(parts, ",")
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
