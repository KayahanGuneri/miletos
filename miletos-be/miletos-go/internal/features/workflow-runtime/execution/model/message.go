package model

type NodeJob struct {
	CompanyID       string          `json:"companyId"`
	WorkflowID      string          `json:"workflowId"`
	ExecutionID     string          `json:"executionId"`
	NodeID          string          `json:"nodeId"`
	NodeExecutionID string          `json:"nodeExecutionId"`
	Attempt         int             `json:"attempt"`
	CorrelationID   string          `json:"correlationId"`
	Origin          ExecutionOrigin `json:"origin"`
	Payload         any             `json:"payload,omitempty"`
}
