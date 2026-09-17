package xkafka

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/omeyang/xkit/pkg/resilience/xretry"
	"github.com/stretchr/testify/assert"
)

// =============================================================================
// runConsumeLoop Tests
// =============================================================================

func TestRunConsumeLoop_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	count := 0
	consume := func(_ context.Context) error {
		count++
		if count >= 3 {
			cancel()
		}
		return nil
	}

	var errorsCount atomic.Int64
	err := runConsumeLoop(ctx, consume, &errorsCount, nil)
	assert.ErrorIs(t, err, context.Canceled)
	assert.GreaterOrEqual(t, count, 3)
}

func TestRunConsumeLoop_ErrorIncrementsCount(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	count := 0
	consume := func(_ context.Context) error {
		count++
		if count >= 3 {
			cancel()
			return nil
		}
		return errors.New("transient error")
	}

	var errorsCount atomic.Int64
	err := runConsumeLoop(ctx, consume, &errorsCount, nil)
	assert.ErrorIs(t, err, context.Canceled)
	// At least 2 errors should have been counted
	assert.GreaterOrEqual(t, errorsCount.Load(), int64(2))
}

func TestRunConsumeLoop_WithCustomBackoff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	count := 0
	consume := func(_ context.Context) error {
		count++
		if count >= 2 {
			cancel()
			return nil
		}
		return errors.New("error")
	}

	var errorsCount atomic.Int64
	backoff := xretry.NewFixedBackoff(0) // no delay
	err := runConsumeLoop(ctx, consume, &errorsCount, backoff)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestRunConsumeLoop_NilBackoff_UsesDefault(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	count := 0
	consume := func(_ context.Context) error {
		count++
		cancel()
		return nil
	}

	var errorsCount atomic.Int64
	err := runConsumeLoop(ctx, consume, &errorsCount, nil)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 1, count)
}
