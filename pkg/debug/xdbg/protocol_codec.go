package xdbg

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"unicode/utf8"
)

// Codec 消息编解码器。
type Codec struct {
	maxPayloadSize int
}

// NewCodec 创建编解码器。
func NewCodec() *Codec {
	return &Codec{
		maxPayloadSize: MaxPayloadSize,
	}
}

// EncodeRequest 编码请求消息。
func (c *Codec) EncodeRequest(req *Request) ([]byte, error) {
	return c.encode(MessageTypeRequest, req)
}

// EncodeResponse 编码响应消息。
func (c *Codec) EncodeResponse(resp *Response) ([]byte, error) {
	return c.encode(MessageTypeResponse, resp)
}

// encode 编码消息。
func (c *Codec) encode(msgType MessageType, payload any) ([]byte, error) {
	// 编码 payload
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	payloadLen := len(payloadBytes)
	if payloadLen > c.maxPayloadSize {
		return nil, ErrMessageTooLarge
	}

	// 安全转换为 uint32（用于协议头）
	payloadLenU32, err := safeIntToUint32(payloadLen)
	if err != nil {
		return nil, err
	}

	// 构造完整消息
	msg := make([]byte, HeaderSize+payloadLen)

	// 写入头部
	binary.BigEndian.PutUint16(msg[0:2], ProtocolMagic)
	msg[2] = ProtocolVersion
	msg[3] = byte(msgType)
	binary.BigEndian.PutUint32(msg[4:8], payloadLenU32)

	// 写入 payload
	copy(msg[HeaderSize:], payloadBytes)

	return msg, nil
}

// DecodeHeader 从 reader 读取并解析消息头。
// 返回消息类型和 payload 长度。
func (c *Codec) DecodeHeader(r io.Reader) (MessageType, uint32, error) {
	header := make([]byte, HeaderSize)
	if _, err := io.ReadFull(r, header); err != nil {
		if err == io.EOF {
			return 0, 0, ErrConnectionClosed
		}
		return 0, 0, fmt.Errorf("read header: %w", err)
	}

	// 解析头部字段
	// io.ReadFull 保证读取了 HeaderSize (8) 字节，所以访问 header[0:8] 是安全的
	return c.parseHeader(header)
}

// parseHeader 解析消息头。
// 要求 header 长度至少为 HeaderSize (8) 字节。
func (c *Codec) parseHeader(header []byte) (MessageType, uint32, error) {
	// 显式检查 header 长度以满足静态分析
	if len(header) < HeaderSize {
		return 0, 0, ErrInvalidMessage
	}

	// 验证 magic
	magic := binary.BigEndian.Uint16(header[0:2])
	if magic != ProtocolMagic {
		return 0, 0, ErrInvalidMessage
	}

	// 验证版本
	version := header[2]
	if version != ProtocolVersion {
		return 0, 0, fmt.Errorf("%w: unsupported version %d", ErrInvalidMessage, version)
	}

	msgType := MessageType(header[3])
	length := binary.BigEndian.Uint32(header[4:8])

	// 边界检查
	maxPayloadU32, err := safeIntToUint32(c.maxPayloadSize)
	if err != nil {
		return 0, 0, ErrMessageTooLarge
	}
	if length > maxPayloadU32 {
		return 0, 0, ErrMessageTooLarge
	}

	return msgType, length, nil
}

// decodePayload 读取并解析 payload。
func (c *Codec) decodePayload(r io.Reader, length uint32, expectedType MessageType, msgType MessageType, target any) error {
	if msgType != expectedType {
		return fmt.Errorf("%w: expected %s, got %s", ErrInvalidMessage, expectedType, msgType)
	}

	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		return fmt.Errorf("read payload: %w", err)
	}

	if err := json.Unmarshal(payload, target); err != nil {
		return fmt.Errorf("unmarshal: %w", err)
	}

	return nil
}

// DecodeRequest 从 reader 读取并解析请求消息。
func (c *Codec) DecodeRequest(r io.Reader) (*Request, error) {
	msgType, length, err := c.DecodeHeader(r)
	if err != nil {
		return nil, err
	}

	var req Request
	if err := c.decodePayload(r, length, MessageTypeRequest, msgType, &req); err != nil {
		return nil, err
	}

	return &req, nil
}

// DecodeResponse 从 reader 读取并解析响应消息。
func (c *Codec) DecodeResponse(r io.Reader) (*Response, error) {
	msgType, length, err := c.DecodeHeader(r)
	if err != nil {
		return nil, err
	}

	var resp Response
	if err := c.decodePayload(r, length, MessageTypeResponse, msgType, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

// TruncateUTF8 安全截断 UTF-8 字符串，不破坏多字节字符。
func TruncateUTF8(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}

	// 从 maxBytes 位置向前找到有效的 UTF-8 边界
	for maxBytes > 0 && !utf8.RuneStart(s[maxBytes]) {
		maxBytes--
	}

	return s[:maxBytes]
}

// TruncateOutput 截断输出，使 JSON 编码后的字符串内容不超过 maxBytes。
// 这确保了即使输出包含大量需要转义的字符（如 \n → \\n），
// 编码后的 Response payload 也不会超过 MaxPayloadSize。
func TruncateOutput(output string, maxBytes int) *Response {
	if jsonStringLen(output) <= maxBytes {
		return NewSuccessResponse(output)
	}

	truncated := truncateJSONSafe(output, maxBytes)
	return NewTruncatedResponse(truncated, len(output))
}

// jsonCharCost 返回单字节字符 c 经 JSON 编码后的字节数。
// 匹配 encoding/json.Marshal 的转义规则（含 HTML 安全转义 <, >, &）。
func jsonCharCost(c byte) int {
	switch c {
	case '"', '\\':
		return 2
	case '\b', '\f', '\n', '\r', '\t':
		return 2
	case '<', '>', '&':
		return 6
	default:
		if c < 0x20 {
			return 6
		}
		return 1
	}
}

// jsonStringLen 返回 s 经 JSON 字符串编码后的内容长度（不含两端引号）。
func jsonStringLen(s string) int {
	n := 0
	for i := 0; i < len(s); i++ {
		n += jsonCharCost(s[i])
	}
	return n
}

// truncateJSONSafe 截断 s 使其 JSON 编码后的字符串内容不超过 maxJSONBytes。
// 不会切断多字节 UTF-8 字符。
func truncateJSONSafe(s string, maxJSONBytes int) string {
	if maxJSONBytes <= 0 {
		return ""
	}

	budget := maxJSONBytes
	i := 0
	for i < len(s) && budget > 0 {
		c := s[i]

		if c >= 0x80 {
			_, size := utf8.DecodeRuneInString(s[i:])
			if size > budget {
				break
			}
			budget -= size
			i += size
			continue
		}

		cost := jsonCharCost(c)
		if cost > budget {
			break
		}
		budget -= cost
		i++
	}

	return s[:i]
}

// safeIntToUint32 安全地将 int 转换为 uint32。
// 如果值为负数或超出 uint32 范围，返回 ErrMessageTooLarge。
func safeIntToUint32(n int) (uint32, error) {
	if n < 0 {
		return 0, ErrMessageTooLarge
	}
	if n > math.MaxUint32 {
		return 0, ErrMessageTooLarge
	}
	return uint32(n), nil
}
