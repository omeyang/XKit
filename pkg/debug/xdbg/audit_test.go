package xdbg

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestDefaultAuditLogger_Log(t *testing.T) {
	var buf bytes.Buffer
	logger := NewAuditLogger(&buf)

	record := &AuditRecord{
		Timestamp: time.Date(2026, 1, 22, 10, 30, 0, 0, time.UTC),
		Event:     AuditEventCommand,
		Identity: &IdentityInfo{
			PeerIdentity: &PeerIdentity{UID: 1000, GID: 1000, PID: 12345},
			Username:     "testuser",
			Groupname:    "testgroup",
		},
		Command:  "setlog",
		Args:     []string{"debug"},
		Duration: 100 * time.Millisecond,
	}

	logger.Log(record)

	output := buf.String()

	// 验证输出包含预期内容
	checks := []string{
		"2026-01-22T10:30:00.000Z",
		"[XDBG]",
		"[COMMAND]",
		"testuser",
		"command=setlog",
		"args=[debug]",
		"duration=100ms",
	}

	for _, check := range checks {
		if !strings.Contains(output, check) {
			t.Errorf("output should contain %q, got: %s", check, output)
		}
	}
}

func TestDefaultAuditLogger_LogWithError(t *testing.T) {
	var buf bytes.Buffer
	logger := NewAuditLogger(&buf)

	record := &AuditRecord{
		Timestamp: time.Now(),
		Event:     AuditEventCommandFailed,
		Command:   "test",
		Error:     "something went wrong",
	}

	logger.Log(record)

	output := buf.String()
	if !strings.Contains(output, `error="something went wrong"`) {
		t.Errorf("output should contain error, got: %s", output)
	}
}

func TestDefaultAuditLogger_LogWithExtra(t *testing.T) {
	var buf bytes.Buffer
	logger := NewAuditLogger(&buf)

	record := &AuditRecord{
		Timestamp: time.Now(),
		Event:     AuditEventServerStart,
		Extra: map[string]string{
			"socket": "/var/run/xdbg.sock",
		},
	}

	logger.Log(record)

	output := buf.String()
	if !strings.Contains(output, `socket="/var/run/xdbg.sock"`) {
		t.Errorf("output should contain extra info, got: %s", output)
	}
}

func TestDefaultAuditLogger_LogWithoutIdentity(t *testing.T) {
	var buf bytes.Buffer
	logger := NewAuditLogger(&buf)

	record := &AuditRecord{
		Timestamp: time.Now(),
		Event:     AuditEventServerStart,
	}

	logger.Log(record)

	output := buf.String()
	if !strings.Contains(output, "identity=unknown") {
		t.Errorf("output should contain identity=unknown, got: %s", output)
	}
}

func TestNewDefaultAuditLogger(t *testing.T) {
	logger := NewDefaultAuditLogger()
	if logger == nil {
		t.Error("NewDefaultAuditLogger() returned nil")
	}
}

func TestNoopAuditLogger(t *testing.T) {
	logger := NewNoopAuditLogger()

	// 应该不会 panic
	logger.Log(&AuditRecord{
		Timestamp: time.Now(),
		Event:     AuditEventCommand,
		Command:   "test",
	})

	err := logger.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestDefaultAuditLogger_Close(t *testing.T) {
	var buf bytes.Buffer
	logger := NewAuditLogger(&buf)

	err := logger.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestAuditEvent_Values(t *testing.T) {
	// 验证所有事件类型的值
	events := []AuditEvent{
		AuditEventServerStart,
		AuditEventServerStop,
		AuditEventSessionStart,
		AuditEventSessionEnd,
		AuditEventCommand,
		AuditEventCommandSuccess,
		AuditEventCommandFailed,
		AuditEventCommandForbidden,
	}

	for _, event := range events {
		if event == "" {
			t.Error("event should not be empty")
		}
	}
}

func TestAuditFormat_Values(t *testing.T) {
	if AuditFormatText != "text" {
		t.Errorf("AuditFormatText = %q, want %q", AuditFormatText, "text")
	}
	if AuditFormatJSON != "json" {
		t.Errorf("AuditFormatJSON = %q, want %q", AuditFormatJSON, "json")
	}
}

func TestJSONAuditLogger_Log(t *testing.T) {
	var buf bytes.Buffer
	logger := NewJSONAuditLogger(&buf)

	record := &AuditRecord{
		Timestamp: time.Date(2026, 1, 22, 10, 30, 0, 0, time.UTC),
		Event:     AuditEventCommand,
		Identity: &IdentityInfo{
			PeerIdentity: &PeerIdentity{UID: 1000, GID: 1000, PID: 12345},
			Username:     "testuser",
			Groupname:    "testgroup",
		},
		Command:  "setlog",
		Args:     []string{"debug"},
		Duration: 100 * time.Millisecond,
	}

	logger.Log(record)

	output := buf.String()

	// 验证输出是有效的 JSON
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v, output: %s", err, output)
	}

	// 验证 JSON 字段
	if parsed["event"] != string(AuditEventCommand) {
		t.Errorf("event = %v, want %s", parsed["event"], AuditEventCommand)
	}
	if parsed["command"] != "setlog" {
		t.Errorf("command = %v, want %s", parsed["command"], "setlog")
	}
	if parsed["duration"] != "100ms" {
		t.Errorf("duration = %v, want %s", parsed["duration"], "100ms")
	}
}

func TestJSONAuditLogger_LogWithError(t *testing.T) {
	var buf bytes.Buffer
	logger := NewJSONAuditLogger(&buf)

	record := &AuditRecord{
		Timestamp: time.Now(),
		Event:     AuditEventCommandFailed,
		Command:   "test",
		Error:     "something went wrong",
	}

	logger.Log(record)

	output := buf.String()

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	if parsed["error"] != "something went wrong" {
		t.Errorf("error = %v, want %s", parsed["error"], "something went wrong")
	}
}

func TestJSONAuditLogger_LogWithExtra(t *testing.T) {
	var buf bytes.Buffer
	logger := NewJSONAuditLogger(&buf)

	record := &AuditRecord{
		Timestamp: time.Now(),
		Event:     AuditEventServerStart,
		Extra: map[string]string{
			"socket": "/var/run/xdbg.sock",
		},
	}

	logger.Log(record)

	output := buf.String()

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	extra, ok := parsed["extra"].(map[string]interface{})
	if !ok {
		t.Fatalf("extra is not a map")
	}
	if extra["socket"] != "/var/run/xdbg.sock" {
		t.Errorf("extra[socket] = %v, want %s", extra["socket"], "/var/run/xdbg.sock")
	}
}

func TestJSONAuditLogger_LogWithoutDuration(t *testing.T) {
	var buf bytes.Buffer
	logger := NewJSONAuditLogger(&buf)

	record := &AuditRecord{
		Timestamp: time.Now(),
		Event:     AuditEventServerStart,
	}

	logger.Log(record)

	output := buf.String()

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	// duration 字段应该被省略（为空字符串时）
	if _, exists := parsed["duration"]; exists && parsed["duration"] != "" {
		t.Errorf("duration should be omitted or empty, got %v", parsed["duration"])
	}
}

func TestJSONAuditLogger_Close(t *testing.T) {
	var buf bytes.Buffer
	logger := NewJSONAuditLogger(&buf)

	err := logger.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestDefaultAuditSanitizer(t *testing.T) {
	args := []string{"arg1", "arg2", "arg3"}
	result := DefaultAuditSanitizer("test", args)

	// 默认脱敏函数应该返回原始参数
	if len(result) != len(args) {
		t.Errorf("DefaultAuditSanitizer() returned %d args, want %d", len(result), len(args))
	}
	for i, arg := range result {
		if arg != args[i] {
			t.Errorf("DefaultAuditSanitizer() arg[%d] = %q, want %q", i, arg, args[i])
		}
	}
}

func TestSanitizeArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "空参数",
			args: []string{},
			want: []string{},
		},
		{
			name: "nil参数",
			args: nil,
			want: nil,
		},
		{
			name: "单个参数",
			args: []string{"secret"},
			want: []string{"***"},
		},
		{
			name: "多个参数",
			args: []string{"password", "123456", "token"},
			want: []string{"***", "***", "***"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeArgs(tt.args)
			if len(got) != len(tt.want) {
				t.Errorf("SanitizeArgs() = %v, want %v", got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("SanitizeArgs()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestSanitizeArgs_DoesNotModifyOriginal(t *testing.T) {
	original := []string{"password", "secret"}
	originalCopy := []string{"password", "secret"}

	_ = SanitizeArgs(original)

	// 验证原始切片未被修改
	for i := range original {
		if original[i] != originalCopy[i] {
			t.Errorf("SanitizeArgs() modified original slice: got %q, want %q", original[i], originalCopy[i])
		}
	}
}
