package xctx_test

import (
	"context"
	"testing"
)

// fuzzThreeFieldsConfig 定义三字段模糊测试的配置
type fuzzThreeFieldsConfig struct {
	FieldNames [3]string
	Setters    [3]func(context.Context, string) (context.Context, error)
	GetFields  func(context.Context) [3]string
}

// runThreeFieldsFuzz 执行三字段模糊测试，用于消除不同 fuzz 用例的代码重复。
func runThreeFieldsFuzz(f *testing.F, seeds [][3]string, cfg fuzzThreeFieldsConfig) {
	for _, seed := range seeds {
		f.Add(seed[0], seed[1], seed[2])
	}
	f.Fuzz(func(t *testing.T, v1, v2, v3 string) {
		values := [3]string{v1, v2, v3}
		ctx := context.Background()
		for i, setter := range cfg.Setters {
			if values[i] != "" {
				newCtx, err := setter(ctx, values[i])
				if err != nil {
					t.Fatalf("setter[%d]() error = %v", i, err)
				}
				ctx = newCtx
			}
		}
		fields := cfg.GetFields(ctx)
		for i, want := range values {
			if want != "" && fields[i] != want {
				t.Errorf("%s = %q, want %q", cfg.FieldNames[i], fields[i], want)
			}
		}
	})
}
