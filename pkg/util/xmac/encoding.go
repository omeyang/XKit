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
	return []byte(a.String()), nil
}

// UnmarshalText 实现 [encoding.TextUnmarshaler]。
// 支持所有 [Parse] 支持的格式。
// 空输入设置为零值。
func (a *Addr) UnmarshalText(text []byte) error {
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
func (a Addr) MarshalJSON() ([]byte, error) {
	if !a.IsValid() {
		return []byte(`""`), nil
	}
	return json.Marshal(a.String())
}

// UnmarshalJSON 实现 [json.Unmarshaler]。
// 支持所有 [Parse] 支持的格式。
// 空字符串或 null 设置为零值。
func (a *Addr) UnmarshalJSON(data []byte) error {
	// 处理 null
	if string(data) == "null" {
		*a = Addr{}
		return nil
	}

	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidFormat, err)
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
func (a *Addr) Scan(src any) error {
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
		// 6 字节视为二进制格式
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
