package queue

import "context"

type Queue interface {
	Push(context.Context, string, string, []byte) error
	Consume(context.Context, string, func(context.Context, []byte) error) error
}
