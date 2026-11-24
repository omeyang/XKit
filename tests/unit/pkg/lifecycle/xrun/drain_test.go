package xrun

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDrainerWaitsForInflight(t *testing.T) {
	d := NewDrainer()
	if err := d.Begin("tenant-a"); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- d.Drain(context.Background(), "tenant-a") }()

	select {
	case err := <-done:
		t.Fatalf("drain returned before End: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	if d.Accepting("tenant-a") {
		t.Fatal("draining resource still accepts work")
	}
	if err := d.Begin("tenant-a"); !errors.Is(err, ErrDraining) {
		t.Fatalf("Begin error = %v", err)
	}
	d.End("tenant-a")
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if d.Accepting("tenant-a") {
		t.Fatal("completed drain reopened the resource")
	}
}

func TestDrainerSeparatesResources(t *testing.T) {
	d := NewDrainer()
	if err := d.Begin("tenant-a"); err != nil {
		t.Fatal(err)
	}
	if err := d.Begin("tenant-b"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := d.Drain(ctx, "tenant-a"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("tenant-a Drain error = %v", err)
	}
	if !d.Accepting("tenant-b") {
		t.Fatal("tenant-a drain closed tenant-b")
	}
	d.End("tenant-b")
	if err := d.Drain(context.Background(), "tenant-b"); err != nil {
		t.Fatal(err)
	}
}

func TestDrainerHonorsContext(t *testing.T) {
	d := NewDrainer()
	if err := d.Begin("tenant-a"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := d.Drain(ctx, "tenant-a"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Drain error = %v", err)
	}
	d.End("tenant-a")
	if err := d.Drain(context.Background(), "tenant-a"); err != nil {
		t.Fatal(err)
	}
}

func TestDrainerEmptyAndUnknownEnd(t *testing.T) {
	d := NewDrainer()
	if !d.Accepting("unknown") {
		t.Fatal("unknown resource should accept work")
	}
	d.End("unknown")
	if err := d.Drain(context.Background(), "unknown"); err != nil {
		t.Fatal(err)
	}
}
