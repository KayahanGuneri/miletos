package plugin

import "errors"

var (
	ErrNodeRegistrationNotFound = errors.New("node registration not found")
	ErrScenarioStartUnavailable = errors.New("node scenario-start lifecycle is unavailable")
)

type NodeError struct {
	Category string
	Code     string
	Message  string
	CanRetry bool
}

func (nodeError *NodeError) Error() string {
	return nodeError.Message
}
