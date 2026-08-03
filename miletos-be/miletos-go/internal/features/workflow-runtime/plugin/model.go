package plugin

type NodeDefinition struct {
	Type                 string
	Version              string
	DisplayName          string
	Description          string
	InputMode            string
	InputPorts           []Port
	OutputPorts          []Port
	InputEdgeConstraint  EdgeConstraint
	OutputEdgeConstraint EdgeConstraint
}

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
