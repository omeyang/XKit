package xnet

import "errors"

var (
	// ErrInvalidAddress 表示无效的 IP 地址字符串。
	ErrInvalidAddress = errors.New("xnet: invalid IP address")

	// ErrInvalidRange 表示无效的 IP 范围格式。
	ErrInvalidRange = errors.New("xnet: invalid IP range")

	// ErrInvalidVersion 表示无效的 IP 版本。
	ErrInvalidVersion = errors.New("xnet: invalid IP version")

	// ErrInvalidBigInt 表示 big.Int 值超出 IP 地址范围。
	ErrInvalidBigInt = errors.New("xnet: big.Int value out of range for IP address")
)
