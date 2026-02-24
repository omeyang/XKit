package xutil

import "testing"

// 设计决策: 使用包级 sink 变量阻止编译器将内联纯函数的返回值丢弃后优化掉整个调用。
// b.Loop() 有部分防优化机制，但对纯值传递的内联函数仍不够可靠。
var (
	sinkInt    int
	sinkString string
	sinkLarge  benchLarge
)

// benchLarge 是用于基准测试的大型结构体，验证值拷贝开销。
type benchLarge struct {
	ID   int
	Name string
	Data [64]byte
}

func BenchmarkIf_True(b *testing.B) {
	var r int
	for b.Loop() {
		r = If(true, 1, 2)
	}
	sinkInt = r
}

func BenchmarkIf_False(b *testing.B) {
	var r int
	for b.Loop() {
		r = If(false, 1, 2)
	}
	sinkInt = r
}

func BenchmarkIfString_True(b *testing.B) {
	var r string
	for b.Loop() {
		r = If(true, "hello", "world")
	}
	sinkString = r
}

func BenchmarkIfString_False(b *testing.B) {
	var r string
	for b.Loop() {
		r = If(false, "hello", "world")
	}
	sinkString = r
}

func BenchmarkIfStruct(b *testing.B) {
	x := benchLarge{ID: 1, Name: "a"}
	y := benchLarge{ID: 2, Name: "b"}
	var r benchLarge
	for b.Loop() {
		r = If(true, x, y)
	}
	sinkLarge = r
}
