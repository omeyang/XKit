package xdbg

import (
	"bytes"
	"encoding/binary"
	"testing"
	"unicode/utf8"
)

// =============================================================================
// 协议解码模糊测试
// =============================================================================

// FuzzCodecDecodeRequest 对 Codec.DecodeRequest 进行模糊测试。
//
// 协议头是 8 字节定长 + 变长 payload，是典型的模糊测试目标：
//   - 不能 panic
//   - 解码失败必须返回错误（而非 nil + 损坏数据）
//   - 解码成功的字节流必须能再次成功解码（语义稳定）
func FuzzCodecDecodeRequest(f *testing.F) {
	codec := NewCodec()

	// 合法 seed：完整的 Request 编码
	if msg, err := codec.EncodeRequest(&Request{Command: "ping"}); err == nil {
		f.Add(msg)
	}
	if msg, err := codec.EncodeRequest(&Request{Command: "echo", Args: []string{"a", "b"}}); err == nil {
		f.Add(msg)
	}

	// 边界 seed：各种残缺/异常字节流
	f.Add([]byte{})
	f.Add([]byte{0x00})
	f.Add([]byte{0xDB, 0x09, 0x01, 0x01, 0x00, 0x00, 0x00, 0x00}) // 空 payload
	f.Add([]byte{0xDB, 0x09, 0x99, 0x01, 0x00, 0x00, 0x00, 0x00}) // 错误版本
	f.Add([]byte{0xFF, 0xFF, 0x01, 0x01, 0x00, 0x00, 0x00, 0x00}) // 错误 magic
	// 长度字段超过 MaxPayloadSize
	hdr := make([]byte, 8)
	binary.BigEndian.PutUint16(hdr[0:2], ProtocolMagic)
	hdr[2] = ProtocolVersion
	hdr[3] = byte(MessageTypeRequest)
	binary.BigEndian.PutUint32(hdr[4:8], MaxPayloadSize+1)
	f.Add(hdr)

	f.Fuzz(func(t *testing.T, data []byte) {
		req, err := codec.DecodeRequest(bytes.NewReader(data))
		if err != nil {
			if req != nil {
				t.Fatalf("decode error but non-nil request: %v", err)
			}
			return
		}
		if req == nil {
			t.Fatal("decode succeeded but request is nil")
		}

		// 不变量：解码成功 → 重新编码 → 再次解码必须等价
		reencoded, err := codec.EncodeRequest(req)
		if err != nil {
			t.Fatalf("re-encode failed: %v", err)
		}
		req2, err := codec.DecodeRequest(bytes.NewReader(reencoded))
		if err != nil {
			t.Fatalf("re-decode failed: %v", err)
		}
		if req2.Command != req.Command || len(req2.Args) != len(req.Args) {
			t.Fatalf("round-trip mismatch: %+v vs %+v", req, req2)
		}
	})
}

// FuzzCodecParseHeader 对 parseHeader 进行模糊测试。
//
// parseHeader 接受任意 8 字节切片并解析协议头，目标：
//   - 不能 panic
//   - 短于 HeaderSize 的输入必须报错
//   - 解析成功的 length 必须 ≤ MaxPayloadSize
func FuzzCodecParseHeader(f *testing.F) {
	codec := NewCodec()

	f.Add([]byte{})
	f.Add(make([]byte, 7))
	f.Add([]byte{0xDB, 0x09, 0x01, 0x01, 0x00, 0x00, 0x00, 0x10})
	f.Add([]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})

	f.Fuzz(func(t *testing.T, header []byte) {
		_, length, err := codec.parseHeader(header)
		if len(header) < HeaderSize {
			if err == nil {
				t.Fatalf("short header (%d bytes) must error", len(header))
			}
			return
		}
		if err == nil && length > MaxPayloadSize {
			t.Fatalf("parsed length %d exceeds MaxPayloadSize", length)
		}
	})
}

// =============================================================================
// UTF-8 截断模糊测试
// =============================================================================

// FuzzTruncateUTF8 对 TruncateUTF8 进行模糊测试。
//
// TruncateUTF8 在字节边界截断字符串而不破坏多字节 UTF-8 字符，目标：
//   - 不能 panic
//   - 输出长度 ≤ maxBytes
//   - 输出必须是合法 UTF-8（前提：输入也是合法 UTF-8）
//   - 输出必须是输入的前缀
func FuzzTruncateUTF8(f *testing.F) {
	f.Add("hello", 3)
	f.Add("hello", 0)
	f.Add("hello", 100)
	f.Add("世界你好", 4)
	f.Add("世界你好", 5)
	f.Add("世界你好", 6)
	f.Add("a世b", 2)
	f.Add("", 10)
	f.Add("\xff\xfe", 1) // 非法 UTF-8

	f.Fuzz(func(t *testing.T, s string, maxBytes int) {
		if maxBytes < 0 || maxBytes > 1<<20 {
			t.Skip()
		}
		out := TruncateUTF8(s, maxBytes)

		if len(out) > maxBytes && len(s) > maxBytes {
			t.Fatalf("output len=%d exceeds maxBytes=%d (input len=%d)", len(out), maxBytes, len(s))
		}
		if !bytesHasPrefix(s, out) {
			t.Fatalf("output %q is not a prefix of input %q", out, s)
		}
		// 仅当输入是合法 UTF-8 时，输出也必须合法
		if utf8.ValidString(s) && !utf8.ValidString(out) {
			t.Fatalf("valid UTF-8 input %q produced invalid output %q", s, out)
		}
	})
}

func bytesHasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
