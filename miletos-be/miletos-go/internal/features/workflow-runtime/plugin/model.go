package plugin

const (
	NodeInputSingle = "SINGLE"
	NodeInputMulti  = "MULTI"
)

type TriggerInputMode string

const (
	TriggerInputAll       TriggerInputMode = "ALL"
	TriggerInputAvailable TriggerInputMode = "AVAILABLE"
)

type Port struct {
	Name           string
	EdgeConstraint *EdgeConstraint
}

type EdgeConstraint struct {
	Minimum uint
	Maximum *uint
}

type ConnectionRestrictionSelector string

const (
	ConnectionRestrictionPrimary    ConnectionRestrictionSelector = "PRIMARY"
	ConnectionRestrictionNotPrimary ConnectionRestrictionSelector = "NOT_PRIMARY"
	ConnectionRestrictionPosition   ConnectionRestrictionSelector = "POSITION"
)

type ConnectionRestriction struct {
	From     string
	To       string
	Selector ConnectionRestrictionSelector
	Position uint
}
