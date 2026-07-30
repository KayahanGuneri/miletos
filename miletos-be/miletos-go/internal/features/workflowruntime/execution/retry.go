package execution

import (
	"context"
	"time"
)

type RetryResult struct {
	Retry bool
	Delay time.Duration
}

func DecideRetry(
	ctx context.Context,
	currentAttempt int,
	maximumAttempts int,
	retryable bool,
	delay time.Duration,
	now time.Time,
	deadline time.Time,
) RetryResult {
	if !retryable || currentAttempt >= maximumAttempts || ctx.Err() != nil {
		return RetryResult{}
	}
	if !deadline.IsZero() && now.Add(delay).After(deadline) {
		return RetryResult{}
	}
	return RetryResult{Retry: true, Delay: delay}
}
