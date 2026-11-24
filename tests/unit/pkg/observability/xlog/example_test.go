package xlog_test

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/omeyang/xkit/pkg/context/xctx"
	"github.com/omeyang/xkit/pkg/observability/xlog"
)

func Example() {
	var buf bytes.Buffer
	logger, cleanup, _ := xlog.New().
		SetOutput(&buf).
		SetLevel(xlog.LevelInfo).
		SetFormat("text").
		SetEnrich(false). // 禁用 enrich 以获得可预测输出
		Build()
	defer cleanup()

	// 记录日志
	ctx := context.Background()
	logger.Info(ctx, "hello xlog")

	output := buf.String()
	fmt.Println("has level:", strings.Contains(output, "level=INFO"))
	fmt.Println("has msg:", strings.Contains(output, "hello xlog"))
	// Output:
	// has level: true
	// has msg: true
}

func Example_withAttrs() {
	var buf bytes.Buffer
	logger, cleanup, _ := xlog.New().
		SetOutput(&buf).
		SetFormat("text").
		SetEnrich(false).
		Build()
	defer cleanup()

	// 使用属性
	logger.Info(context.Background(), "user action",
		slog.String("user_id", "u123"),
		slog.String("action", "login"),
	)

	output := buf.String()
	fmt.Println("contains user_id:", strings.Contains(output, "user_id"))
	fmt.Println("contains action:", strings.Contains(output, "action"))
	// Output:
	// contains user_id: true
	// contains action: true
}

func Example_dynamicLevel() {
	var buf bytes.Buffer
	logger, cleanup, _ := xlog.New().
		SetOutput(&buf).
		SetLevel(xlog.LevelError). // 初始只记录 Error
		SetEnrich(false).
		Build()
	defer cleanup()

	ctx := context.Background()

	// Info 不会输出
	logger.Info(ctx, "should not appear")
	fmt.Println("before SetLevel, has output:", buf.Len() > 0)

	// 动态调整到 Info
	logger.SetLevel(xlog.LevelInfo)
	logger.Info(ctx, "now visible")
	fmt.Println("after SetLevel, has output:", buf.Len() > 0)
	// Output:
	// before SetLevel, has output: false
	// after SetLevel, has output: true
}

func Example_withContext() {
	var buf bytes.Buffer
	logger, cleanup, _ := xlog.New().
		SetOutput(&buf).
		SetFormat("json").
		Build() // 默认启用 enrich
	defer cleanup()

	// 设置 context 追踪和身份信息
	ctx := context.Background()
	ctx, _ = xctx.WithTraceID(ctx, "trace-example-123")
	ctx, _ = xctx.WithTenantID(ctx, "tenant-abc")

	logger.Info(ctx, "request handled")

	output := buf.String()
	fmt.Println("has trace_id:", strings.Contains(output, "trace-example-123"))
	fmt.Println("has tenant_id:", strings.Contains(output, "tenant-abc"))
	// Output:
	// has trace_id: true
	// has tenant_id: true
}

func Example_deploymentType() {
	var buf bytes.Buffer
	logger, cleanup, _ := xlog.New().
		SetOutput(&buf).
		SetFormat("json").
		SetDeploymentType(xctx.DeploymentSaaS).
		SetEnrich(false).
		Build()
	defer cleanup()

	logger.Info(context.Background(), "saas log")

	output := buf.String()
	fmt.Println("has deployment_type:", strings.Contains(output, "deployment_type"))
	fmt.Println("is SAAS:", strings.Contains(output, "SAAS"))
	// Output:
	// has deployment_type: true
	// is SAAS: true
}

func Example_lazy() {
	var buf bytes.Buffer
	logger, cleanup, _ := xlog.New().
		SetOutput(&buf).
		SetLevel(xlog.LevelError). // 禁用 Debug
		SetEnrich(false).
		Build()
	defer cleanup()

	computed := false
	expensiveFunc := func() any {
		computed = true
		return "expensive result"
	}

	// Debug 被禁用，Lazy 函数不会被调用
	logger.Debug(context.Background(), "debug message",
		xlog.Lazy("data", expensiveFunc),
	)

	fmt.Println("expensive func called:", computed)
	// Output:
	// expensive func called: false
}

func Example_globalLogger() {
	// 重置全局状态
	xlog.ResetDefault()
	defer xlog.ResetDefault()

	var buf bytes.Buffer
	customLogger, cleanup, _ := xlog.New().
		SetOutput(&buf).
		SetEnrich(false).
		Build()
	defer cleanup()

	// 设置自定义全局 Logger
	xlog.SetDefault(customLogger)

	// 使用全局便利函数
	xlog.Info(context.Background(), "global log message")

	fmt.Println("has message:", strings.Contains(buf.String(), "global log message"))
	// Output:
	// has message: true
}

func Example_childLogger() {
	var buf bytes.Buffer
	logger, cleanup, _ := xlog.New().
		SetOutput(&buf).
		SetFormat("json").
		SetEnrich(false).
		Build()
	defer cleanup()

	// 创建带固定属性的子 Logger
	childLogger := logger.With(slog.String("service", "user-api"))
	childLogger.Info(context.Background(), "child log")

	output := buf.String()
	fmt.Println("has service:", strings.Contains(output, "user-api"))
	// Output:
	// has service: true
}

func Example_withGroup() {
	var buf bytes.Buffer
	logger, cleanup, _ := xlog.New().
		SetOutput(&buf).
		SetFormat("json").
		SetEnrich(false).
		Build()
	defer cleanup()

	// 创建分组 Logger
	reqLogger := logger.WithGroup("request")
	reqLogger.Info(context.Background(), "grouped log",
		slog.String("method", "GET"),
		slog.String("path", "/api/users"),
	)

	output := buf.String()
	fmt.Println("has request group:", strings.Contains(output, "request"))
	// Output:
	// has request group: true
}
