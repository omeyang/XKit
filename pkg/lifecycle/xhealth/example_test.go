package xhealth_test

import (
	"context"
	"fmt"

	"github.com/omeyang/xkit/pkg/lifecycle/xhealth"
)

func Example() {
	h, err := xhealth.New(
		xhealth.WithAddr(":8081"),
		xhealth.WithCacheTTL(0),
	)
	if err != nil {
		panic(err)
	}

	err = h.AddLivenessCheck("goroutines", xhealth.CheckConfig{
		Check: xhealth.GoroutineCountCheck(10000),
	})
	if err != nil {
		panic(err)
	}

	result, err := h.CheckLiveness(context.Background())
	if err != nil {
		panic(err)
	}

	fmt.Println(result.Status)
	// Output:
	// up
}

func ExampleHealth_CheckReadiness() {
	h, err := xhealth.New(xhealth.WithCacheTTL(0))
	if err != nil {
		panic(err)
	}

	err = h.AddReadinessCheck("db", xhealth.CheckConfig{
		Check: func(_ context.Context) error { return nil },
	})
	if err != nil {
		panic(err)
	}

	result, err := h.CheckReadiness(context.Background())
	if err != nil {
		panic(err)
	}

	fmt.Println(result.Status)
	fmt.Println(result.Checks["db"].Status)
	// Output:
	// up
	// up
}

func ExampleHealth_Shutdown() {
	h, err := xhealth.New()
	if err != nil {
		panic(err)
	}

	// Shutdown 标记所有端点为不健康
	h.Shutdown()

	result, err := h.CheckReadiness(context.Background())
	if err != nil {
		panic(err)
	}

	fmt.Println(result.Status)
	// Output:
	// down
}
