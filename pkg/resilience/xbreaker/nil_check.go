package xbreaker

import "reflect"

// isNilInterfaceValue 检测 typed-nil 接口值。
// var p *MyPolicy = nil 赋给 TripPolicy 接口后，p != nil 为 true，
// 但底层指针为 nil，调用方法会 panic。
func isNilInterfaceValue(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	k := rv.Kind()
	if k == reflect.Ptr || k == reflect.Interface {
		return rv.IsNil()
	}
	return false
}
