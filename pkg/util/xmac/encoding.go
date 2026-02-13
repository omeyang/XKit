package xmac

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

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
	s := a.String()
	// len(`"`) + 17 + len(`"`) = 19
	buf := make([]byte, 0, len(s)+2)
	buf = append(buf, '"')
	buf = append(buf, s...)
	buf = append(buf, '"')
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
		return fmt.Errorf("%w: unsupported type %T", ErrInvalidFormat, src)
	}
}
