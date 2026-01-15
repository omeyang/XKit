package xmac

import "errors"

// 预定义错误变量，支持 errors.Is 判断。
var (
	// ErrEmpty 表示输入为空字符串。
	ErrEmpty = errors.New("xmac: empty input")

	// ErrInvalidFormat 表示 MAC 地址格式无效。
	ErrInvalidFormat = errors.New("xmac: invalid format")

	// ErrInvalidLength 表示 MAC 地址长度不正确（期望 6 字节）。
	ErrInvalidLength = errors.New("xmac: invalid length")

	// ErrOverflow 表示地址运算溢出（超过 ff:ff:ff:ff:ff:ff）。
	ErrOverflow = errors.New("xmac: address overflow")

	// ErrUnderflow 表示地址运算下溢（低于 00:00:00:00:00:00）。
	ErrUnderflow = errors.New("xmac: address underflow")
)
