package xdbg

import (
	"bytes"
	"encoding/binary"
	"errors"
	"strings"
	"testing"
)

func TestCodec_EncodeDecodeRequest(t *testing.T) {
	codec := NewCodec()

	tests := []struct {
		name string
		req  *Request
	}{
		{
			name: "simple command",
			req: &Request{
				Command: "help",
			},
		},
		{
			name: "command with args",
			req: &Request{
				Command: "setlog",
				Args:    []string{"debug"},
			},
		},
		{
			name: "command with multiple args",
			req: &Request{
				Command: "pprof",
				Args:    []string{"cpu", "start"},
			},
		},
		{
			name: "command with empty args",
			req: &Request{
				Command: "stack",
				Args:    []string{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 编码
			data, err := codec.EncodeRequest(tt.req)
			if err != nil {
				t.Fatalf("EncodeRequest() error = %v", err)
			}

			// 解码
			reader := bytes.NewReader(data)
			got, err := codec.DecodeRequest(reader)
			if err != nil {
				t.Fatalf("DecodeRequest() error = %v", err)
			}

			// 验证
			if got.Command != tt.req.Command {
				t.Errorf("Command = %q, want %q", got.Command, tt.req.Command)
			}
			if len(got.Args) != len(tt.req.Args) {
				t.Errorf("Args length = %d, want %d", len(got.Args), len(tt.req.Args))
			}
			for i := range got.Args {
				if got.Args[i] != tt.req.Args[i] {
					t.Errorf("Args[%d] = %q, want %q", i, got.Args[i], tt.req.Args[i])
				}
			}
		})
	}
}

func TestCodec_EncodeDecodeResponse(t *testing.T) {
	codec := NewCodec()

	tests := []struct {
		name string
		resp *Response
	}{
		{
			name: "success response",
			resp: NewSuccessResponse("OK"),
		},
		{
			name: "error response",
			resp: NewErrorResponse(errors.New("test error")),
		},
		{
			name: "truncated response",
			resp: NewTruncatedResponse("truncated...", 1000000),
		},
		{
			name: "success with long output",
			resp: NewSuccessResponse(strings.Repeat("a", 10000)),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 编码
			data, err := codec.EncodeResponse(tt.resp)
			if err != nil {
				t.Fatalf("EncodeResponse() error = %v", err)
			}

			// 解码
			reader := bytes.NewReader(data)
			got, err := codec.DecodeResponse(reader)
			if err != nil {
				t.Fatalf("DecodeResponse() error = %v", err)
			}

			// 验证
			if got.Success != tt.resp.Success {
				t.Errorf("Success = %v, want %v", got.Success, tt.resp.Success)
			}
			if got.Output != tt.resp.Output {
				t.Errorf("Output = %q, want %q", got.Output, tt.resp.Output)
			}
			if got.Error != tt.resp.Error {
				t.Errorf("Error = %q, want %q", got.Error, tt.resp.Error)
			}
			if got.Truncated != tt.resp.Truncated {
				t.Errorf("Truncated = %v, want %v", got.Truncated, tt.resp.Truncated)
			}
			if got.OriginalSize != tt.resp.OriginalSize {
				t.Errorf("OriginalSize = %d, want %d", got.OriginalSize, tt.resp.OriginalSize)
			}
		})
	}
}

func TestCodec_DecodeHeader_InvalidMagic(t *testing.T) {
	codec := NewCodec()

	// 创建无效的 magic
	header := make([]byte, HeaderSize)
	binary.BigEndian.PutUint16(header[0:2], 0xFFFF) // 无效 magic
	header[2] = ProtocolVersion
	header[3] = byte(MessageTypeRequest)
	binary.BigEndian.PutUint32(header[4:8], 0)

	reader := bytes.NewReader(header)
	_, _, err := codec.DecodeHeader(reader)

	if !errors.Is(err, ErrInvalidMessage) {
		t.Errorf("expected ErrInvalidMessage, got %v", err)
	}
}

func TestCodec_DecodeHeader_UnsupportedVersion(t *testing.T) {
	codec := NewCodec()

	// 创建无效的版本
	header := make([]byte, HeaderSize)
	binary.BigEndian.PutUint16(header[0:2], ProtocolMagic)
	header[2] = 0xFF // 无效版本
	header[3] = byte(MessageTypeRequest)
	binary.BigEndian.PutUint32(header[4:8], 0)

	reader := bytes.NewReader(header)
	_, _, err := codec.DecodeHeader(reader)

	if !errors.Is(err, ErrInvalidMessage) {
		t.Errorf("expected ErrInvalidMessage, got %v", err)
	}
}

func TestCodec_DecodeHeader_MessageTooLarge(t *testing.T) {
	codec := NewCodec()

	// 创建超大消息头
	header := make([]byte, HeaderSize)
	binary.BigEndian.PutUint16(header[0:2], ProtocolMagic)
	header[2] = ProtocolVersion
	header[3] = byte(MessageTypeRequest)
	binary.BigEndian.PutUint32(header[4:8], MaxPayloadSize+1)

	reader := bytes.NewReader(header)
	_, _, err := codec.DecodeHeader(reader)

	if !errors.Is(err, ErrMessageTooLarge) {
		t.Errorf("expected ErrMessageTooLarge, got %v", err)
	}
}

func TestCodec_DecodeHeader_ConnectionClosed(t *testing.T) {
	codec := NewCodec()

	// 空 reader
	reader := bytes.NewReader([]byte{})
	_, _, err := codec.DecodeHeader(reader)

	if !errors.Is(err, ErrConnectionClosed) {
		t.Errorf("expected ErrConnectionClosed, got %v", err)
	}
}

func TestTruncateUTF8(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxBytes int
		want     string
	}{
		{
			name:     "no truncation needed",
			input:    "hello",
			maxBytes: 10,
			want:     "hello",
		},
		{
			name:     "simple truncation",
			input:    "hello world",
			maxBytes: 5,
			want:     "hello",
		},
		{
			name:     "chinese characters",
			input:    "你好世界",
			maxBytes: 6, // 每个中文字符 3 字节
			want:     "你好",
		},
		{
			name:     "chinese truncate at boundary",
			input:    "你好世界",
			maxBytes: 7, // 不能完整放下 "世"
			want:     "你好",
		},
		{
			name:     "mixed content",
			input:    "hello你好",
			maxBytes: 8, // "hello" = 5, "你" = 3
			want:     "hello你",
		},
		{
			name:     "empty string",
			input:    "",
			maxBytes: 10,
			want:     "",
		},
		{
			name:     "zero max bytes",
			input:    "hello",
			maxBytes: 0,
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TruncateUTF8(tt.input, tt.maxBytes)
			if got != tt.want {
				t.Errorf("TruncateUTF8() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTruncateOutput(t *testing.T) {
	tests := []struct {
		name         string
		output       string
		maxBytes     int
		wantTrunc    bool
		wantOrigSize int
	}{
		{
			name:      "no truncation",
			output:    "hello",
			maxBytes:  10,
			wantTrunc: false,
		},
		{
			name:         "needs truncation",
			output:       "hello world",
			maxBytes:     5,
			wantTrunc:    true,
			wantOrigSize: 11,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TruncateOutput(tt.output, tt.maxBytes)
			if got.Truncated != tt.wantTrunc {
				t.Errorf("Truncated = %v, want %v", got.Truncated, tt.wantTrunc)
			}
			if tt.wantTrunc && got.OriginalSize != tt.wantOrigSize {
				t.Errorf("OriginalSize = %d, want %d", got.OriginalSize, tt.wantOrigSize)
			}
		})
	}
}

func TestMessageType_String(t *testing.T) {
	tests := []struct {
		msgType MessageType
		want    string
	}{
		{MessageTypeRequest, "Request"},
		{MessageTypeResponse, "Response"},
		{MessageType(0xFF), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.msgType.String(); got != tt.want {
				t.Errorf("MessageType.String() = %q, want %q", got, tt.want)
			}
		})
	}
}
