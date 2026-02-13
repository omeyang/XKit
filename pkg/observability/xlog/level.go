package xlog

import (
	"fmt"
	"log/slog"
	"strings"
)

// Level 日志级别，与 slog.Level 兼容
type Level slog.Level

// 日志级别常量，与 slog 保持一致
const (
	LevelDebug = Level(slog.LevelDebug)
	LevelInfo  = Level(slog.LevelInfo)
	LevelWarn  = Level(slog.LevelWarn)
	LevelError = Level(slog.LevelError)
)

// String 返回级别的字符串表示
//
// 对于标准级别返回大写名称（DEBUG/INFO/WARN/ERROR），
// 非标准级别委托给 slog.Level.String()（如 "INFO+2"）。
func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return slog.Level(l).String()
	}
}

// MarshalText 实现 encoding.TextMarshaler 接口
//
// 支持配置序列化场景（YAML/TOML/JSON）。
func (l Level) MarshalText() ([]byte, error) {
	return []byte(l.String()), nil
}

// UnmarshalText 实现 encoding.TextUnmarshaler 接口
//
// 支持从配置文件直接反序列化日志级别。
func (l *Level) UnmarshalText(data []byte) error {
	parsed, err := ParseLevel(string(data))
	if err != nil {
		return err
	}
	*l = parsed
	return nil
}

// ParseLevel 解析字符串为日志级别
// 支持 debug/info/warn/warning/error（大小写不敏感）
// 输入会自动 TrimSpace，与 SetFormat 行为一致
func ParseLevel(s string) (Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return LevelDebug, nil
	case "info":
		return LevelInfo, nil
	case "warn", "warning":
		return LevelWarn, nil
	case "error":
		return LevelError, nil
	default:
		return LevelInfo, fmt.Errorf("xlog: unknown level %q", s)
	}
}
