package postgres

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"miletos-go/internal/engine/runtime"
	repository "miletos-go/internal/ports/persistence"
)

const asyncRuntimePersistenceVersion = 1

type persistedArtifactReferenceV1 struct {
	ID          string            `json:"id"`
	Location    string            `json:"location"`
	ContentType string            `json:"contentType"`
	SizeBytes   int64             `json:"sizeBytes"`
	Checksum    string            `json:"checksum"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}
type persistedPayloadV1 struct {
	Version     int                           `json:"version"`
	Source      string                        `json:"source"`
	ContentType string                        `json:"contentType"`
	InlineData  []byte                        `json:"inlineData,omitempty"`
	Artifact    *persistedArtifactReferenceV1 `json:"artifact,omitempty"`
	Metadata    map[string]string             `json:"metadata,omitempty"`
}
type persistedRoutedOutputsV1 struct {
	Version int                             `json:"version"`
	Outputs map[string][]persistedPayloadV1 `json:"outputs"`
}
type persistedContextChangesV1 struct {
	Version int                        `json:"version"`
	Set     map[string]json.RawMessage `json:"set"`
	Delete  []string                   `json:"delete"`
}
type persistedRuntimeFailureV1 struct {
	Version   int               `json:"version"`
	Category  string            `json:"category"`
	Code      string            `json:"code"`
	Message   string            `json:"message"`
	Retryable bool              `json:"retryable"`
	Details   map[string]string `json:"details,omitempty"`
}
type persistedNodeResultV1 struct {
	Version        int                        `json:"version"`
	Status         string                     `json:"status"`
	RoutedOutputs  *persistedRoutedOutputsV1  `json:"routedOutputs,omitempty"`
	TerminalOutput *persistedPayloadV1        `json:"terminalOutput,omitempty"`
	ContextChanges *persistedContextChangesV1 `json:"contextChanges,omitempty"`
	Failure        *persistedRuntimeFailureV1 `json:"failure,omitempty"`
}

func encodeRuntimePayload(payload runtime.Payload) ([]byte, error) {
	persisted, err := toPersistedPayload(payload)
	if err != nil {
		return nil, safeRuntimeCodecError("encode", "runtime payload", err)
	}
	return marshalBoundedRuntimeJSON("runtime payload", persisted)
}
func decodeRuntimePayload(encoded []byte) (runtime.Payload, error) {
	var persisted persistedPayloadV1
	if err := decodeStrictRuntimeJSON("runtime payload", encoded, &persisted); err != nil {
		return runtime.Payload{}, err
	}
	payload, err := fromPersistedPayload(persisted)
	if err != nil {
		return runtime.Payload{}, safeRuntimeCodecError("decode", "runtime payload", err)
	}
	return payload, nil
}
func encodeRoutedOutputs(outputs map[string][]runtime.Payload) ([]byte, error) {
	persisted := persistedRoutedOutputsV1{Version: asyncRuntimePersistenceVersion, Outputs: make(map[string][]persistedPayloadV1, len(outputs))}
	for port, payloads := range outputs {
		persistedPayloads := make([]persistedPayloadV1, len(payloads))
		for index, payload := range payloads {
			mapped, err := toPersistedPayload(payload)
			if err != nil {
				return nil, safeRuntimeCodecError("encode", "routed outputs", err)
			}
			persistedPayloads[index] = mapped
		}
		persisted.Outputs[port] = persistedPayloads
	}
	if _, err := runtime.NewNodeSuccessResult(outputs, runtime.ContextChanges{}); err != nil {
		return nil, safeRuntimeCodecError("encode", "routed outputs", err)
	}
	return marshalBoundedRuntimeJSON("routed outputs", persisted)
}
func decodeRoutedOutputs(encoded []byte) (map[string][]runtime.Payload, error) {
	var persisted persistedRoutedOutputsV1
	if err := decodeStrictRuntimeJSON("routed outputs", encoded, &persisted); err != nil {
		return nil, err
	}
	if persisted.Version != asyncRuntimePersistenceVersion {
		return nil, safeRuntimeCodecError("decode", "routed outputs", errors.New("unsupported persistence version"))
	}
	outputs := make(map[string][]runtime.Payload, len(persisted.Outputs))
	for port, payloads := range persisted.Outputs {
		decoded := make([]runtime.Payload, len(payloads))
		for index, payload := range payloads {
			mapped, err := fromPersistedPayload(payload)
			if err != nil {
				return nil, safeRuntimeCodecError("decode", "routed outputs", err)
			}
			decoded[index] = mapped
		}
		outputs[port] = decoded
	}
	result, err := runtime.NewNodeSuccessResult(outputs, runtime.ContextChanges{})
	if err != nil {
		return nil, safeRuntimeCodecError("decode", "routed outputs", err)
	}
	return result.OutputsSnapshot(), nil
}
func encodeContextChanges(changes runtime.ContextChanges) ([]byte, error) {
	if !changes.IsValid() {
		return nil, safeRuntimeCodecError("encode", "context changes", errors.New("invalid context changes"))
	}
	persisted := persistedContextChangesV1{Version: asyncRuntimePersistenceVersion, Set: make(map[string]json.RawMessage), Delete: changes.DeleteKeys()}
	for key, value := range changes.SetValues() {
		persisted.Set[key] = json.RawMessage(value.Bytes())
	}
	return marshalBoundedRuntimeJSON("context changes", persisted)
}
func decodeContextChanges(encoded []byte) (runtime.ContextChanges, error) {
	var persisted persistedContextChangesV1
	if err := decodeStrictRuntimeJSON("context changes", encoded, &persisted); err != nil {
		return runtime.ContextChanges{}, err
	}
	if persisted.Version != asyncRuntimePersistenceVersion {
		return runtime.ContextChanges{}, safeRuntimeCodecError("decode", "context changes", errors.New("unsupported persistence version"))
	}
	values := make(map[string]runtime.RuntimeValue, len(persisted.Set))
	for key, raw := range persisted.Set {
		value, err := runtime.NewRuntimeValue(raw)
		if err != nil {
			return runtime.ContextChanges{}, safeRuntimeCodecError("decode", "context changes", err)
		}
		values[key] = value
	}
	changes, err := runtime.NewContextChanges(values, persisted.Delete)
	if err != nil {
		return runtime.ContextChanges{}, safeRuntimeCodecError("decode", "context changes", err)
	}
	return changes, nil
}
func encodeRuntimeFailure(failure runtime.RuntimeFailure) ([]byte, error) {
	persisted, err := toPersistedFailure(failure)
	if err != nil {
		return nil, safeRuntimeCodecError("encode", "runtime failure", err)
	}
	return marshalBoundedRuntimeJSON("runtime failure", persisted)
}
func decodeRuntimeFailure(encoded []byte) (runtime.RuntimeFailure, error) {
	var persisted persistedRuntimeFailureV1
	if err := decodeStrictRuntimeJSON("runtime failure", encoded, &persisted); err != nil {
		return runtime.RuntimeFailure{}, err
	}
	failure, err := fromPersistedFailure(persisted)
	if err != nil {
		return runtime.RuntimeFailure{}, safeRuntimeCodecError("decode", "runtime failure", err)
	}
	return failure, nil
}
func encodeNodeResult(result runtime.NodeResult) ([]byte, error) {
	if !result.IsValid() {
		return nil, safeRuntimeCodecError("encode", "node result", errors.New("invalid node result"))
	}
	persisted := persistedNodeResultV1{Version: asyncRuntimePersistenceVersion, Status: result.Status().String()}
	if result.IsFailure() {
		failure, _ := result.Failure()
		mapped, err := toPersistedFailure(failure)
		if err != nil {
			return nil, safeRuntimeCodecError("encode", "node result", err)
		}
		persisted.Failure = &mapped
	} else {
		changesBytes, err := encodeContextChanges(result.ContextChanges())
		if err != nil {
			return nil, err
		}
		var changes persistedContextChangesV1
		if err := decodeStrictRuntimeJSON("context changes", changesBytes, &changes); err != nil {
			return nil, err
		}
		persisted.ContextChanges = &changes
		if terminal, exists := result.TerminalOutput(); exists {
			mapped, err := toPersistedPayload(terminal)
			if err != nil {
				return nil, safeRuntimeCodecError("encode", "node result", err)
			}
			persisted.TerminalOutput = &mapped
		} else {
			encoded, err := encodeRoutedOutputs(result.OutputsSnapshot())
			if err != nil {
				return nil, err
			}
			var outputs persistedRoutedOutputsV1
			if err := decodeStrictRuntimeJSON("routed outputs", encoded, &outputs); err != nil {
				return nil, err
			}
			persisted.RoutedOutputs = &outputs
		}
	}
	return marshalBoundedRuntimeJSON("node result", persisted)
}
func decodeNodeResult(encoded []byte) (runtime.NodeResult, error) {
	var persisted persistedNodeResultV1
	if err := decodeStrictRuntimeJSON("node result", encoded, &persisted); err != nil {
		return runtime.NodeResult{}, err
	}
	if persisted.Version != asyncRuntimePersistenceVersion {
		return runtime.NodeResult{}, safeRuntimeCodecError("decode", "node result", errors.New("unsupported persistence version"))
	}
	switch runtime.NodeResultStatus(persisted.Status) {
	case runtime.NodeResultStatusFailed:
		if persisted.Failure == nil || persisted.RoutedOutputs != nil || persisted.TerminalOutput != nil || persisted.ContextChanges != nil {
			return runtime.NodeResult{}, safeRuntimeCodecError("decode", "node result", errors.New("invalid failed result shape"))
		}
		failure, err := fromPersistedFailure(*persisted.Failure)
		if err != nil {
			return runtime.NodeResult{}, safeRuntimeCodecError("decode", "node result", err)
		}
		return runtime.NewNodeFailureResult(failure)
	case runtime.NodeResultStatusSucceeded:
		if persisted.Failure != nil || persisted.ContextChanges == nil || (persisted.RoutedOutputs == nil) == (persisted.TerminalOutput == nil) {
			return runtime.NodeResult{}, safeRuntimeCodecError("decode", "node result", errors.New("invalid successful result shape"))
		}
		changesBytes, _ := json.Marshal(persisted.ContextChanges)
		changes, err := decodeContextChanges(changesBytes)
		if err != nil {
			return runtime.NodeResult{}, err
		}
		if persisted.TerminalOutput != nil {
			payload, err := fromPersistedPayload(*persisted.TerminalOutput)
			if err != nil {
				return runtime.NodeResult{}, safeRuntimeCodecError("decode", "node result", err)
			}
			return runtime.NewTerminalNodeSuccessResult(payload, changes)
		}
		outputsBytes, _ := json.Marshal(persisted.RoutedOutputs)
		outputs, err := decodeRoutedOutputs(outputsBytes)
		if err != nil {
			return runtime.NodeResult{}, err
		}
		return runtime.NewNodeSuccessResult(outputs, changes)
	default:
		return runtime.NodeResult{}, safeRuntimeCodecError("decode", "node result", errors.New("unsupported result status"))
	}
}
func toPersistedPayload(payload runtime.Payload) (persistedPayloadV1, error) {
	persisted := persistedPayloadV1{Version: asyncRuntimePersistenceVersion, ContentType: payload.ContentType().String(), Metadata: payload.Metadata()}
	if data, inline := payload.InlineData(); inline {
		persisted.Source = "INLINE"
		persisted.InlineData = data
		return persisted, nil
	}
	artifact, exists := payload.Artifact()
	if !exists {
		return persistedPayloadV1{}, errors.New("invalid payload source")
	}
	persisted.Source = "ARTIFACT"
	persisted.Artifact = &persistedArtifactReferenceV1{ID: artifact.ID(), Location: artifact.Location(), ContentType: artifact.ContentType().String(), SizeBytes: artifact.SizeBytes(),
		Checksum: artifact.Checksum(), Metadata: artifact.Metadata()}
	return persisted, nil
}
func fromPersistedPayload(persisted persistedPayloadV1) (runtime.Payload, error) {
	if persisted.Version != asyncRuntimePersistenceVersion {
		return runtime.Payload{}, errors.New("unsupported persistence version")
	}
	contentType, err := runtime.NewContentType(persisted.ContentType)
	if err != nil {
		return runtime.Payload{}, err
	}
	switch persisted.Source {
	case "INLINE":
		if persisted.Artifact != nil {
			return runtime.Payload{}, errors.New("inline payload contains artifact")
		}
		return runtime.NewInlinePayload(contentType, persisted.InlineData, persisted.Metadata, repository.MaximumAsyncEncodedPayloadBytes)
	case "ARTIFACT":
		if persisted.Artifact == nil || len(persisted.InlineData) != 0 {
			return runtime.Payload{}, errors.New("artifact payload has invalid shape")
		}
		artifactType, err := runtime.NewContentType(persisted.Artifact.ContentType)
		if err != nil {
			return runtime.Payload{}, err
		}
		artifact, err := runtime.NewArtifactReference(persisted.Artifact.ID, persisted.Artifact.Location, artifactType, persisted.Artifact.SizeBytes, persisted.Artifact.Checksum,
			persisted.Artifact.Metadata)
		if err != nil {
			return runtime.Payload{}, err
		}
		return runtime.NewArtifactPayload(artifact, persisted.Metadata)
	default:
		return runtime.Payload{}, errors.New("unsupported payload source")
	}
}
func toPersistedFailure(failure runtime.RuntimeFailure) (persistedRuntimeFailureV1, error) {
	if !failure.IsValid() {
		return persistedRuntimeFailureV1{}, errors.New("invalid runtime failure")
	}
	return persistedRuntimeFailureV1{Version: asyncRuntimePersistenceVersion, Category: failure.Category().String(), Code: failure.Code(), Message: failure.Message(),
		Retryable: failure.Retryable(), Details: failure.Details()}, nil
}
func fromPersistedFailure(p persistedRuntimeFailureV1) (runtime.RuntimeFailure, error) {
	if p.Version != asyncRuntimePersistenceVersion {
		return runtime.RuntimeFailure{}, errors.New("unsupported persistence version")
	}
	category := runtime.FailureCategory(p.Category)
	if !category.IsValid() {
		return runtime.RuntimeFailure{}, errors.New("unsupported failure category")
	}
	return runtime.NewRuntimeFailure(category, p.Code, p.Message, p.Retryable, p.Details)
}
func marshalBoundedRuntimeJSON(resource string, value any) ([]byte, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, safeRuntimeCodecError("encode", resource, err)
	}
	if len(encoded) == 0 || len(encoded) > repository.MaximumAsyncEncodedPayloadBytes {
		return nil, safeRuntimeCodecError("encode", resource, errors.New("encoded value exceeds persistence size limit"))
	}
	return bytes.Clone(encoded), nil
}
func decodeStrictRuntimeJSON(resource string, encoded []byte, target any) error {
	if len(encoded) == 0 || len(encoded) > repository.MaximumAsyncEncodedPayloadBytes {
		return safeRuntimeCodecError("decode", resource, errors.New("encoded value has invalid size"))
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return safeRuntimeCodecError("decode", resource, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("multiple JSON values")
		}
		return safeRuntimeCodecError("decode", resource, err)
	}
	return nil
}
func safeRuntimeCodecError(operation, resource string, cause error) error {
	return &databaseError{operation: operation, resource: "persisted " + resource, cause: fmt.Errorf("runtime persistence codec: %w", cause)}
}
