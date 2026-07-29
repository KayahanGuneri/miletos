package modules_test

import (
	"context"
	"testing"
	"time"

	"miletos-go/internal/engine/modules"
)

func TestDecideRetryUsesSuppliedTimeAndExactDeadlineBoundary(t *testing.T) {
	now := time.Date(2026, time.July, 28, 10, 30, 0, 0, time.UTC)
	tests := []struct {
		name      string
		attempt   int
		maximum   int
		retryable bool
		delay     time.Duration
		deadline  time.Time
		wantRetry bool
	}{
		{
			name: "attempts remaining", attempt: 1, maximum: 3,
			retryable: true, delay: 250 * time.Millisecond, wantRetry: true,
		},
		{
			name: "maximum reached", attempt: 3, maximum: 3,
			retryable: true, delay: time.Second,
		},
		{
			name: "attempt exceeds maximum", attempt: 4, maximum: 3,
			retryable: true, delay: time.Second,
		},
		{
			name: "invalid maximum", attempt: 1, maximum: 0,
			retryable: true, delay: time.Second,
		},
		{
			name: "non retryable failure", attempt: 1, maximum: 3,
			delay: time.Second,
		},
		{
			name: "zero delay", attempt: 1, maximum: 2,
			retryable: true, wantRetry: true,
		},
		{
			name: "deadline before now", attempt: 1, maximum: 2,
			retryable: true, deadline: now.Add(-time.Nanosecond),
		},
		{
			name: "deadline equal to now with zero delay", attempt: 1, maximum: 2,
			retryable: true, deadline: now, wantRetry: true,
		},
		{
			name: "next attempt before deadline", attempt: 1, maximum: 2,
			retryable: true, delay: time.Second,
			deadline: now.Add(time.Second + time.Nanosecond), wantRetry: true,
		},
		{
			name: "next attempt equal to deadline", attempt: 1, maximum: 2,
			retryable: true, delay: time.Second,
			deadline: now.Add(time.Second), wantRetry: true,
		},
		{
			name: "next attempt after deadline", attempt: 1, maximum: 2,
			retryable: true, delay: time.Second,
			deadline: now.Add(time.Second - time.Nanosecond),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := modules.DecideRetry(
				context.Background(), test.attempt, test.maximum, test.retryable,
				test.delay, now, test.deadline,
			)
			if result.Retry != test.wantRetry {
				t.Fatalf("DecideRetry().Retry = %v, want %v", result.Retry, test.wantRetry)
			}
			wantDelay := time.Duration(0)
			if test.wantRetry {
				wantDelay = test.delay
			}
			if result.Delay != wantDelay {
				t.Fatalf("DecideRetry().Delay = %v, want %v", result.Delay, wantDelay)
			}
			if result.Retry {
				wantNextAttempt := now.Add(test.delay)
				if nextAttempt := now.Add(result.Delay); !nextAttempt.Equal(wantNextAttempt) {
					t.Fatalf("next attempt = %v, want %v", nextAttempt, wantNextAttempt)
				}
			}
		})
	}
}

func TestDecideRetryHonorsContextCancellation(t *testing.T) {
	now := time.Date(2026, time.July, 28, 10, 30, 0, 0, time.UTC)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result := modules.DecideRetry(ctx, 1, 3, true, time.Second, now, time.Time{})

	if result.Retry {
		t.Fatal("DecideRetry() retried canceled context")
	}
}

func TestDecideRetryIsDeterministicForEquivalentInstants(t *testing.T) {
	utc := time.Date(2026, time.July, 28, 10, 30, 0, 0, time.UTC)
	istanbul := utc.In(time.FixedZone("UTC+3", 3*60*60))
	deadline := utc.Add(time.Minute)

	first := modules.DecideRetry(
		context.Background(), 1, 3, true, 5*time.Second, utc, deadline,
	)
	second := modules.DecideRetry(
		context.Background(), 1, 3, true, 5*time.Second, utc, deadline,
	)
	equivalentZone := modules.DecideRetry(
		context.Background(), 1, 3, true, 5*time.Second, istanbul, deadline,
	)

	if first != second {
		t.Fatalf("identical calls differ: first=%#v second=%#v", first, second)
	}
	if first != equivalentZone {
		t.Fatalf("equivalent instants differ: UTC=%#v UTC+3=%#v", first, equivalentZone)
	}
}
