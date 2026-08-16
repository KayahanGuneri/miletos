package plugin

import "context"

const ExecutionSourceDataArrival = "DATA_ARRIVAL"

const dataArrivalPayloadKey = "__miletosDataArrivalPayload"

type DataArrival struct {
	Cursor         any
	Key            string
	Payload        map[string]any
	CheckpointOnly bool
}

type DataArrivalSource struct {
	Enabled  func(configuration map[string]any) bool
	Validate func(configuration map[string]any) error
	Poll     func(
		ctx context.Context,
		companyID string,
		configuration map[string]any,
		cursor any,
	) ([]DataArrival, error)
}

func DataArrivalStartInput(payload map[string]any) map[string]any {
	return map[string]any{dataArrivalPayloadKey: payload}
}

func dataArrivalPayload(payload any) (map[string]any, bool) {
	envelope, ok := payload.(map[string]any)
	if !ok || len(envelope) != 1 {
		return nil, false
	}
	value, exists := envelope[dataArrivalPayloadKey]
	object, valid := value.(map[string]any)
	return object, exists && valid && object != nil
}
