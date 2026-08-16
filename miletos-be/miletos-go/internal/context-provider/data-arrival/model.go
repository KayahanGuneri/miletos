package dataarrival

type Binding struct {
	CompanyID        string
	WorkflowID       string
	WorkflowRevision uint64
	SnapshotID       string
	NodeID           string
	PluginType       string
}
