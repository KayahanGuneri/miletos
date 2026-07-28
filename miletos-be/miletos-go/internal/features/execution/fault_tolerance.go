package execution

import (
	"math"
	"time"
)

type AttemptNumber int16

const InitialAttemptNumber AttemptNumber = 1

func NewAttemptNumber(value int16) (AttemptNumber, error) {
	attempt := AttemptNumber(value)
	if !attempt.IsValid() {
		return 0, newValidationError(
			"attempt", "must be greater than zero")
	}
	return attempt, nil
}

func (attempt AttemptNumber) Int16() int16 {
	return int16(attempt)
}

func (attempt AttemptNumber) IsValid() bool {
	return attempt > 0
}

func (attempt AttemptNumber) Next() (AttemptNumber, error) {
	if !attempt.IsValid() {
		return 0, newValidationError(
			"attempt", "must be greater than zero")
	}
	if attempt == AttemptNumber(math.MaxInt16) {
		return 0, newValidationError(
			"attempt", "must allow increment")
	}
	return attempt + 1, nil
}

type RetryPolicy struct {
	maxAttempts    AttemptNumber
	initialBackoff time.Duration
	maxBackoff     time.Duration
}

func NewRetryPolicy(
	maxAttempts AttemptNumber,
	initialBackoff time.Duration,
	maxBackoff time.Duration,
) (RetryPolicy, error) {
	if !maxAttempts.IsValid() {
		return RetryPolicy{}, newValidationError(
			"maxAttempts", "must be greater than zero")
	}
	if initialBackoff <= 0 {
		return RetryPolicy{}, newValidationError(
			"initialBackoff", "must be greater than zero")
	}
	if maxBackoff < initialBackoff {
		return RetryPolicy{}, newValidationError(
			"maxBackoff", "must be greater than or equal to initialBackoff")
	}
	return RetryPolicy{
		maxAttempts:    maxAttempts,
		initialBackoff: initialBackoff,
		maxBackoff:     maxBackoff,
	}, nil
}

func (policy RetryPolicy) MaxAttempts() AttemptNumber {
	return policy.maxAttempts
}

func (policy RetryPolicy) InitialBackoff() time.Duration {
	return policy.initialBackoff
}

func (policy RetryPolicy) MaxBackoff() time.Duration {
	return policy.maxBackoff
}

func (policy RetryPolicy) IsValid() bool {
	_, err := NewRetryPolicy(
		policy.maxAttempts,
		policy.initialBackoff,
		policy.maxBackoff,
	)
	return err == nil
}

type RetryDecisionKind string

const (
	RetryDecisionRetry      RetryDecisionKind = "RETRY"
	RetryDecisionDoNotRetry RetryDecisionKind = "DO_NOT_RETRY"
	RetryDecisionExhausted  RetryDecisionKind = "EXHAUSTED"
)

func (kind RetryDecisionKind) String() string {
	return string(kind)
}

func (kind RetryDecisionKind) IsValid() bool {
	switch kind {
	case RetryDecisionRetry,
		RetryDecisionDoNotRetry,
		RetryDecisionExhausted:
		return true
	default:
		return false
	}
}

type RetryDecisionReason string

const (
	RetryReasonScheduled               RetryDecisionReason = "RETRY_SCHEDULED"
	RetryReasonFailureNotRetryable     RetryDecisionReason = "FAILURE_NOT_RETRYABLE"
	RetryReasonCategoryNotRetryable    RetryDecisionReason = "CATEGORY_NOT_RETRYABLE"
	RetryReasonMaxAttemptsReached      RetryDecisionReason = "MAX_ATTEMPTS_REACHED"
	RetryReasonDeadlineWouldBeExceeded RetryDecisionReason = "DEADLINE_WOULD_BE_EXCEEDED"
)

func (reason RetryDecisionReason) String() string {
	return string(reason)
}

func (reason RetryDecisionReason) IsValid() bool {
	switch reason {
	case RetryReasonScheduled,
		RetryReasonFailureNotRetryable,
		RetryReasonCategoryNotRetryable,
		RetryReasonMaxAttemptsReached,
		RetryReasonDeadlineWouldBeExceeded:
		return true
	default:
		return false
	}
}

type RetryDecision struct {
	kind           RetryDecisionKind
	reason         RetryDecisionReason
	currentAttempt AttemptNumber
	nextAttempt    AttemptNumber
	backoff        time.Duration
}

func NewRetryDecision(
	kind RetryDecisionKind,
	reason RetryDecisionReason,
	currentAttempt AttemptNumber,
	nextAttempt AttemptNumber,
	backoff time.Duration,
) (RetryDecision, error) {
	if !kind.IsValid() {
		return RetryDecision{}, newValidationError(
			"retryDecision.kind", "must contain a supported decision kind")
	}
	if !reason.IsValid() {
		return RetryDecision{}, newValidationError(
			"retryDecision.reason", "must contain a supported decision reason")
	}
	if !currentAttempt.IsValid() {
		return RetryDecision{}, newValidationError(
			"retryDecision.currentAttempt", "must be greater than zero")
	}
	if kind == RetryDecisionRetry {
		if reason != RetryReasonScheduled {
			return RetryDecision{}, newValidationError(
				"retryDecision.reason", "must be RETRY_SCHEDULED for a retry decision")
		}
		expectedNext, err := currentAttempt.Next()
		if err != nil {
			return RetryDecision{}, newValidationError(
				"retryDecision.nextAttempt", "current attempt does not allow a next attempt")
		}
		if nextAttempt != expectedNext {
			return RetryDecision{}, newValidationError(
				"retryDecision.nextAttempt", "must immediately follow currentAttempt")
		}
		if backoff <= 0 {
			return RetryDecision{}, newValidationError(
				"retryDecision.backoff", "must be greater than zero")
		}
	} else {
		if nextAttempt != 0 {
			return RetryDecision{}, newValidationError(
				"retryDecision.nextAttempt", "must be empty when retry is not scheduled")
		}
		if backoff != 0 {
			return RetryDecision{}, newValidationError(
				"retryDecision.backoff", "must be empty when retry is not scheduled")
		}
		switch kind {
		case RetryDecisionDoNotRetry:
			if reason != RetryReasonFailureNotRetryable &&
				reason != RetryReasonCategoryNotRetryable &&
				reason != RetryReasonDeadlineWouldBeExceeded {
				return RetryDecision{}, newValidationError(
					"retryDecision.reason", "does not match a do-not-retry decision")
			}
		case RetryDecisionExhausted:
			if reason != RetryReasonMaxAttemptsReached {
				return RetryDecision{}, newValidationError(
					"retryDecision.reason", "must be MAX_ATTEMPTS_REACHED for an exhausted decision")
			}
		}
	}
	return RetryDecision{
		kind:           kind,
		reason:         reason,
		currentAttempt: currentAttempt,
		nextAttempt:    nextAttempt,
		backoff:        backoff,
	}, nil
}

func (decision RetryDecision) Kind() RetryDecisionKind {
	return decision.kind
}

func (decision RetryDecision) Reason() RetryDecisionReason {
	return decision.reason
}

func (decision RetryDecision) CurrentAttempt() AttemptNumber {
	return decision.currentAttempt
}

func (decision RetryDecision) NextAttempt() (AttemptNumber, bool) {
	if decision.kind != RetryDecisionRetry {
		return 0, false
	}
	return decision.nextAttempt, true
}

func (decision RetryDecision) Backoff() (time.Duration, bool) {
	if decision.kind != RetryDecisionRetry {
		return 0, false
	}
	return decision.backoff, true
}

func (decision RetryDecision) IsValid() bool {
	_, err := NewRetryDecision(
		decision.kind,
		decision.reason,
		decision.currentAttempt,
		decision.nextAttempt,
		decision.backoff,
	)
	return err == nil
}
