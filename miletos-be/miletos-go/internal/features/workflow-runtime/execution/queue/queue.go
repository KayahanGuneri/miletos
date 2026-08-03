package queue

import "context"

type RecordDisposition string

const (
	RecordHandled    RecordDisposition = "HANDLED"
	RecordRetry      RecordDisposition = "RETRY"
	RecordDeadLetter RecordDisposition = "DEAD_LETTER"
)

type RecordResult struct {
	Disposition RecordDisposition
	Code        string
	Err         error
}

type Queue interface {
	Push(context.Context, string, string, []byte) error
	Consume(context.Context, string, func(context.Context, []byte) RecordResult) error
}
