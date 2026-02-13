package xdbg

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// AuditEvent 审计事件类型。
type AuditEvent string

const (
	// AuditEventServerStart 服务启动。
	AuditEventServerStart AuditEvent = "SERVER_START"

	// AuditEventServerStop 服务停止。
	AuditEventServerStop AuditEvent = "SERVER_STOP"

	// AuditEventSessionStart 会话开始。
	AuditEventSessionStart AuditEvent = "SESSION_START"

	// AuditEventSessionEnd 会话结束。
	AuditEventSessionEnd AuditEvent = "SESSION_END"

	// AuditEventCommand 命令执行。
	AuditEventCommand AuditEvent = "COMMAND"

	// AuditEventCommandSuccess 命令执行成功。
	AuditEventCommandSuccess AuditEvent = "COMMAND_SUCCESS"

	// AuditEventCommandFailed 命令执行失败。
	AuditEventCommandFailed AuditEvent = "COMMAND_FAILED"

	// AuditEventCommandForbidden 命令被禁止。
	AuditEventCommandForbidden AuditEvent = "COMMAND_FORBIDDEN"
)

// AuditRecord 审计记录。
type AuditRecord struct {
	// Timestamp 时间戳。
	Timestamp time.Time

	// Event 事件类型。
	Event AuditEvent

	// Identity 身份信息（可能为 nil）。
	Identity *IdentityInfo

	// Command 命令名（仅命令事件有值）。
	Command string

	// Args 命令参数（仅命令事件有值）。
	Args []string

	// Duration 执行耗时（仅命令完成事件有值）。
	Duration time.Duration

	// Error 错误信息（仅失败事件有值）。
	Error string

	// Extra 额外信息。
	Extra map[string]string
}

// AuditLogger 审计日志记录器接口。
type AuditLogger interface {
	// Log 记录审计事件。
	Log(record *AuditRecord)

	// Close 关闭记录器。
	Close() error
}

// AuditSanitizer 审计参数脱敏函数类型。
// 用于在记录审计日志前对敏感参数进行脱敏处理。
type AuditSanitizer func(command string, args []string) []string

// DefaultAuditSanitizer 默认脱敏函数。
// 默认不进行脱敏，直接返回原始参数。
//
// 设计决策: 默认透传而非"默认全遮蔽"。内置命令（setlog、stack、freemem、pprof）
// 的参数均不含敏感信息，"默认遮蔽"会让审计日志在大多数场景下丧失可读性。
// 如需对自定义命令参数脱敏，使用 WithAuditSanitizer 配置按命令/参数名过滤。
func DefaultAuditSanitizer(_ string, args []string) []string {
	return args
}

// SanitizeArgs 脱敏参数辅助函数。
// 将所有参数值替换为 "***"，保留参数个数但隐藏具体值。
// 可用于自定义 AuditSanitizer 实现中对敏感参数的处理。
func SanitizeArgs(args []string) []string {
	if len(args) == 0 {
		return args
	}
	sanitized := make([]string, len(args))
	for i := range args {
		sanitized[i] = "***"
	}
	return sanitized
}

// defaultAuditLogger 默认审计日志记录器（输出到 stderr）。
type defaultAuditLogger struct {
	mu     sync.Mutex
	writer io.Writer
}

// NewDefaultAuditLogger 创建默认审计日志记录器。
func NewDefaultAuditLogger() AuditLogger {
	return &defaultAuditLogger{
		writer: os.Stderr,
	}
}

// NewAuditLogger 创建自定义输出的审计日志记录器。
func NewAuditLogger(writer io.Writer) AuditLogger {
	return &defaultAuditLogger{
		writer: writer,
	}
}

// Log 记录审计事件。
func (l *defaultAuditLogger) Log(record *AuditRecord) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// 格式: [timestamp] [event] identity=xxx command=xxx args=xxx duration=xxx error=xxx
	ts := record.Timestamp.Format("2006-01-02T15:04:05.000Z07:00")

	var identity string
	if record.Identity != nil {
		identity = record.Identity.String()
	} else {
		identity = "unknown"
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "[%s] [XDBG] [%s] identity=%s", ts, record.Event, identity)

	if record.Command != "" {
		fmt.Fprintf(&sb, " command=%q", record.Command)
	}

	if len(record.Args) > 0 {
		fmt.Fprintf(&sb, " args=%q", record.Args)
	}

	if record.Duration > 0 {
		fmt.Fprintf(&sb, " duration=%s", record.Duration)
	}

	if record.Error != "" {
		fmt.Fprintf(&sb, " error=%q", record.Error)
	}

	for k, v := range record.Extra {
		fmt.Fprintf(&sb, " %q=%q", k, v)
	}

	line := sb.String()
	if _, err := fmt.Fprintln(l.writer, line); err != nil {
		// 如果 writer 不是 stderr，尝试写入 stderr 作为后备
		if l.writer != os.Stderr {
			fmt.Fprintf(os.Stderr, "[XDBG] audit log write failed: %v, original: %s\n", err, line)
		}
		// 如果 writer 就是 stderr，写入失败无法有效处理，静默忽略
	}
}

// Close 关闭记录器。
func (l *defaultAuditLogger) Close() error {
	return nil
}

// noopAuditLogger 空操作审计日志记录器。
type noopAuditLogger struct{}

// NewNoopAuditLogger 创建空操作审计日志记录器。
func NewNoopAuditLogger() AuditLogger {
	return &noopAuditLogger{}
}

// Log 空操作。
func (l *noopAuditLogger) Log(_ *AuditRecord) {}

// Close 空操作。
func (l *noopAuditLogger) Close() error { return nil }

// jsonAuditLogger JSON 格式审计日志记录器。
// 便于日志聚合系统（如 ELK、Loki）解析。
type jsonAuditLogger struct {
	mu     sync.Mutex
	writer io.Writer
}

// NewJSONAuditLogger 创建 JSON 格式审计日志记录器。
func NewJSONAuditLogger(writer io.Writer) AuditLogger {
	return &jsonAuditLogger{writer: writer}
}

// jsonAuditRecord JSON 审计记录结构（用于序列化）。
type jsonAuditRecord struct {
	Timestamp string            `json:"timestamp"`
	Event     AuditEvent        `json:"event"`
	Identity  *IdentityInfo     `json:"identity,omitempty"`
	Command   string            `json:"command,omitempty"`
	Args      []string          `json:"args,omitempty"`
	Duration  string            `json:"duration,omitempty"`
	Error     string            `json:"error,omitempty"`
	Extra     map[string]string `json:"extra,omitempty"`
}

// Log 记录审计事件（JSON 格式）。
func (l *jsonAuditLogger) Log(record *AuditRecord) {
	l.mu.Lock()
	defer l.mu.Unlock()

	jr := jsonAuditRecord{
		Timestamp: record.Timestamp.Format(time.RFC3339Nano),
		Event:     record.Event,
		Identity:  record.Identity,
		Command:   record.Command,
		Args:      record.Args,
		Error:     record.Error,
		Extra:     record.Extra,
	}

	if record.Duration > 0 {
		jr.Duration = record.Duration.String()
	}

	data, err := json.Marshal(jr)
	if err != nil {
		// JSON 序列化失败是代码 bug，输出到 stderr
		fmt.Fprintf(os.Stderr, "[XDBG] json marshal failed: %v\n", err)
		return
	}

	if _, err := fmt.Fprintln(l.writer, string(data)); err != nil {
		if l.writer != os.Stderr {
			fmt.Fprintf(os.Stderr, "[XDBG] audit log write failed: %v\n", err)
		}
	}
}

// Close 关闭记录器。
func (l *jsonAuditLogger) Close() error { return nil }
