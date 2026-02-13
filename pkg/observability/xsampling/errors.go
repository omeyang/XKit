package xsampling

import "errors"

// 采样器创建相关的错误
var (
	// ErrInvalidRate 表示采样比率不在 [0.0, 1.0] 范围内
	ErrInvalidRate = errors.New("xsampling: rate must be in [0.0, 1.0]")

	// ErrNilKeyFunc 表示 KeyBasedSampler 的 keyFunc 为 nil
	ErrNilKeyFunc = errors.New("xsampling: keyFunc must not be nil")

	// ErrInvalidCount 表示 CountSampler 的采样间隔 n 不合法（必须 >= 1）
	ErrInvalidCount = errors.New("xsampling: count n must be >= 1")
)
