//go:build !windows

package xdbg_test

import (
	"context"
	"fmt"
	"time"

	"github.com/omeyang/xkit/pkg/debug/xdbg"
)

// Example 演示 xdbg 调试服务的基本用法
func Example() {
	// 创建调试服务（后台模式，不监听信号）
	srv, err := xdbg.New(
		xdbg.WithBackgroundMode(true),
		xdbg.WithAutoShutdown(5*time.Minute),
	)
	if err != nil {
		fmt.Println("创建失败:", err)
		return
	}

	// 启动服务
	ctx := context.Background()
	if err := srv.Start(ctx); err != nil {
		fmt.Println("启动失败:", err)
		return
	}
	//nolint:errcheck // test cleanup: example 函数中的清理操作
	defer func() { _ = srv.Stop() }()

	fmt.Println("服务状态:", srv.State())
	// Output: 服务状态: Started
}

// Example_withBackgroundMode 演示后台模式和手动启用
func Example_withBackgroundMode() {
	// 后台模式下，服务不监听信号，仅通过 Enable/Disable 控制
	srv, err := xdbg.New(
		xdbg.WithBackgroundMode(true),
		xdbg.WithSocketPath("/tmp/xdbg-example.sock"),
	)
	if err != nil {
		fmt.Println("创建失败:", err)
		return
	}

	ctx := context.Background()
	if err := srv.Start(ctx); err != nil {
		fmt.Println("启动失败:", err)
		return
	}
	//nolint:errcheck // test cleanup: example 函数中的清理操作
	defer func() { _ = srv.Stop() }()

	// 初始状态：已启动但未监听
	fmt.Println("初始状态:", srv.State())
	fmt.Println("正在监听:", srv.IsListening())

	// Output:
	// 初始状态: Started
	// 正在监听: false
}

// Example_withCustomOptions 演示自定义配置选项
func Example_withCustomOptions() {
	// 创建带自定义配置的调试服务
	srv, err := xdbg.New(
		xdbg.WithBackgroundMode(true),
		xdbg.WithSocketPath("/tmp/xdbg-custom.sock"),
		xdbg.WithSocketPerm(0o600),
		xdbg.WithMaxSessions(3),
		xdbg.WithMaxConcurrentCommands(10),
		xdbg.WithCommandTimeout(60*time.Second),
		xdbg.WithAutoShutdown(10*time.Minute),
	)
	if err != nil {
		fmt.Println("创建失败:", err)
		return
	}

	ctx := context.Background()
	if err := srv.Start(ctx); err != nil {
		fmt.Println("启动失败:", err)
		return
	}
	//nolint:errcheck // test cleanup: example 函数中的清理操作
	defer func() { _ = srv.Stop() }()

	fmt.Println("服务创建成功")
	// Output: 服务创建成功
}

// Example_withCommandWhitelist 演示命令白名单
func Example_withCommandWhitelist() {
	// 只允许特定命令
	srv, err := xdbg.New(
		xdbg.WithBackgroundMode(true),
		xdbg.WithCommandWhitelist([]string{"setlog", "stack"}),
	)
	if err != nil {
		fmt.Println("创建失败:", err)
		return
	}

	ctx := context.Background()
	if err := srv.Start(ctx); err != nil {
		fmt.Println("启动失败:", err)
		return
	}
	//nolint:errcheck // test cleanup: example 函数中的清理操作
	defer func() { _ = srv.Stop() }()

	// 注意：help 和 exit 命令始终被允许，即使不在白名单中
	fmt.Println("白名单配置成功")
	// Output: 白名单配置成功
}

// ExampleNewErrorResponse 演示错误响应创建
func ExampleNewErrorResponse() {
	err := fmt.Errorf("something went wrong")
	resp := xdbg.NewErrorResponse(err)

	fmt.Println("成功:", resp.Success)
	fmt.Println("错误:", resp.Error)
	// Output:
	// 成功: false
	// 错误: something went wrong
}

// ExampleNewSuccessResponse 演示成功响应创建
func ExampleNewSuccessResponse() {
	resp := xdbg.NewSuccessResponse("operation completed")

	fmt.Println("成功:", resp.Success)
	fmt.Println("输出:", resp.Output)
	// Output:
	// 成功: true
	// 输出: operation completed
}

// ExampleNewCodec 演示消息编解码
func ExampleNewCodec() {
	codec := xdbg.NewCodec()

	// 编码请求
	req := &xdbg.Request{
		Command: "setlog",
		Args:    []string{"debug"},
	}
	data, err := codec.EncodeRequest(req)
	if err != nil {
		fmt.Println("编码失败:", err)
		return
	}

	fmt.Println("请求编码成功，数据长度:", len(data))
	// Output: 请求编码成功，数据长度: 45
}

// ExampleTruncateUTF8 演示安全 UTF-8 截断
func ExampleTruncateUTF8() {
	// 中文字符串，每个中文字符占 3 字节
	s := "你好世界"

	// 截断到 6 字节（刚好 2 个中文字符）
	truncated := xdbg.TruncateUTF8(s, 6)
	fmt.Println(truncated)

	// 截断到 7 字节（不会破坏字符，返回 6 字节）
	truncated = xdbg.TruncateUTF8(s, 7)
	fmt.Println(truncated)
	// Output:
	// 你好
	// 你好
}
