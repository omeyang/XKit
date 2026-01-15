package xdbg

// 协议常量。
const (
	// ProtocolMagic 协议魔数，用于消息识别。
	ProtocolMagic uint16 = 0xDB09

	// ProtocolVersion 协议版本。
	ProtocolVersion uint8 = 0x01

	// HeaderSize 消息头大小（字节）。
	// Magic(2) + Version(1) + Type(1) + Length(4) = 8 bytes
	HeaderSize = 8

	// MaxPayloadSize 最大 Payload 大小（1MB）。
	MaxPayloadSize = 1024 * 1024

	// JSONOverhead JSON 结构开销预留空间（字节）。
	// 包括 Response 结构的 JSON 字段名、引号、括号等开销。
	JSONOverhead = 200

	// DefaultMaxOutputSize 默认最大输出大小。
	// 必须小于 MaxPayloadSize 以留出 JSON 结构开销的空间。
	DefaultMaxOutputSize = MaxPayloadSize - JSONOverhead
)

// MessageType 消息类型。
type MessageType uint8

const (
	// MessageTypeRequest 请求消息（客户端 -> 服务端）。
	MessageTypeRequest MessageType = 0x01

	// MessageTypeResponse 响应消息（服务端 -> 客户端）。
	MessageTypeResponse MessageType = 0x02
)

// String 返回消息类型的字符串表示。
func (t MessageType) String() string {
	switch t {
	case MessageTypeRequest:
		return "Request"
	case MessageTypeResponse:
		return "Response"
	default:
		return "Unknown"
	}
}

// Request 请求消息。
type Request struct {
	// Command 命令名称。
	Command string `json:"command"`

	// Args 命令参数。
	Args []string `json:"args,omitempty"`
}

// Response 响应消息。
type Response struct {
	// Success 是否成功。
	Success bool `json:"success"`

	// Output 命令输出。
	Output string `json:"output,omitempty"`

	// Error 错误信息。
	Error string `json:"error,omitempty"`

	// Truncated 输出是否被截断。
	Truncated bool `json:"truncated,omitempty"`

	// OriginalSize 原始输出大小（仅当截断时有值）。
	OriginalSize int `json:"original_size,omitempty"`
}

// NewSuccessResponse 创建成功响应。
func NewSuccessResponse(output string) *Response {
	return &Response{
		Success: true,
		Output:  output,
	}
}

// NewErrorResponse 创建错误响应。
func NewErrorResponse(err error) *Response {
	return &Response{
		Success: false,
		Error:   err.Error(),
	}
}

// NewTruncatedResponse 创建截断响应。
func NewTruncatedResponse(output string, originalSize int) *Response {
	return &Response{
		Success:      true,
		Output:       output,
		Truncated:    true,
		OriginalSize: originalSize,
	}
}
