package xenv_test

import (
	"errors"
	"fmt"
	"os"

	"github.com/omeyang/xkit/pkg/context/xenv"
)

// Example_quickStart 演示 xenv 包的典型使用场景。
//
// 在服务启动时从环境变量初始化部署类型，然后在业务代码中使用。
func Example_quickStart() {
	// 清理之前的状态（测试用）
	xenv.Reset()

	// 设置环境变量（实际场景由 K8s ConfigMap 注入）
	if err := os.Setenv(xenv.EnvDeployType, "SAAS"); err != nil {
		fmt.Println("设置环境变量失败:", err)
		return
	}
	defer func() {
		if err := os.Unsetenv(xenv.EnvDeployType); err != nil {
			fmt.Println("清理环境变量失败:", err)
		}
	}()

	// 服务启动时初始化
	if err := xenv.Init(); err != nil {
		fmt.Println("初始化失败:", err)
		return
	}

	// 业务代码中使用
	fmt.Printf("部署类型: %s\n", xenv.Type())
	fmt.Printf("IsSaaS: %v\n", xenv.IsSaaS())
	fmt.Printf("IsLocal: %v\n", xenv.IsLocal())

	// Output:
	// 部署类型: SAAS
	// IsSaaS: true
	// IsLocal: false
}

// Example_initWith 演示使用 InitWith 进行测试场景的初始化。
//
// InitWith 允许直接指定部署类型，无需依赖环境变量。
func Example_initWith() {
	// 清理之前的状态
	xenv.Reset()

	// 测试场景：直接指定部署类型
	if err := xenv.InitWith(xenv.DeployLocal); err != nil {
		fmt.Println("初始化失败:", err)
		return
	}

	fmt.Printf("部署类型: %s\n", xenv.Type())
	fmt.Printf("IsLocal: %v\n", xenv.IsLocal())
	fmt.Printf("IsInitialized: %v\n", xenv.IsInitialized())

	// Output:
	// 部署类型: LOCAL
	// IsLocal: true
	// IsInitialized: true
}

// ExampleParse_caseInsensitive 演示 Parse 函数的大小写不敏感特性。
func ExampleParse_caseInsensitive() {
	// Parse 支持大小写不敏感匹配
	inputs := []string{"local", "LOCAL", "Local", "saas", "SAAS", "SaaS"}

	for _, input := range inputs {
		dt, err := xenv.Parse(input)
		if err != nil {
			fmt.Printf("%q -> 错误\n", input)
			continue
		}
		fmt.Printf("%q -> %s\n", input, dt)
	}

	// Output:
	// "local" -> LOCAL
	// "LOCAL" -> LOCAL
	// "Local" -> LOCAL
	// "saas" -> SAAS
	// "SAAS" -> SAAS
	// "SaaS" -> SAAS
}

// ExampleRequireType 演示 RequireType 函数的使用。
//
// RequireType 在未初始化时返回错误，适用于必须明确知道部署类型的场景。
func ExampleRequireType() {
	xenv.Reset()
	if err := xenv.InitWith(xenv.DeployLocal); err != nil {
		fmt.Println("初始化失败:", err)
		return
	}
	defer xenv.Reset()

	dt, err := xenv.RequireType()
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	fmt.Println("DeployType:", dt)
	// Output:
	// DeployType: LOCAL
}

// ExampleParse_invalidInput 演示 Parse 函数的错误处理。
func ExampleParse_invalidInput() {
	// 无效输入返回包含输入值和合法值提示的 ErrInvalidType
	_, err := xenv.Parse("invalid")
	fmt.Printf("无效输入: %v\n", errors.Is(err, xenv.ErrInvalidType))
	fmt.Printf("错误信息包含输入值: %v\n", err)

	// 空输入返回 ErrInvalidType
	_, err = xenv.Parse("")
	fmt.Printf("空输入: %v\n", errors.Is(err, xenv.ErrInvalidType))

	// Output:
	// 无效输入: true
	// 错误信息包含输入值: xenv: invalid deployment type: "invalid" (expected LOCAL or SAAS)
	// 空输入: true
}
