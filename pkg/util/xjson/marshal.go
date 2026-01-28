package xjson

import (
	"encoding/json"
	"fmt"
)

// Pretty 将任意值序列化为格式化的 JSON 字符串。
// 用于日志和调试输出。序列化失败时返回 "<marshal error: ...>"。
func Pretty(v any) string {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("<marshal error: %v>", err)
	}
	return string(data)
}
