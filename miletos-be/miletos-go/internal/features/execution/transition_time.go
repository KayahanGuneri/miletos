package execution

import "time"

func validateCreatedAt(createdAt time.Time) error {
	if createdAt.IsZero() {
		return newTimestampError("createdAt",
			"must not be zero")
	}
	return nil
}
func validateTransitionTimestamp(at time.Time,
	createdAt time.Time, startedAt time.Time, mustNotPrecedeStartedAt bool,
) error {
	if at.IsZero() {
		return newTimestampError(
			"transitionAt", "must not be zero")
	}
	if at.Before(createdAt) {
		return newTimestampError("transitionAt", "must not be before createdAt")
	}
	if mustNotPrecedeStartedAt && !startedAt.IsZero() && at.Before(startedAt) {
		return newTimestampError("transitionAt", "must not be before startedAt")
	}
	return nil
}
