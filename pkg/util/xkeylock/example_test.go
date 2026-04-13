package xkeylock_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/omeyang/xkit/pkg/util/xkeylock"
)

func ExampleNew() {
	kl, err := xkeylock.New()
	if err != nil {
		panic(err)
	}

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

func ExampleLocker_TryAcquire() {
	kl, err := xkeylock.New()
	if err != nil {
		panic(err)
	}

	// First acquire
	h1, err := kl.TryAcquire("resource:123")
	if err != nil {
		panic(err)
	}
	fmt.Println("first acquired:", h1 != nil)

	// Second acquire — lock is occupied
	_, err = kl.TryAcquire("resource:123")
	fmt.Println("lock occupied:", errors.Is(err, xkeylock.ErrLockOccupied))

	if err := h1.Unlock(); err != nil {
		panic(err)
	}
	if err := kl.Close(); err != nil {
		panic(err)
	}
	// Output:
	// first acquired: true
	// lock occupied: true
}
