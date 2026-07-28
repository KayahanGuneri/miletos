package engine

import (
	"fmt"
	"time"

	"miletos-go/internal/engine/runtime"
	"miletos-go/internal/features/execution"
)

func DecideRetry(
	policy execution.RetryPolicy,
	currentAttempt execution.AttemptNumber,
	failure runtime.RuntimeFailure,
	now time.Time,
	deadline *time.Time,
) (execution.RetryDecision, error) {
	if !policy.IsValid() {
		return execution.RetryDecision{}, newValidationError(
			"retryPolicy", "must be valid")
	}
	if !currentAttempt.IsValid() {
		return execution.RetryDecision{}, newValidationError(
			"currentAttempt", "must be valid")
	}
	if currentAttempt > policy.MaxAttempts() {
		return execution.RetryDecision{}, newValidationError(
			"currentAttempt", "must not exceed retry policy maxAttempts")
	}
	if !failure.IsValid() {
		return execution.RetryDecision{}, newValidationError(
			"failure", "must be valid")
	}
	normalizedNow, err := normalizeRetryTime("now", now)
	if err != nil {
		return execution.RetryDecision{}, err
	}
	normalizedDeadline, hasDeadline, err := normalizeRetryDeadline(deadline)
	if err != nil {
		return execution.RetryDecision{}, err
	}
	if !retryCategoryAllowed(failure.Category()) {
		return newRetryDecision(
			execution.RetryDecisionDoNotRetry,
			execution.RetryReasonCategoryNotRetryable,
			currentAttempt,
			0,
		)
	}
	if !failure.Retryable() {
		return newRetryDecision(
			execution.RetryDecisionDoNotRetry,
			execution.RetryReasonFailureNotRetryable,
			currentAttempt,
			0,
		)
	}
	if currentAttempt >= policy.MaxAttempts() {
		return newRetryDecision(
			execution.RetryDecisionExhausted,
			execution.RetryReasonMaxAttemptsReached,
			currentAttempt,
			0,
		)
	}
	backoff := retryBackoffValidated(policy, currentAttempt)
	if hasDeadline &&
		(!normalizedDeadline.After(normalizedNow) ||
			normalizedDeadline.Sub(normalizedNow) <= backoff) {
		return newRetryDecision(
			execution.RetryDecisionDoNotRetry,
			execution.RetryReasonDeadlineWouldBeExceeded,
			currentAttempt,
			0,
		)
	}
	nextAttempt, err := currentAttempt.Next()
	if err != nil {
		return execution.RetryDecision{}, fmt.Errorf(
			"calculate next retry attempt: %w", err)
	}
	return execution.NewRetryDecision(
		execution.RetryDecisionRetry,
		execution.RetryReasonScheduled,
		currentAttempt,
		nextAttempt,
		backoff,
	)
}

func RetryBackoff(
	policy execution.RetryPolicy,
	currentAttempt execution.AttemptNumber,
) (time.Duration, error) {
	if !policy.IsValid() {
		return 0, newValidationError(
			"retryPolicy", "must be valid")
	}
	if !currentAttempt.IsValid() {
		return 0, newValidationError(
			"currentAttempt", "must be valid")
	}
	if currentAttempt >= policy.MaxAttempts() {
		return 0, newValidationError(
			"currentAttempt", "must be lower than retry policy maxAttempts")
	}
	return retryBackoffValidated(policy, currentAttempt), nil
}

func retryBackoffValidated(
	policy execution.RetryPolicy,
	currentAttempt execution.AttemptNumber,
) time.Duration {
	backoff := policy.InitialBackoff()
	remainingDoublings := int(currentAttempt) - 1
	for remainingDoublings > 0 && backoff < policy.MaxBackoff() {
		if backoff > policy.MaxBackoff()/2 {
			return policy.MaxBackoff()
		}
		backoff *= 2
		remainingDoublings--
	}
	if backoff > policy.MaxBackoff() {
		return policy.MaxBackoff()
	}
	return backoff
}

func retryCategoryAllowed(category runtime.FailureCategory) bool {
	switch category {
	case runtime.FailureCategoryExecution,
		runtime.FailureCategoryDependency,
		runtime.FailureCategoryTimeout,
		runtime.FailureCategoryInternal:
		return true
	default:
		return false
	}
}

func newRetryDecision(
	kind execution.RetryDecisionKind,
	reason execution.RetryDecisionReason,
	currentAttempt execution.AttemptNumber,
	backoff time.Duration,
) (execution.RetryDecision, error) {
	return execution.NewRetryDecision(
		kind,
		reason,
		currentAttempt,
		0,
		backoff,
	)
}

func normalizeRetryTime(field string, value time.Time) (time.Time, error) {
	if value.IsZero() {
		return time.Time{}, newValidationError(
			field, "must not be zero")
	}
	return value.UTC(), nil
}

func normalizeRetryDeadline(
	value *time.Time,
) (time.Time, bool, error) {
	if value == nil {
		return time.Time{}, false, nil
	}
	normalized, err := normalizeRetryTime("deadline", *value)
	if err != nil {
		return time.Time{}, false, err
	}
	return normalized, true, nil
}
