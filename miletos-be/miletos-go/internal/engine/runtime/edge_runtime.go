package runtime

import (
	"fmt"
	"strings"
	"sync"

	"miletos-go/internal/engine/plugin"
	"miletos-go/internal/features/workflow"
)

type CacheEntry struct {
	key   string
	value RuntimeValue
}

func (entry CacheEntry) Key() string {
	return entry.key
}
func (entry CacheEntry) Value() RuntimeValue {
	return cloneRuntimeValue(entry.value)
}

type EdgeCache struct {
	mu       sync.RWMutex
	capacity int
	values   map[string]RuntimeValue
	order    []string
}

func NewEdgeCache(capacity int,
) (*EdgeCache, error) {
	if capacity <= 0 {
		return nil, newValidationError(
			"cache.capacity", "must be greater than zero")
	}
	return &EdgeCache{
		capacity: capacity, values: make(map[string]RuntimeValue,
			capacity), order: make(
			[]string, 0, capacity,
		)}, nil
}
func (cache *EdgeCache) Set(key string,
	value RuntimeValue) (CacheEntry, bool, error) {
	if cache == nil {
		return CacheEntry{}, false, newValidationError("cache", "must not be nil")
	}
	normalizedKey, err := normalizeCacheKey(key)
	if err != nil {
		return CacheEntry{}, false, err
	}
	if !value.IsValid() {
		return CacheEntry{}, false, newValidationError("cache.value", "must contain valid JSON")
	}
	clonedValue := cloneRuntimeValue(value)
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if cache.capacity <= 0 {
		return CacheEntry{}, false, newValidationError("cache.capacity", "must be greater than zero")
	}
	if _, exists := cache.values[normalizedKey]; exists {
		cache.values[normalizedKey] = clonedValue
		return CacheEntry{}, false, nil
	}
	if len(cache.order) < cache.capacity {
		cache.values[normalizedKey] = clonedValue
		cache.order = append(
			cache.order, normalizedKey)
		return CacheEntry{}, false, nil
	}
	oldestKey := cache.order[0]
	oldestValue := cloneRuntimeValue(
		cache.values[oldestKey])
	delete(cache.values, oldestKey)
	copy(
		cache.order, cache.order[1:])
	cache.order[len(cache.order)-1] = normalizedKey
	cache.values[normalizedKey] = clonedValue
	return CacheEntry{key: oldestKey,
		value: oldestValue}, true, nil
}
func (cache *EdgeCache) Get(key string,
) (RuntimeValue, bool, error) {
	if cache == nil {
		return RuntimeValue{}, false, newValidationError(
			"cache", "must not be nil")
	}
	normalizedKey, err := normalizeCacheKey(key)
	if err != nil {
		return RuntimeValue{}, false, err
	}
	cache.mu.RLock()
	defer cache.mu.RUnlock()
	value, exists := cache.values[normalizedKey]
	if !exists {
		return RuntimeValue{}, false, nil
	}
	return cloneRuntimeValue(value), true, nil
}
func (cache *EdgeCache) Delete(key string) (RuntimeValue, bool, error) {
	if cache == nil {
		return RuntimeValue{}, false, newValidationError("cache",
			"must not be nil")
	}
	normalizedKey, err := normalizeCacheKey(key)
	if err != nil {
		return RuntimeValue{}, false, err
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	value, exists := cache.values[normalizedKey]
	if !exists {
		return RuntimeValue{}, false, nil
	}
	delete(
		cache.values, normalizedKey)
	cache.removeKeyFromOrder(normalizedKey)
	return cloneRuntimeValue(value), true, nil
}
func (cache *EdgeCache) Len() int {
	if cache == nil {
		return 0
	}
	cache.mu.RLock()
	defer cache.mu.RUnlock()
	return len(cache.values)
}
func (cache *EdgeCache) Capacity() int {
	if cache == nil {
		return 0
	}
	cache.mu.RLock()
	defer cache.mu.RUnlock()
	return cache.capacity
}
func (cache *EdgeCache) Keys() []string {
	if cache == nil {
		return nil
	}
	cache.mu.RLock()
	defer cache.mu.RUnlock()
	if len(cache.order) == 0 {
		return nil
	}
	return append([]string(nil), cache.order...,
	)
}
func (cache *EdgeCache) Snapshot() []CacheEntry {
	if cache == nil {
		return nil
	}
	cache.mu.RLock()
	defer cache.mu.RUnlock()
	if len(cache.order) == 0 {
		return nil
	}
	snapshot := make([]CacheEntry, 0,
		len(cache.order))
	for _, key := range cache.order {
		snapshot = append(snapshot,
			CacheEntry{key: key, value: cloneRuntimeValue(
				cache.values[key])},
		)
	}
	return snapshot
}
func (cache *EdgeCache) removeKeyFromOrder(key string) {
	for index, currentKey := range cache.order {
		if currentKey != key {
			continue
		}
		copy(
			cache.order[index:], cache.order[index+1:])
		lastIndex := len(cache.order) - 1
		cache.order[lastIndex] = ""
		cache.order = cache.order[:lastIndex]
		return
	}
}
func normalizeCacheKey(value string) (string, error) {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return "", newValidationError(
			"cache.key", "must not be empty")
	}
	return normalized, nil
}

type EdgeRuntime struct {
	id               workflow.EdgeID
	sourceNodeID     workflow.NodeID
	sourceOutputPort string
	targetNodeID     workflow.NodeID
	targetInputPort  string
	queue            *EdgeQueue
	cache            *EdgeCache
}

func NewEdgeRuntime(edge workflow.EdgeDefinition, targetNode workflow.NodeDefinition,
	targetDescriptor plugin.Descriptor, limits RuntimeLimits) (*EdgeRuntime, error) {
	normalizedLimits, err := limits.normalized()
	if err != nil {
		return nil, err
	}
	normalizedEdge, err := workflow.NewEdgeDefinition(
		edge.ID(), edge.SourceNodeID(), edge.SourceOutputPort(),
		edge.TargetNodeID(), edge.TargetInputPort())
	if err != nil {
		return nil, newValidationError("edge",
			err.Error())
	}
	normalizedTargetNodeID, err := workflow.NewNodeID(targetNode.ID().String())
	if err != nil {
		return nil, newValidationError(
			"targetNode", err.Error())
	}
	targetIdentity, err := plugin.NewPluginIdentity(
		targetNode.PluginType(), targetNode.PluginVersion())
	if err != nil {
		return nil, newValidationError("targetNode.pluginIdentity",
			err.Error())
	}
	if normalizedEdge.TargetNodeID() != normalizedTargetNodeID {
		return nil, newValidationError(
			"edge.targetNodeID", "must match the provided target node")
	}
	if !targetDescriptor.IsValid() {
		return nil, newValidationError("targetDescriptor", "must be valid")
	}
	if targetDescriptor.Identity() != targetIdentity {
		return nil, newValidationError("targetDescriptor.identity",
			"must match the target node plugin identity")
	}
	if !targetDescriptor.HasInputPort(normalizedEdge.TargetInputPort()) {
		return nil, newValidationError("edge.targetInputPort",
			fmt.Sprintf("port %q is not declared by the target plugin", normalizedEdge.TargetInputPort()))
	}
	queuePolicy := targetDescriptor.QueuePolicy()
	if !queuePolicy.IsValid() {
		return nil, newValidationError("queuePolicy", "must be valid")
	}
	if queuePolicy.OverflowStrategy() != plugin.OverflowStrategyDropOldest {
		return nil, newValidationError(
			"queuePolicy.overflowStrategy", "must be DROP_OLDEST")
	}
	if err := normalizedLimits.validateQueueCapacity(
		"queuePolicy.defaultCapacity", queuePolicy.DefaultCapacity()); err != nil {
		return nil, err
	}
	cachePolicy := targetDescriptor.CachePolicy()
	if !cachePolicy.IsValid() {
		return nil, newValidationError(
			"cachePolicy", "must be valid")
	}
	if cachePolicy.OverflowStrategy() !=
		plugin.OverflowStrategyDropOldest {
		return nil, newValidationError("cachePolicy.overflowStrategy",
			"must be DROP_OLDEST")
	}
	if err := normalizedLimits.validateCacheCapacity("cachePolicy.defaultCapacity",
		cachePolicy.DefaultCapacity()); err != nil {
		return nil, err
	}
	queueCapacity, err := runtimeCapacityToInt(
		"queuePolicy.defaultCapacity", queuePolicy.DefaultCapacity())
	if err != nil {
		return nil, err
	}
	cacheCapacity, err := runtimeCapacityToInt("cachePolicy.defaultCapacity",
		cachePolicy.DefaultCapacity())
	if err != nil {
		return nil, err
	}
	queue, err := NewEdgeQueue(queueCapacity)
	if err != nil {
		return nil, err
	}
	cache, err := NewEdgeCache(cacheCapacity)
	if err != nil {
		return nil, err
	}
	return &EdgeRuntime{id: normalizedEdge.ID(),
		sourceNodeID: normalizedEdge.SourceNodeID(), sourceOutputPort: normalizedEdge.SourceOutputPort(), targetNodeID: normalizedEdge.TargetNodeID(),
		targetInputPort: normalizedEdge.TargetInputPort(), queue: queue, cache: cache,
	}, nil
}
func (edgeRuntime *EdgeRuntime) ID() workflow.EdgeID {
	if edgeRuntime == nil {
		return ""
	}
	return edgeRuntime.id
}
func (edgeRuntime *EdgeRuntime) SourceNodeID() workflow.NodeID {
	if edgeRuntime == nil {
		return ""
	}
	return edgeRuntime.sourceNodeID
}
func (edgeRuntime *EdgeRuntime) SourceOutputPort() string {
	if edgeRuntime == nil {
		return ""
	}
	return edgeRuntime.sourceOutputPort
}
func (edgeRuntime *EdgeRuntime) TargetNodeID() workflow.NodeID {
	if edgeRuntime == nil {
		return ""
	}
	return edgeRuntime.targetNodeID
}
func (edgeRuntime *EdgeRuntime) TargetInputPort() string {
	if edgeRuntime == nil {
		return ""
	}
	return edgeRuntime.targetInputPort
}
func (edgeRuntime *EdgeRuntime) Queue() *EdgeQueue {
	if edgeRuntime == nil {
		return nil
	}
	return edgeRuntime.queue
}
func (edgeRuntime *EdgeRuntime) Cache() *EdgeCache {
	if edgeRuntime == nil {
		return nil
	}
	return edgeRuntime.cache
}
func runtimeCapacityToInt(
	field string, capacity uint) (int, error) {
	if capacity == 0 {
		return 0, newValidationError(field,
			"must be greater than zero")
	}
	if capacity > maximumSupportedCapacity() {
		return 0, newValidationError(
			field, "must not exceed the platform integer capacity")
	}
	return int(capacity), nil
}

type EdgeQueue struct {
	mu       sync.RWMutex
	capacity int
	items    []Payload
}

func NewEdgeQueue(
	capacity int) (*EdgeQueue, error) {
	if capacity <= 0 {
		return nil, newValidationError("queue.capacity", "must be greater than zero")
	}
	return &EdgeQueue{capacity: capacity, items: make(
		[]Payload, 0, capacity,
	)}, nil
}
func (queue *EdgeQueue) Push(payload Payload,
) (Payload, bool, error) {
	if queue == nil {
		return Payload{}, false, newValidationError(
			"queue", "must not be nil")
	}
	if err := payload.validate(); err != nil {
		return Payload{}, false, err
	}
	clonedPayload := clonePayload(payload)
	queue.mu.Lock()
	defer queue.mu.Unlock()
	if queue.capacity <= 0 {
		return Payload{}, false, newValidationError("queue.capacity", "must be greater than zero")
	}
	if len(queue.items) < queue.capacity {
		queue.items = append(queue.items,
			clonedPayload)
		return Payload{}, false, nil
	}
	evicted := clonePayload(queue.items[0])
	copy(queue.items,
		queue.items[1:])
	queue.items[len(queue.items)-1] = clonedPayload
	return evicted, true, nil
}
func (queue *EdgeQueue) Pop() (Payload, bool) {
	if queue == nil {
		return Payload{}, false
	}
	queue.mu.Lock()
	defer queue.mu.Unlock()
	if len(queue.items) == 0 {
		return Payload{}, false
	}
	oldest := clonePayload(
		queue.items[0])
	copy(queue.items, queue.items[1:])
	lastIndex := len(queue.items) - 1
	queue.items[lastIndex] = Payload{}
	queue.items = queue.items[:lastIndex]
	return oldest, true
}
func (queue *EdgeQueue) Peek() (Payload, bool) {
	if queue == nil {
		return Payload{}, false
	}
	queue.mu.RLock()
	defer queue.mu.RUnlock()
	if len(queue.items) == 0 {
		return Payload{}, false
	}
	return clonePayload(queue.items[0]), true
}
func (queue *EdgeQueue) Len() int {
	if queue == nil {
		return 0
	}
	queue.mu.RLock()
	defer queue.mu.RUnlock()
	return len(queue.items)
}
func (queue *EdgeQueue) Capacity() int {
	if queue == nil {
		return 0
	}
	queue.mu.RLock()
	defer queue.mu.RUnlock()
	return queue.capacity
}
func (queue *EdgeQueue) Snapshot() []Payload {
	if queue == nil {
		return nil
	}
	queue.mu.RLock()
	defer queue.mu.RUnlock()
	if len(queue.items) == 0 {
		return nil
	}
	snapshot := make([]Payload, len(queue.items))
	for index, payload := range queue.items {
		snapshot[index] = clonePayload(payload)
	}
	return snapshot
}
