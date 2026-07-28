package runtime

type RuntimeLimits struct {
	maximumQueueCapacity      uint
	maximumCacheCapacity      uint
	maximumInlinePayloadBytes int
}

func NewRuntimeLimits(maximumQueueCapacity uint,
	maximumCacheCapacity uint, maximumInlinePayloadBytes int) (RuntimeLimits, error) {
	if maximumQueueCapacity == 0 {
		return RuntimeLimits{}, newValidationError("maximumQueueCapacity",
			"must be greater than zero")
	}
	if maximumQueueCapacity > maximumSupportedCapacity() {
		return RuntimeLimits{}, newValidationError(
			"maximumQueueCapacity", "must not exceed the platform integer capacity")
	}
	if maximumCacheCapacity == 0 {
		return RuntimeLimits{}, newValidationError("maximumCacheCapacity", "must be greater than zero")
	}
	if maximumCacheCapacity > maximumSupportedCapacity() {
		return RuntimeLimits{}, newValidationError("maximumCacheCapacity",
			"must not exceed the platform integer capacity")
	}
	if maximumInlinePayloadBytes <= 0 {
		return RuntimeLimits{}, newValidationError(
			"maximumInlinePayloadBytes", "must be greater than zero")
	}
	return RuntimeLimits{
		maximumQueueCapacity: maximumQueueCapacity, maximumCacheCapacity: maximumCacheCapacity, maximumInlinePayloadBytes: maximumInlinePayloadBytes,
	}, nil
}
func (limits RuntimeLimits) MaximumQueueCapacity() uint { return limits.maximumQueueCapacity }
func (limits RuntimeLimits) MaximumCacheCapacity() uint {
	return limits.maximumCacheCapacity
}
func (limits RuntimeLimits) MaximumInlinePayloadBytes() int {
	return limits.maximumInlinePayloadBytes
}
func (limits RuntimeLimits) IsValid() bool {
	_, err := limits.normalized()
	return err == nil
}
func (limits RuntimeLimits) ValidateQueueCapacity(capacity uint) error {
	return limits.validateQueueCapacity("queue.capacity", capacity)
}
func (limits RuntimeLimits) ValidateCacheCapacity(capacity uint) error {
	return limits.validateCacheCapacity("cache.capacity", capacity)
}
func (limits RuntimeLimits) ValidateInlinePayloadSize(sizeBytes int) error {
	normalized, err := limits.normalized()
	if err != nil {
		return err
	}
	if sizeBytes < 0 {
		return newValidationError("payload.inlineData", "size must not be negative")
	}
	if sizeBytes > normalized.maximumInlinePayloadBytes {
		return newValidationError("payload.inlineData",
			"exceeds the maximum inline payload size")
	}
	return nil
}
func (limits RuntimeLimits) validateQueueCapacity(field string,
	capacity uint) error {
	normalized, err := limits.normalized()
	if err != nil {
		return err
	}
	if capacity == 0 {
		return newValidationError(
			field, "must be greater than zero")
	}
	if capacity > normalized.maximumQueueCapacity {
		return newValidationError(field, "exceeds the maximum queue capacity")
	}
	return nil
}
func (limits RuntimeLimits) validateCacheCapacity(field string, capacity uint,
) error {
	normalized, err := limits.normalized()
	if err != nil {
		return err
	}
	if capacity == 0 {
		return newValidationError(field,
			"must be greater than zero")
	}
	if capacity > normalized.maximumCacheCapacity {
		return newValidationError(
			field, "exceeds the maximum cache capacity")
	}
	return nil
}
func (limits RuntimeLimits) normalized() (
	RuntimeLimits, error) {
	return NewRuntimeLimits(limits.maximumQueueCapacity, limits.maximumCacheCapacity,
		limits.maximumInlinePayloadBytes)
}
func maximumSupportedCapacity() uint {
	return ^uint(0) >> 1
}
