package xplatform_test

import (
	"fmt"

	"github.com/omeyang/xkit/pkg/context/xplatform"
)

// Example_quickStart 演示 xplatform 包的典型使用场景。
//
// 在服务启动时从 AUTH 服务或配置初始化平台信息，然后在业务代码中使用。
func Example_quickStart() {
	// 清理之前的状态（测试用）
	xplatform.Reset()
	defer xplatform.Reset()

	// 服务启动时初始化（信息来自 AUTH 服务或配置）
	cfg := xplatform.Config{
		PlatformID:      "platform-001",
		HasParent:       true, // SaaS 多级部署场景
		UnclassRegionID: "region-default",
	}
	if err := xplatform.Init(cfg); err != nil {
		fmt.Println("初始化失败:", err)
		return
	}

	// 业务代码中使用
	fmt.Printf("PlatformID: %s\n", xplatform.PlatformID())
	fmt.Printf("HasParent: %v\n", xplatform.HasParent())
	fmt.Printf("UnclassRegionID: %s\n", xplatform.UnclassRegionID())

	// Output:
	// PlatformID: platform-001
	// HasParent: true
	// UnclassRegionID: region-default
}

// Example_mustInit 演示 MustInit 的使用场景。
//
// MustInit 适用于初始化失败应该终止程序的场景。
func Example_mustInit() {
	// 清理之前的状态
	xplatform.Reset()
	defer xplatform.Reset()

	// 使用 MustInit（失败时 panic）
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("初始化失败:", r)
		}
	}()

	// 缺少必填字段 PlatformID 会 panic
	xplatform.MustInit(xplatform.Config{})

	// Output:
	// 初始化失败: xplatform: missing platform_id
}

// ExampleConfig_Validate 演示配置验证。
func ExampleConfig_Validate() {
	// PlatformID 是必填字段
	cfg := xplatform.Config{}
	err := cfg.Validate()
	fmt.Printf("空配置: %v\n", err == xplatform.ErrMissingPlatformID)

	// 有效配置
	cfg.PlatformID = "platform-001"
	err = cfg.Validate()
	fmt.Printf("有效配置: %v\n", err == nil)

	// Output:
	// 空配置: true
	// 有效配置: true
}

// Example_checkInitialized 演示初始化状态检查。
func Example_checkInitialized() {
	// 清理之前的状态
	xplatform.Reset()
	defer xplatform.Reset()

	fmt.Printf("初始化前: %v\n", xplatform.IsInitialized())

	if err := xplatform.Init(xplatform.Config{PlatformID: "platform-001"}); err != nil {
		fmt.Println("初始化失败:", err)
		return
	}
	fmt.Printf("初始化后: %v\n", xplatform.IsInitialized())

	// Output:
	// 初始化前: false
	// 初始化后: true
}
