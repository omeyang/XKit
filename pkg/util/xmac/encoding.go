package xmac

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

// MarshalBinary 实现 [encoding.BinaryMarshaler]。
// 返回 6 字节的原始 MAC 地址数据。
// 无效地址返回 6 字节全零。
func (a Addr) MarshalBinary() ([]byte, error) {
	b := a.bytes
	return b[:], nil
}

// UnmarshalBinary 实现 [encoding.BinaryUnmarshaler]。
// 输入必须为 6 字节。
// 对 nil 接收者返回 [ErrNilReceiver]。
func (a *Addr) UnmarshalBinary(data []byte) error {
	if a == nil {
		return ErrNilReceiver
	}
	if len(data) != 6 {
		return fmt.Errorf("%w: expected 6 bytes, got %d", ErrInvalidLength, len(data))
	}
	copy(a.bytes[:], data)
	return nil
}

// MarshalText 实现 [encoding.TextMarshaler]。
// 输出小写冒号格式（aa:bb:cc:dd:ee:ff）。
// 无效地址输出空字节切片。
func (a Addr) MarshalText() ([]byte, error) {
	if !a.IsValid() {
		return []byte{}, nil
	}
	// 直接构造 []byte，避免 String() 的 string→[]byte 二次分配。
	return marshalColonBytes(a.bytes), nil
}

// UnmarshalText 实现 [encoding.TextUnmarshaler]。
// 支持所有 [Parse] 支持的格式。
// 空输入设置为零值。
// 对 nil 接收者返回 [ErrNilReceiver]。
func (a *Addr) UnmarshalText(text []byte) error {
	if a == nil {
		return ErrNilReceiver
	}
	if len(text) == 0 {
		*a = Addr{}
		return nil
	}
	parsed, err := Parse(string(text))
	if err != nil {
		return err
	}
	*a = parsed
	return nil
}

// MarshalJSON 实现 [json.Marshaler]。
// 输出带引号的小写冒号格式字符串（"aa:bb:cc:dd:ee:ff"）。
// 无效地址输出空字符串（""）。
//
// MAC 地址字符串仅包含 [0-9a-f:] 字符，无需 JSON 转义，
// 因此直接构造带引号的字节切片，避免 [json.Marshal] 的反射开销。
func (a Addr) MarshalJSON() ([]byte, error) {
	if !a.IsValid() {
		return []byte(`""`), nil
	}
	// 设计决策: 手动内联 hex 编码逻辑（与 marshalColonBytes 重复），而非调用
	// marshalColonBytes 后拼接引号。内联方式直接构造 19 字节（`"` + 17 字节 MAC + `"`），
	// 仅 1 次堆分配；若调用 marshalColonBytes + 拼接引号则需 2 次堆分配。
	b := a.bytes
	buf := make([]byte, 19)
	buf[0] = '"'
	buf[1] = hexLower[b[0]>>4]
	buf[2] = hexLower[b[0]&0x0f]
	buf[3] = ':'
	buf[4] = hexLower[b[1]>>4]
	buf[5] = hexLower[b[1]&0x0f]
	buf[6] = ':'
	buf[7] = hexLower[b[2]>>4]
	buf[8] = hexLower[b[2]&0x0f]
	buf[9] = ':'
	buf[10] = hexLower[b[3]>>4]
	buf[11] = hexLower[b[3]&0x0f]
	buf[12] = ':'
	buf[13] = hexLower[b[4]>>4]
	buf[14] = hexLower[b[4]&0x0f]
	buf[15] = ':'
	buf[16] = hexLower[b[5]>>4]
	buf[17] = hexLower[b[5]&0x0f]
	buf[18] = '"'
	return buf, nil
}

// UnmarshalJSON 实现 [json.Unmarshaler]。
// 支持所有 [Parse] 支持的格式。
// 空字符串或 null 设置为零值。
// 对 nil 接收者返回 [ErrNilReceiver]。
//
// 此方法应通过 [json.Unmarshal] 间接调用，不建议直接调用。
// 直接调用时 null 匹配为精确字节比较（不去除空白），
// 这与 Go 标准库 [time.Time.UnmarshalJSON] 的行为一致。
func (a *Addr) UnmarshalJSON(data []byte) error {
	if a == nil {
		return ErrNilReceiver
	}
	// 处理 null
	if string(data) == "null" {
		*a = Addr{}
		return nil
	}

	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidFormat, err)
	}
	if s == "" {
		*a = Addr{}
		return nil
	}
	parsed, err := Parse(s)
	if err != nil {
		return err
	}
	*a = parsed
	return nil
}

// Value 实现 [database/sql/driver.Valuer]。
// 用于 SQL 数据库写入。
// 输出小写冒号格式字符串，无效地址返回 nil（SQL NULL）。
func (a Addr) Value() (driver.Value, error) {
	if !a.IsValid() {
		return nil, nil
	}
	return a.String(), nil
}

// Scan 实现 [database/sql.Scanner]。
// 用于 SQL 数据库读取。
// 支持 string、[]byte（字符串或 6 字节二进制）、nil 输入。
// 对 nil 接收者返回 [ErrNilReceiver]。
//
// 对于 []byte 输入，恰好 6 字节时始终视为二进制 MAC（适用于 BINARY(6) 列），
// 其他长度视为文本格式。如需强制文本解析 6 字节内容，应先转换为 string 再传入。
func (a *Addr) Scan(src any) error {
	if a == nil {
		return ErrNilReceiver
	}
	switch v := src.(type) {
	case nil:
		*a = Addr{}
		return nil
	case string:
		if v == "" {
			*a = Addr{}
			return nil
		}
		parsed, err := Parse(v)
		if err != nil {
			return err
		}
		*a = parsed
		return nil
	case []byte:
		if len(v) == 0 {
			*a = Addr{}
			return nil
		}
		// 6 字节视为二进制格式，适用于 BINARY(6) 列存储的原始 MAC 字节。
		// 文本格式 MAC 最短 12 字符（如 "aabbccddeeff"），不会与 6 字节二进制冲突。
		// 设计决策: 全零字节 {0,0,0,0,0,0} 直接 copy 后等于零值 Addr{}，
		// IsValid() 返回 false，与 Parse("00:00:00:00:00:00") 行为一致。
		if len(v) == 6 {
			copy(a.bytes[:], v)
			return nil
		}
		// 其他长度视为字符串格式
		parsed, err := Parse(string(v))
		if err != nil {
			return err
		}
		*a = parsed
		return nil
	default:
		return fmt.Errorf("%w: %T", ErrUnsupportedType, src)
	}
}
