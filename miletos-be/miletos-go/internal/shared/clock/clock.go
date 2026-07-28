package clock

import "time"

// Clock is the shared technical boundary for UTC timestamp sources.
type Clock interface {
	Now() time.Time
}

type Function func() time.Time

func From(now func() time.Time) Function {
	return Function(now)
}

func (clock Function) Now() time.Time {
	if clock == nil {
		return time.Time{}
	}
	return clock().UTC()
}

type SystemClock struct{}

func System() SystemClock {
	return SystemClock{}
}

func (SystemClock) Now() time.Time {
	return time.Now().UTC()
}
