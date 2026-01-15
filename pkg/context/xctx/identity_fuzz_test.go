package xctx_test

import (
	"context"
	"testing"

	"github.com/omeyang/xkit/pkg/context/xctx"
)

var identityFuzzSeeds = [][3]string{
	{"p1", "t1", "n1"},
	{"", "", ""},
	{"platform", "", "租户名"},
}

var identityFuzzConfig = fuzzThreeFieldsConfig{
	FieldNames: [3]string{"PlatformID", "TenantID", "TenantName"},
	Setters:    [3]func(context.Context, string) (context.Context, error){xctx.WithPlatformID, xctx.WithTenantID, xctx.WithTenantName},
	GetFields: func(ctx context.Context) [3]string {
		id := xctx.GetIdentity(ctx)
		return [3]string{id.PlatformID, id.TenantID, id.TenantName}
	},
}

func FuzzIdentityFields(f *testing.F) {
	runThreeFieldsFuzz(f, identityFuzzSeeds, identityFuzzConfig)
}
