package xelection_test

import (
	"context"
	"fmt"
	"time"

	"github.com/omeyang/xkit/pkg/distributed/xelection"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// ExampleNewEtcdElection 展示标准的竞选 → fencing 写 → 监听丢失的用法。
// 本示例不含 Output 指令，因其依赖真实 etcd。
func ExampleNewEtcdElection() {
	client, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{"localhost:2379"},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		return
	}
	defer closeQuietly(client.Close)

	election, err := xelection.NewEtcdElection(client, "/myapp/leader/",
		xelection.WithTTL(15))
	if err != nil {
		return
	}
	defer closeQuietly(func() error { return election.Close(context.Background()) })

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	leader, err := election.Campaign(ctx, "node-1")
	if err != nil {
		return
	}
	defer closeQuietly(func() error { return leader.Resign(context.Background()) })

	// 监听 leadership 丢失：另一 goroutine 中触发降级或重新 Campaign。
	go func() {
		<-leader.Lost()
		// handle loss
	}()

	// 写 etcd 前的 fencing 检查：
	if err := leader.CheckLeader(); err != nil {
		return
	}

	fmt.Println("leader:", leader.CandidateID())
}

// closeQuietly 吞掉 defer 中的 cleanup 错误，仅用于示例。
func closeQuietly(fn func() error) {
	if err := fn(); err != nil {
		_ = err
	}
}
