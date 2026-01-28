package xkeylock_test

import (
	"context"
	"fmt"

	"github.com/omeyang/xkit/pkg/util/xkeylock"
)

func ExampleNew() {
	kl := xkeylock.New()

	handle, err := kl.Acquire(context.Background(), "resource:123")
	if err != nil {
		panic(err)
	}

	fmt.Println("lock acquired for:", handle.Key())

	if err := handle.Unlock(); err != nil {
		panic(err)
	}
	if err := kl.Close(); err != nil {
		panic(err)
	}
	// Output:
	// lock acquired for: resource:123
}

func ExampleKeyLock_TryAcquire() {
	kl := xkeylock.New()

	// First acquire
	h1, err := kl.TryAcquire("resource:123")
	if err != nil {
		panic(err)
	}
	if h1 == nil {
		fmt.Println("lock occupied")
		return
	}

	// Second acquire — lock is occupied
	h2, err := kl.TryAcquire("resource:123")
	if err != nil {
		panic(err)
	}
	fmt.Println("first acquired:", h1 != nil)
	fmt.Println("second acquired:", h2 != nil)

	if err := h1.Unlock(); err != nil {
		panic(err)
	}
	if err := kl.Close(); err != nil {
		panic(err)
	}
	// Output:
	// first acquired: true
	// second acquired: false
}
