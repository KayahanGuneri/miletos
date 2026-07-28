package engine

import (
	"context"
	"time"
)

type RetryWaiter interface {
	Wait(context.Context, time.Duration) error
}

type SystemRetryWaiter struct{}

func (SystemRetryWaiter) Wait(ctx context.Context, duration time.Duration) error {
	if ctx == nil {
		return newValidationError("retryWait.context", "must not be nil")
	}
	if duration <= 0 {
		return newValidationError("retryWait.duration", "must be greater than zero")
	}
	timer := time.NewTimer(duration)
	defer func() {
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
	}()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
