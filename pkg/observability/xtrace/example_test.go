package xtrace_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/omeyang/xkit/pkg/context/xctx"
	"github.com/omeyang/xkit/pkg/observability/xtrace"

	"google.golang.org/grpc/metadata"
)

func ExampleExtractFromHTTPHeader() {
	h := http.Header{}
	h.Set("traceparent", "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01")

	info := xtrace.ExtractFromHTTPHeader(h)
	fmt.Println("TraceID:", info.TraceID)
	fmt.Println("SpanID:", info.SpanID)
	fmt.Println("TraceFlags:", info.TraceFlags)
	// Output:
	// TraceID: 0af7651916cd43dd8448eb211c80319c
	// SpanID: b7ad6b7169203331
	// TraceFlags: 01
}

func ExampleExtractFromHTTPRequest() {
	req := httptest.NewRequest("GET", "/api/users", nil)
	req.Header.Set("X-Trace-ID", "0af7651916cd43dd8448eb211c80319c")
	req.Header.Set("X-Request-ID", "req-abc-123")

	info := xtrace.ExtractFromHTTPRequest(req)
	fmt.Println("TraceID:", info.TraceID)
	fmt.Println("RequestID:", info.RequestID)
	// Output:
	// TraceID: 0af7651916cd43dd8448eb211c80319c
	// RequestID: req-abc-123
}

func ExampleHTTPMiddleware() {
	handler := xtrace.HTTPMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceID := xtrace.TraceID(r.Context())
		fmt.Println("has TraceID:", traceID != "")
	}))

	req := httptest.NewRequest("GET", "/api/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	// Output:
	// has TraceID: true
}

func ExampleHTTPMiddleware_disableAutoGenerate() {
	handler := xtrace.HTTPMiddleware(
		xtrace.WithAutoGenerate(false),
	)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceID := xtrace.TraceID(r.Context())
		fmt.Println("TraceID empty:", traceID == "")
	}))

	req := httptest.NewRequest("GET", "/api/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	// Output:
	// TraceID empty: true
}

func ExampleInjectToRequest() {
	ctx := context.Background()
	ctx, _ = xctx.WithTraceID(ctx, "0af7651916cd43dd8448eb211c80319c")
	ctx, _ = xctx.WithSpanID(ctx, "b7ad6b7169203331")

	req := httptest.NewRequest("GET", "/downstream", nil)
	xtrace.InjectToRequest(ctx, req)

	fmt.Println("X-Trace-ID:", req.Header.Get("X-Trace-ID"))
	fmt.Println("traceparent:", req.Header.Get("traceparent"))
	// Output:
	// X-Trace-ID: 0af7651916cd43dd8448eb211c80319c
	// traceparent: 00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-00
}

func ExampleInjectTraceToHeader() {
	h := http.Header{}
	info := xtrace.TraceInfo{
		TraceID:    "0af7651916cd43dd8448eb211c80319c",
		SpanID:     "b7ad6b7169203331",
		RequestID:  "req-123",
		Tracestate: "vendor=opaque",
	}
	xtrace.InjectTraceToHeader(h, info)

	fmt.Println("traceparent:", h.Get("traceparent"))
	fmt.Println("tracestate:", h.Get("tracestate"))
	// Output:
	// traceparent: 00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-00
	// tracestate: vendor=opaque
}

func ExampleExtractFromMetadata() {
	md := metadata.Pairs(
		"traceparent", "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01",
	)

	info := xtrace.ExtractFromMetadata(md)
	fmt.Println("TraceID:", info.TraceID)
	fmt.Println("SpanID:", info.SpanID)
	// Output:
	// TraceID: 0af7651916cd43dd8448eb211c80319c
	// SpanID: b7ad6b7169203331
}

func ExampleInjectToOutgoingContext() {
	ctx := context.Background()
	ctx, _ = xctx.WithTraceID(ctx, "0af7651916cd43dd8448eb211c80319c")
	ctx, _ = xctx.WithSpanID(ctx, "b7ad6b7169203331")
	ctx, _ = xctx.WithRequestID(ctx, "req-123")

	ctx = xtrace.InjectToOutgoingContext(ctx)

	md, _ := metadata.FromOutgoingContext(ctx)
	fmt.Println("x-trace-id:", md.Get("x-trace-id")[0])
	fmt.Println("traceparent:", md.Get("traceparent")[0])
	// Output:
	// x-trace-id: 0af7651916cd43dd8448eb211c80319c
	// traceparent: 00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-00
}

func ExampleTraceInfo_IsEmpty() {
	empty := xtrace.TraceInfo{}
	fmt.Println("empty:", empty.IsEmpty())

	withTrace := xtrace.TraceInfo{TraceID: "abc"}
	fmt.Println("with trace:", withTrace.IsEmpty())
	// Output:
	// empty: true
	// with trace: false
}
