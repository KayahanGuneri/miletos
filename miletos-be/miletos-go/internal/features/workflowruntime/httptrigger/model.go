package httptrigger

import "time"

const (
	StatusActive   = "ACTIVE"
	StatusDisabled = "DISABLED"
)

type Binding struct {
	ID               string
	CompanyID        string
	WorkflowID       string
	WorkflowRevision uint64
	SnapshotID       string
	TriggerNodeID    string
	Method           string
	TokenHash        []byte
	Status           string
	ResolvedMode     string
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DisabledAt       *time.Time
	LockVersion      int64
}

type CreatedBinding struct {
	Binding
	PublicURL string
}
