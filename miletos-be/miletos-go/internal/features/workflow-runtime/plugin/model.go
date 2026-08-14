package plugin

const (
	NodeInputSingle = "SINGLE"
	NodeInputMulti  = "MULTI"
)

type Port struct {
	Name        string
	DisplayName string
	Description string
}

type EdgeConstraint struct {
	Minimum uint
	Maximum *uint
}
