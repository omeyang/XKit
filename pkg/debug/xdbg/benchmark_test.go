//go:build !windows

package xdbg

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
)

// BenchmarkCodec_EncodeRequest 测试请求编码性能。
func BenchmarkCodec_EncodeRequest(b *testing.B) {
	codec := NewCodec()
	req := &Request{
		Command: "test",
		Args:    []string{"arg1", "arg2", "arg3"},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := codec.EncodeRequest(req)
		if err != nil {
			b.Fatalf("EncodeRequest error = %v", err)
		}
	}
}

// BenchmarkCodec_DecodeRequest 测试请求解码性能。
func BenchmarkCodec_DecodeRequest(b *testing.B) {
	codec := NewCodec()
	req := &Request{
		Command: "test",
		Args:    []string{"arg1", "arg2", "arg3"},
	}
	data, _ := codec.EncodeRequest(req)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		r := bytes.NewReader(data)
		_, err := codec.DecodeRequest(r)
		if err != nil {
			b.Fatalf("DecodeRequest error = %v", err)
		}
	}
}

// BenchmarkCodec_EncodeResponse 测试响应编码性能。
func BenchmarkCodec_EncodeResponse(b *testing.B) {
	codec := NewCodec()
	resp := &Response{
		Success: true,
		Output:  "test output with some content that is reasonably sized",
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := codec.EncodeResponse(resp)
		if err != nil {
			b.Fatalf("EncodeResponse error = %v", err)
		}
	}
}

// BenchmarkCodec_DecodeResponse 测试响应解码性能。
func BenchmarkCodec_DecodeResponse(b *testing.B) {
	codec := NewCodec()
	resp := &Response{
		Success: true,
		Output:  "test output with some content that is reasonably sized",
	}
	data, _ := codec.EncodeResponse(resp)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		r := bytes.NewReader(data)
		_, err := codec.DecodeResponse(r)
		if err != nil {
			b.Fatalf("DecodeResponse error = %v", err)
		}
	}
}

// BenchmarkCodec_EncodeLargeResponse 测试大响应编码性能。
func BenchmarkCodec_EncodeLargeResponse(b *testing.B) {
	codec := NewCodec()
	// 创建 100KB 的输出
	largeOutput := make([]byte, 100*1024)
	for i := range largeOutput {
		largeOutput[i] = byte('a' + i%26)
	}
	resp := &Response{
		Success: true,
		Output:  string(largeOutput),
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := codec.EncodeResponse(resp)
		if err != nil {
			b.Fatalf("EncodeResponse error = %v", err)
		}
	}
}

// BenchmarkCommandRegistry_Get 测试命令查找性能。
func BenchmarkCommandRegistry_Get(b *testing.B) {
	reg := NewCommandRegistry()

	// 注册多个命令
	for i := 0; i < 20; i++ {
		name := "cmd" + string(rune('a'+i))
		reg.Register(NewCommandFunc(name, "test", func(_ context.Context, _ []string) (string, error) {
			return "", nil
		}))
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = reg.Get("cmdk") // 中间的命令
	}
}

// BenchmarkCommandRegistry_Has 测试命令存在检查性能。
func BenchmarkCommandRegistry_Has(b *testing.B) {
	reg := NewCommandRegistry()

	for i := 0; i < 20; i++ {
		name := "cmd" + string(rune('a'+i))
		reg.Register(NewCommandFunc(name, "test", func(_ context.Context, _ []string) (string, error) {
			return "", nil
		}))
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = reg.Has("cmdk")
	}
}

// BenchmarkCommandRegistry_Whitelist 测试带白名单的命令查找性能。
func BenchmarkCommandRegistry_Whitelist(b *testing.B) {
	whitelist := []string{"cmda", "cmdb", "cmdc", "cmdd", "cmde"}
	reg := NewCommandRegistry()
	reg.SetWhitelist(whitelist)

	for i := 0; i < 20; i++ {
		name := "cmd" + string(rune('a'+i))
		reg.Register(NewCommandFunc(name, "test", func(_ context.Context, _ []string) (string, error) {
			return "", nil
		}))
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = reg.Get("cmdc")
	}
}

// BenchmarkTruncateUTF8 测试 UTF-8 截断性能。
func BenchmarkTruncateUTF8(b *testing.B) {
	// 包含中文字符的长字符串
	input := "这是一个包含中文字符的测试字符串，用于测试UTF-8安全截断功能的性能表现。"
	// 重复多次
	for i := 0; i < 10; i++ {
		input += input
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = TruncateUTF8(input, 1000)
	}
}

// BenchmarkTruncateUTF8_ASCII 测试纯 ASCII 截断性能。
func BenchmarkTruncateUTF8_ASCII(b *testing.B) {
	input := "This is a test string for benchmarking UTF-8 truncation with ASCII only content."
	for i := 0; i < 10; i++ {
		input += input
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = TruncateUTF8(input, 1000)
	}
}

// BenchmarkJSONMarshal_Request 对比标准 JSON 编码性能。
func BenchmarkJSONMarshal_Request(b *testing.B) {
	req := &Request{
		Command: "test",
		Args:    []string{"arg1", "arg2", "arg3"},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := json.Marshal(req)
		if err != nil {
			b.Fatalf("json.Marshal error = %v", err)
		}
	}
}

// BenchmarkJSONUnmarshal_Request 对比标准 JSON 解码性能。
func BenchmarkJSONUnmarshal_Request(b *testing.B) {
	req := &Request{
		Command: "test",
		Args:    []string{"arg1", "arg2", "arg3"},
	}
	data, _ := json.Marshal(req)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		var r Request
		err := json.Unmarshal(data, &r)
		if err != nil {
			b.Fatalf("json.Unmarshal error = %v", err)
		}
	}
}

// BenchmarkIdentityInfo_String 测试身份信息字符串化性能。
func BenchmarkIdentityInfo_String(b *testing.B) {
	info := &IdentityInfo{
		PeerIdentity: &PeerIdentity{
			UID: 1000,
			GID: 1000,
			PID: 12345,
		},
		Username:  "testuser",
		Groupname: "testgroup",
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = info.String()
	}
}

// BenchmarkAuditRecord_Format 测试审计记录格式化性能。
func BenchmarkAuditRecord_Format(b *testing.B) {
	logger := NewDefaultAuditLogger()
	record := &AuditRecord{
		Event:    AuditEventCommand,
		Command:  "test",
		Args:     []string{"arg1", "arg2"},
		Duration: 100,
		Identity: &IdentityInfo{
			PeerIdentity: &PeerIdentity{
				UID: 1000,
				GID: 1000,
				PID: 12345,
			},
			Username:  "testuser",
			Groupname: "testgroup",
		},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		logger.Log(record)
	}
}
