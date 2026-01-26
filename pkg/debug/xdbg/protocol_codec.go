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

// TruncateOutput 截断输出并返回截断后的响应。
func TruncateOutput(output string, maxBytes int) *Response {
	if len(output) <= maxBytes {
		return NewSuccessResponse(output)
	}

	truncated := TruncateUTF8(output, maxBytes)
	return NewTruncatedResponse(truncated, len(output))
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
