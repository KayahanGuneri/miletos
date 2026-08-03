package plugin

type NodeError struct {
	Category string
	Code     string
	Message  string
	CanRetry bool
}

func (nodeError *NodeError) Error() string {
	return nodeError.Message
}
