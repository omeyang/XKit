package xctx_test

import (
	"context"
	"testing"

	"github.com/omeyang/xkit/pkg/context/xctx"
)

var traceFuzzSeeds = [][3]string{
	{"t1", "s1", "r1"},
	{"", "", ""},
	{"trace", "", "request"},
}

var traceFuzzConfig = fuzzThreeFieldsConfig{
	FieldNames: [3]string{"TraceID", "SpanID", "RequestID"},
	Setters:    [3]func(context.Context, string) (context.Context, error){xctx.WithTraceID, xctx.WithSpanID, xctx.WithRequestID},
	GetFields: func(ctx context.Context) [3]string {
		tr := xctx.GetTrace(ctx)
		return [3]string{tr.TraceID, tr.SpanID, tr.RequestID}
	},
}

func FuzzTraceFields(f *testing.F) {
	runThreeFieldsFuzz(f, traceFuzzSeeds, traceFuzzConfig)
}
