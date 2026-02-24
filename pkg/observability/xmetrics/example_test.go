package xmetrics_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/omeyang/xkit/pkg/observability/xmetrics"
)

func ExampleNewOTelObserver() {
	obs, err := xmetrics.NewOTelObserver()
	if err != nil {
		panic(err)
	}

	// 推荐使用闭包 defer 捕获业务错误，确保 span 正确记录错误状态。
	// 若使用 defer span.End(xmetrics.Result{})，则始终记录 StatusOK。
	var bizErr error
	ctx, span := xmetrics.Start(context.Background(), obs, xmetrics.SpanOptions{
		Component: "myservice",
		Operation: "do_work",
		Kind:      xmetrics.KindClient,
		Attrs:     []xmetrics.Attr{xmetrics.String("db.system", "redis")},
	})
	defer func() { span.End(xmetrics.Result{Err: bizErr}) }()

	_ = ctx
	_ = bizErr
	fmt.Println("span created")
	// Output: span created
}

func ExampleStart_nilObserver() {
	// nil observer 安全返回 NoopSpan，零开销
	ctx, span := xmetrics.Start(context.Background(), nil, xmetrics.SpanOptions{
		Component: "test",
		Operation: "skip",
	})
	span.End(xmetrics.Result{})

	_ = ctx
	fmt.Println("noop span ended")
	// Output: noop span ended
}

func ExampleNoopObserver() {
	obs := xmetrics.NoopObserver{}
	ctx, span := obs.Start(context.Background(), xmetrics.SpanOptions{
		Component: "test",
		Operation: "noop",
	})
	span.End(xmetrics.Result{Status: xmetrics.StatusOK})

	_ = ctx
	fmt.Println("noop observer")
	// Output: noop observer
}

func ExampleResult_withError() {
	obs := xmetrics.NoopObserver{}
	_, span := obs.Start(context.Background(), xmetrics.SpanOptions{
		Component: "myservice",
		Operation: "fetch_data",
	})

	err := errors.New("connection refused")
	// Err 非 nil 时自动推导 StatusError
	span.End(xmetrics.Result{Err: err})

	fmt.Println("error recorded")
	// Output: error recorded
}

func ExampleString() {
	attr := xmetrics.String("service", "api-gateway")
	fmt.Println(attr.Key)
	// Output: service
}

func ExampleKind_String() {
	fmt.Println(xmetrics.KindClient)
	fmt.Println(xmetrics.KindServer)
	// Output:
	// Client
	// Server
}
