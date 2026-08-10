package cronexpr

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

const DefaultTimezone = "UTC"

type ValidationError struct {
	Code    string
	Message string
}

func (err *ValidationError) Error() string {
	return err.Message
}

var (
	ErrExpressionRequired = &ValidationError{Code: "EXPRESSION_REQUIRED", Message: "configuration.expression is required"}
	ErrExpressionInvalid  = &ValidationError{Code: "EXPRESSION_INVALID", Message: "configuration.expression must be a valid five-field cron expression"}
	ErrTimezoneInvalid    = &ValidationError{Code: "TIMEZONE_INVALID", Message: "configuration.timezone must be a valid IANA timezone"}
)

var parser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

type Schedule struct {
	Expression string
	Timezone   string
	Location   *time.Location
	Parsed     cron.Schedule
}

func Parse(expression string, timezone string) (Schedule, error) {
	normalizedExpression := strings.Join(strings.Fields(expression), " ")
	if normalizedExpression == "" {
		return Schedule{}, ErrExpressionRequired
	}
	if len(strings.Fields(normalizedExpression)) != 5 {
		return Schedule{}, ErrExpressionInvalid
	}
	parsed, err := parser.Parse(normalizedExpression)
	if err != nil {
		return Schedule{}, fmt.Errorf("%w: %v", ErrExpressionInvalid, err)
	}
	normalizedTimezone := strings.TrimSpace(timezone)
	if normalizedTimezone == "" {
		normalizedTimezone = DefaultTimezone
	}
	location, err := time.LoadLocation(normalizedTimezone)
	if err != nil {
		return Schedule{}, fmt.Errorf("%w: %v", ErrTimezoneInvalid, err)
	}
	return Schedule{
		Expression: normalizedExpression,
		Timezone:   location.String(),
		Location:   location,
		Parsed:     parsed,
	}, nil
}

func ValidationCode(err error) (string, bool) {
	var validationError *ValidationError
	if errors.As(err, &validationError) {
		return validationError.Code, true
	}
	return "", false
}

func Next(expression string, timezone string, from time.Time) (time.Time, Schedule, error) {
	schedule, err := Parse(expression, timezone)
	if err != nil {
		return time.Time{}, Schedule{}, err
	}
	next := schedule.Parsed.Next(from.In(schedule.Location)).UTC()
	return next, schedule, nil
}
