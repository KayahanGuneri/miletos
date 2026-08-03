package httptrigger

import "time"

type BindingStatus string
type ResolvedMode string

const (
	StatusActive   BindingStatus = "ACTIVE"
	StatusDisabled BindingStatus = "DISABLED"
	ModeAsync      ResolvedMode  = "ASYNC"
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
	Status           BindingStatus
	ResolvedMode     ResolvedMode
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DisabledAt       *time.Time
	LockVersion      int64
}

type CreatedBinding struct {
	Binding
	PublicURL string
}
