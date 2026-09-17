package xrun_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/omeyang/xkit/pkg/lifecycle/xrun"
)

func ExampleRun() {
	ctx, cancel := context.WithCancel(context.Background())

	// 模拟收到信号
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	err := xrun.Run(ctx, func(ctx context.Context) error {
		fmt.Println("service started")
		<-ctx.Done()
		fmt.Println("service stopped")
		return nil
	})

	if err != nil {
		fmt.Printf("error: %v\n", err)
	}
	fmt.Println("done")

	// Output:
	// service started
	// service stopped
	// done
}

func ExampleGroup() {
	ctx, cancel := context.WithCancel(context.Background())

	g, gCtx := xrun.NewGroup(ctx, xrun.WithName("my-service"))
	_ = gCtx // 用于演示 context 传播

	// 服务 1：等待取消
	g.Go(func(ctx context.Context) error {
		fmt.Println("service 1 started")
		<-ctx.Done()
		fmt.Println("service 1 stopped")
		return nil
	})

	// 服务 2：立即返回
	g.Go(func(ctx context.Context) error {
		fmt.Println("service 2 started")
		time.Sleep(50 * time.Millisecond)
		fmt.Println("service 2 exiting")
		return errors.New("done")
	})

	// 模拟外部取消（如果服务 2 没有先退出）
	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()

	err := g.Wait()
	fmt.Printf("group exited: %v\n", err)

	// Unordered output:
	// service 1 started
	// service 2 started
	// service 2 exiting
	// service 1 stopped
	// group exited: done
}

func ExampleTicker() {
	ctx, cancel := context.WithCancel(context.Background())

	count := 0
	g, _ := xrun.NewGroup(ctx)
	g.Go(xrun.Ticker(30*time.Millisecond, true, func(ctx context.Context) error {
		count++
		fmt.Printf("tick %d\n", count)
		if count >= 3 {
			cancel()
		}
		return nil
	}))

	if err := g.Wait(); err != nil {
		fmt.Printf("error: %v\n", err)
	}
	fmt.Println("done")

	// Output:
	// tick 1
	// tick 2
	// tick 3
	// done
}

func ExampleHTTPServer() {
	ctx, cancel := context.WithCancel(context.Background())

	// 使用 mock server
	server := newMockServer()

	g, _ := xrun.NewGroup(ctx)
	g.Go(xrun.HTTPServer(server, time.Second))

	// 模拟关闭
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	if err := g.Wait(); err != nil {
		fmt.Printf("error: %v\n", err)
	}
	fmt.Println("server stopped")

	// Output:
	// server stopped
}

type mockServer struct {
	ch chan struct{}
}

func newMockServer() *mockServer {
	return &mockServer{
		ch: make(chan struct{}),
	}
}

func (m *mockServer) ListenAndServe() error {
	<-m.ch
	return http.ErrServerClosed
}

func (m *mockServer) Shutdown(ctx context.Context) error {
	close(m.ch)
	return nil
}

func ExampleService() {
	ctx, cancel := context.WithCancel(context.Background())

	// 定义服务
	svc := xrun.ServiceFunc(func(ctx context.Context) error {
		fmt.Println("service running")
		<-ctx.Done()
		fmt.Println("service stopped")
		return nil
	})

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	if err := xrun.RunServices(ctx, svc); err != nil {
		fmt.Printf("error: %v\n", err)
	}
	fmt.Println("done")

	// Output:
	// service running
	// service stopped
	// done
}

func ExampleTimer() {
	g, _ := xrun.NewGroup(context.Background())

	g.Go(xrun.Timer(10*time.Millisecond, func(ctx context.Context) error {
		fmt.Println("timer fired")
		return nil
	}))

	if err := g.Wait(); err != nil {
		fmt.Printf("error: %v\n", err)
	}
	fmt.Println("done")

	// Output:
	// timer fired
	// done
}

func ExampleRunServicesWithOptions() {
	ctx, cancel := context.WithCancel(context.Background())

	svc := xrun.ServiceFunc(func(ctx context.Context) error {
		fmt.Println("service running")
		<-ctx.Done()
		fmt.Println("service stopped")
		return nil
	})

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := xrun.RunServicesWithOptions(ctx, []xrun.Option{
		xrun.WithName("my-app"),
	}, svc)
	if err != nil {
		fmt.Printf("error: %v\n", err)
	}
	fmt.Println("done")

	// Output:
	// service running
	// service stopped
	// done
}

func ExampleWaitForDone() {
	ctx, cancel := context.WithCancel(context.Background())

	g, _ := xrun.NewGroup(ctx)

	// WaitForDone 作为占位服务，保持 Group 运行
	g.Go(xrun.WaitForDone())

	// 另一个服务可以在需要时触发关闭
	g.Go(func(ctx context.Context) error {
		time.Sleep(50 * time.Millisecond)
		fmt.Println("triggering shutdown")
		cancel()
		return nil
	})

	if err := g.Wait(); err != nil {
		fmt.Printf("error: %v\n", err)
	}
	fmt.Println("done")

	// Output:
	// triggering shutdown
	// done
}
