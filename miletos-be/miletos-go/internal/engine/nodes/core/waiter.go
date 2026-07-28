package core

import (
	"context"
	"errors"
	"time"
)

type Waiter interface {
	Wait(context.Context, time.Duration,
	) error
}
type WaiterFunc func(context.Context, time.Duration,
) error

func (waiter WaiterFunc) Wait(
	ctx context.Context, duration time.Duration) error {
	if waiter == nil {
		return errors.New("waiter function must not be nil")
	}
	return waiter(ctx, duration)
}

type TimerWaiter struct{}

func (TimerWaiter) Wait(
	ctx context.Context, duration time.Duration) error {
	if ctx == nil {
		return errors.New("wait context must not be nil")
	}
	if duration <= 0 {
		return errors.New("wait duration must be greater than zero")
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
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
