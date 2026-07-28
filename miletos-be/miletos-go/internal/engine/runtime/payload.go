package runtime

import (
	"bytes"
	"encoding/json"
	"mime"
	"strings"
)

// PortPayloads is the runtime contract for payload collections routed by
// plugin port.
type PortPayloads map[string][]Payload

type ContentType string

const (
	ContentTypeApplicationJSON        ContentType = "application/json"
	ContentTypeTextPlain              ContentType = "text/plain"
	ContentTypeApplicationOctetStream ContentType = "application/octet-stream"
)

func NewContentType(value string) (ContentType, error) {
	normalized, err := normalizeContentType("contentType", value)
	return ContentType(normalized), err
}
func (contentType ContentType) String() string {
	return string(contentType)
}
func (contentType ContentType) IsValid() bool {
	_, err := normalizeContentType("contentType",
		contentType.String())
	return err == nil
}
func (contentType ContentType) IsJSON() bool {
	mediaType, _, err := mime.ParseMediaType(contentType.String())
	if err != nil {
		return false
	}
	mediaType = strings.ToLower(mediaType)
	return mediaType == ContentTypeApplicationJSON.String() || strings.HasSuffix(mediaType, "+json")
}

type ArtifactReference struct {
	id          string
	location    string
	contentType ContentType
	sizeBytes   int64
	checksum    string
	metadata    map[string]string
}

func NewArtifactReference(
	id string, location string, contentType ContentType,
	sizeBytes int64, checksum string, metadata map[string]string,
) (ArtifactReference, error) {
	normalizedID, err := normalizeRequiredString("artifact.id",
		id)
	if err != nil {
		return ArtifactReference{}, err
	}
	normalizedLocation, err := normalizeRequiredString("artifact.location", location)
	if err != nil {
		return ArtifactReference{}, err
	}
	normalizedContentTypeValue, err := normalizeContentType(
		"artifact.contentType", contentType.String())
	if err != nil {
		return ArtifactReference{}, err
	}
	if sizeBytes < 0 {
		return ArtifactReference{}, newValidationError(
			"artifact.sizeBytes", "must not be negative")
	}
	normalizedMetadata, err := normalizeMetadata(
		"artifact.metadata", metadata)
	if err != nil {
		return ArtifactReference{}, err
	}
	return ArtifactReference{id: normalizedID,
		location: normalizedLocation, contentType: ContentType(normalizedContentTypeValue), sizeBytes: sizeBytes,
		checksum: strings.TrimSpace(checksum), metadata: normalizedMetadata}, nil
}
func (reference ArtifactReference) ID() string {
	return reference.id
}
func (reference ArtifactReference) Location() string { return reference.location }
func (reference ArtifactReference) ContentType() ContentType {
	return reference.contentType
}
func (reference ArtifactReference) SizeBytes() int64 {
	return reference.sizeBytes
}
func (reference ArtifactReference) Checksum() string { return reference.checksum }
func (reference ArtifactReference) Metadata() map[string]string {
	return cloneStringMap(reference.metadata)
}
func (reference ArtifactReference) validate() error {
	if _, err := normalizeRequiredString("artifact.id", reference.id); err != nil {
		return err
	}
	if _, err := normalizeRequiredString("artifact.location",
		reference.location); err != nil {
		return err
	}
	if _, err := normalizeContentType(
		"artifact.contentType", reference.contentType.String()); err != nil {
		return err
	}
	if reference.sizeBytes < 0 {
		return newValidationError("artifact.sizeBytes",
			"must not be negative")
	}
	_, err := normalizeMetadata("artifact.metadata",
		reference.metadata)
	return err
}

type payloadSource uint8

const (
	payloadSourceInline payloadSource = iota + 1
	payloadSourceArtifact
)

type Payload struct {
	contentType ContentType
	source      payloadSource
	inlineData  []byte
	artifact    ArtifactReference
	metadata    map[string]string
}

func NewInlinePayload(contentType ContentType, data []byte,
	metadata map[string]string, maximumInlineBytes int) (Payload, error) {
	return newPayload(contentType, data,
		true, nil, metadata,
		maximumInlineBytes)
}
func NewArtifactPayload(artifact ArtifactReference,
	metadata map[string]string) (Payload, error) {
	return newPayload(
		artifact.ContentType(), nil, false,
		&artifact, metadata, 0,
	)
}
func (payload Payload) ContentType() ContentType { return payload.contentType }
func (payload Payload) IsInline() bool {
	return payload.source == payloadSourceInline
}
func (payload Payload) IsArtifact() bool {
	return payload.source == payloadSourceArtifact
}
func (payload Payload) InlineData() ([]byte, bool) {
	if !payload.IsInline() {
		return nil, false
	}
	return bytes.Clone(payload.inlineData), true
}
func (payload Payload) Artifact() (ArtifactReference, bool) {
	if !payload.IsArtifact() {
		return ArtifactReference{}, false
	}
	return cloneArtifactReference(payload.artifact), true
}
func (payload Payload) Metadata() map[string]string {
	return cloneStringMap(payload.metadata)
}
func DecodePayloadJSON[T any](
	payload Payload) (T, error) {
	var zero T
	if err := payload.validate(); err != nil {
		return zero, err
	}
	if !payload.IsInline() {
		return zero, newValidationError("payload", "artifact payload cannot be decoded inline")
	}
	if !payload.contentType.IsJSON() {
		return zero, newValidationError("payload.contentType",
			"must be compatible with JSON decoding")
	}
	return decodeJSON[T]("payload",
		payload.inlineData)
}
func newPayload(contentType ContentType,
	inlineData []byte, hasInlineData bool, artifact *ArtifactReference,
	metadata map[string]string, maximumInlineBytes int) (Payload, error) {
	if hasInlineData == (artifact != nil) {
		return Payload{}, newValidationError("payload.source",
			"must contain exactly one of inline data or artifact reference")
	}
	normalizedContentTypeValue, err := normalizeContentType("payload.contentType",
		contentType.String())
	if err != nil {
		return Payload{}, err
	}
	normalizedContentType := ContentType(normalizedContentTypeValue)
	normalizedMetadata, err := normalizeMetadata("payload.metadata",
		metadata)
	if err != nil {
		return Payload{}, err
	}
	if hasInlineData {
		if maximumInlineBytes <= 0 {
			return Payload{}, newValidationError(
				"maximumInlineBytes", "must be greater than zero")
		}
		if len(inlineData) > maximumInlineBytes {
			return Payload{}, newValidationError("payload.inlineData", "exceeds the maximum inline payload size")
		}
		return Payload{contentType: normalizedContentType, source: payloadSourceInline,
			inlineData: bytes.Clone(inlineData), metadata: normalizedMetadata}, nil
	}
	if err := artifact.validate(); err != nil {
		return Payload{}, err
	}
	if artifact.ContentType() != normalizedContentType {
		return Payload{}, newValidationError("payload.contentType",
			"must match the artifact content type")
	}
	return Payload{contentType: normalizedContentType,
		source: payloadSourceArtifact, artifact: cloneArtifactReference(*artifact), metadata: normalizedMetadata,
	}, nil
}
func (payload Payload) validate() error {
	if _, err := normalizeContentType("payload.contentType",
		payload.contentType.String()); err != nil {
		return err
	}
	if _, err := normalizeMetadata(
		"payload.metadata", payload.metadata); err != nil {
		return err
	}
	switch payload.source {
	case payloadSourceInline:
		return nil
	case payloadSourceArtifact:
		if err := payload.artifact.validate(); err != nil {
			return err
		}
		if payload.artifact.ContentType() != payload.contentType {
			return newValidationError("payload.contentType",
				"must match the artifact content type")
		}
		return nil
	default:
		return newValidationError("payload.source",
			"must contain exactly one of inline data or artifact reference")
	}
}
func clonePayload(
	payload Payload) Payload {
	cloned := Payload{
		contentType: payload.contentType, source: payload.source, inlineData: bytes.Clone(payload.inlineData),
		metadata: cloneStringMap(payload.metadata)}
	if payload.IsArtifact() {
		cloned.artifact = cloneArtifactReference(payload.artifact)
	}
	return cloned
}
func cloneArtifactReference(reference ArtifactReference) ArtifactReference {
	return ArtifactReference{id: reference.id, location: reference.location,
		contentType: reference.contentType, sizeBytes: reference.sizeBytes, checksum: reference.checksum,
		metadata: cloneStringMap(reference.metadata)}
}
func normalizeContentType(field string,
	value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", newValidationError(field,
			"must not be empty")
	}
	mediaType, parameters, err := mime.ParseMediaType(trimmed)
	if err != nil {
		return "", newValidationError(
			field, "must contain a valid concrete media type")
	}
	mediaType = strings.ToLower(
		strings.TrimSpace(mediaType))
	if strings.Count(mediaType, "/") != 1 {
		return "", newValidationError(field,
			"must contain a valid concrete media type")
	}
	parts := strings.SplitN(mediaType,
		"/", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" ||
		strings.TrimSpace(parts[1]) == "" {
		return "", newValidationError(field,
			"must contain a valid concrete media type")
	}
	if strings.Contains(parts[0], "*") || strings.Contains(parts[1], "*") {
		return "", newValidationError(field, "must contain a valid concrete media type")
	}
	normalized := mime.FormatMediaType(mediaType, parameters)
	if normalized == "" {
		return "", newValidationError(
			field, "must contain a valid concrete media type")
	}
	return normalized, nil
}
func normalizeRequiredString(
	field string, value string) (string, error) {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return "", newValidationError(
			field, "must not be empty")
	}
	return normalized, nil
}
func normalizeMetadata(
	field string, metadata map[string]string) (map[string]string, error) {
	if len(metadata) == 0 {
		return nil, nil
	}
	normalized := make(map[string]string,
		len(metadata))
	for key, value := range metadata {
		normalizedKey := strings.TrimSpace(key)
		if normalizedKey == "" {
			return nil, newValidationError(field, "keys must not be empty")
		}
		if _, exists := normalized[normalizedKey]; exists {
			return nil, newValidationError(field,
				"contains duplicate keys after normalization")
		}
		normalized[normalizedKey] = value
	}
	return normalized, nil
}
func cloneStringMap(values map[string]string,
) map[string]string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(
		map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

type RuntimeValue struct{ raw json.RawMessage }

func NewRuntimeValue(value []byte,
) (RuntimeValue, error) {
	trimmed := bytes.TrimSpace(value)
	if len(trimmed) == 0 {
		return RuntimeValue{}, newValidationError("runtimeValue", "must not be empty")
	}
	if !json.Valid(trimmed) {
		return RuntimeValue{}, newValidationError("runtimeValue",
			"must contain valid JSON")
	}
	return RuntimeValue{raw: json.RawMessage(bytes.Clone(trimmed))}, nil
}
func (value RuntimeValue) Bytes() []byte { return bytes.Clone(value.raw) }
func (value RuntimeValue) String() string {
	return string(value.raw)
}
func (value RuntimeValue) IsValid() bool {
	return len(value.raw) > 0 && json.Valid(value.raw)
}
func DecodeRuntimeValue[T any](value RuntimeValue) (T, error) {
	if !value.IsValid() {
		var zero T
		return zero, newValidationError("runtimeValue", "must contain valid JSON")
	}
	return decodeJSON[T]("runtime value", value.raw)
}
func cloneRuntimeValue(value RuntimeValue) RuntimeValue {
	return RuntimeValue{raw: json.RawMessage(bytes.Clone(value.raw))}
}
func decodeJSON[T any](
	source string, value []byte) (T, error) {
	var decoded T
	if !json.Valid(value) {
		return decoded, newDecodeError(source, "content is not valid JSON",
			nil)
	}
	if err := json.Unmarshal(value, &decoded); err != nil {
		return decoded, newDecodeError(
			source, "content is incompatible with the target type", err,
		)
	}
	return decoded, nil
}
