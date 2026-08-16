package model

const PersistedRoutedOutputFormat = "miletos.workflow.routed-output.v1"

type NodeRoutingOutcome struct {
	Explicit     bool
	EdgePayloads map[string]any
}

type PersistedRoutedOutput struct {
	Format       string         `json:"format"`
	Output       map[string]any `json:"output"`
	EdgePayloads map[string]any `json:"edgePayloads"`
}
