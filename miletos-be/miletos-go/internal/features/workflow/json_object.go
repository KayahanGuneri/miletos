package workflow

import (
	"bytes"
	"encoding/json"
)

const emptyJSONObject = "{}"

type JSONObject struct {
	raw json.RawMessage
}

func NewJSONObject(value []byte) (JSONObject, error) {
	return newJSONObject("jsonObject",
		value)
}
func (object JSONObject) Bytes() []byte {
	if len(object.raw) == 0 {
		return []byte(emptyJSONObject)
	}
	return bytes.Clone(object.raw)
}
func (object JSONObject) String() string {
	if len(object.raw) == 0 {
		return emptyJSONObject
	}
	return string(object.raw)
}
func (object JSONObject) IsValid() bool {
	_, err := newJSONObject("jsonObject", object.raw)
	return err == nil
}
func newJSONObject(
	field string, value []byte) (JSONObject, error) {
	trimmed := bytes.TrimSpace(value)
	if len(trimmed) == 0 {
		return JSONObject{
			raw: json.RawMessage(emptyJSONObject)}, nil
	}
	if !json.Valid(trimmed) {
		return JSONObject{}, newValidationError(
			field, "must contain valid JSON")
	}
	if trimmed[0] != '{' {
		return JSONObject{}, newValidationError(field, "must have an object as its root value")
	}
	return JSONObject{raw: json.RawMessage(bytes.Clone(trimmed))}, nil
}
