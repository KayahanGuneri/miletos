package plugin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

type inputContentParser func([]byte, map[string]any) ([]map[string]any, error)

type fileArrivalCursor struct {
	Version     int    `json:"version"`
	Fingerprint string `json:"fingerprint"`
	Sequence    uint64 `json:"sequence"`
	NextRecord  int    `json:"nextRecord"`
}

const fileArrivalCursorVersion = 2

func fileArrivalSource(
	runtime InputNodeRuntime,
	validate func(map[string]any) error,
	parse inputContentParser,
	codePrefix string,
) *DataArrivalSource {
	return &DataArrivalSource{
		Enabled:  func(map[string]any) bool { return true },
		Validate: validate,
		Poll: func(
			ctx context.Context,
			companyID string,
			configuration map[string]any,
			cursor any,
		) ([]DataArrival, error) {
			if err := validate(configuration); err != nil {
				return nil, err
			}
			content, exists, err := readInputContentIfExists(
				ctx,
				runtime,
				companyID,
				configuration,
				configString(configuration, "fileName"),
				codePrefix,
			)
			if err != nil || !exists {
				return nil, err
			}
			digest := sha256.Sum256(content)
			fingerprint := hex.EncodeToString(digest[:])
			previous, err := decodeFileArrivalCursor(cursor)
			if err != nil {
				return nil, err
			}
			if previous.Fingerprint != fingerprint && previous.Sequence == ^uint64(0) {
				return nil, fmt.Errorf("file arrival cursor sequence is exhausted")
			}
			records, err := parse(content, configuration)
			if err != nil {
				return nil, err
			}
			sequence := previous.Sequence
			start := previous.NextRecord
			if previous.Fingerprint != fingerprint {
				sequence++
				start = 0
			} else if previous.Version != fileArrivalCursorVersion {
				return nil, nil
			}
			if start < 0 || start > len(records) {
				return nil, fmt.Errorf("file arrival cursor record offset is invalid")
			}
			if start == len(records) {
				if previous.Fingerprint == fingerprint {
					return nil, nil
				}
				return []DataArrival{{
					Cursor: fileArrivalCursor{
						Version: fileArrivalCursorVersion, Fingerprint: fingerprint,
						Sequence: sequence, NextRecord: len(records),
					},
					CheckpointOnly: true,
				}}, nil
			}
			events := make([]DataArrival, 0, len(records)-start)
			for index := start; index < len(records); index++ {
				next := fileArrivalCursor{
					Version: fileArrivalCursorVersion, Fingerprint: fingerprint,
					Sequence: sequence, NextRecord: index + 1,
				}
				events = append(events, DataArrival{
					Cursor:  next,
					Key:     fmt.Sprintf("%d:%s:%d", sequence, fingerprint, index),
					Payload: records[index],
				})
			}
			return events, nil
		},
	}
}

func decodeFileArrivalCursor(cursor any) (fileArrivalCursor, error) {
	if cursor == nil {
		return fileArrivalCursor{}, nil
	}
	if fingerprint, ok := cursor.(string); ok {
		return fileArrivalCursor{Fingerprint: fingerprint}, nil
	}
	if state, ok := cursor.(fileArrivalCursor); ok {
		return state, nil
	}
	encoded, err := json.Marshal(cursor)
	if err != nil {
		return fileArrivalCursor{}, fmt.Errorf("encode file arrival cursor: %w", err)
	}
	var state fileArrivalCursor
	if err := json.Unmarshal(encoded, &state); err != nil || state.Fingerprint == "" {
		return fileArrivalCursor{}, fmt.Errorf("file arrival cursor is invalid")
	}
	return state, nil
}
