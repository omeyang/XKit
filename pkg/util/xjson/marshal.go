package xjson

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ErrMarshal 表示 JSON 序列化失败。
// 可通过 [errors.Is] 判断错误类型：
//
//	_, err := xjson.PrettyE(v)
//	if errors.Is(err, xjson.ErrMarshal) { ... }
var ErrMarshal = errors.New("json marshal")

// PrettyE 将任意值序列化为格式化（缩进两空格）的 JSON 字符串。
// 序列化失败时返回空字符串和 [ErrMarshal] 包装的错误，
// 适用于需要可靠区分成功与失败的场景。
func PrettyE(v any) (string, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrMarshal, err)
	}
	return string(data), nil
}

// Pretty 将任意值序列化为格式化的 JSON 字符串。
// 用于日志和调试输出，不需要调用方处理错误。
//
// 设计决策: Pretty 保留 string 单返回值签名，因为在日志/调试场景中
// 调用方通常不关心错误（如 log.Info("config", xjson.Pretty(cfg))）。
// 需要错误处理时使用 [PrettyE]。序列化失败时返回 "<marshal error: ...>"
// 而非空字符串，便于在日志中识别问题。
func Pretty(v any) string {
	s, err := PrettyE(v)
	if err != nil {
		return fmt.Sprintf("<marshal error: %v>", err)
	}
	return s
}
