package execution

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"miletos-go/internal/features/workflow-runtime/execution/queue"
	"miletos-go/internal/features/workflow-runtime/plugin"
)

const PluginEventTopic = "miletos.plugin.events.v1"

type PluginEvent struct {
	Key     string `json:"key"`
	Payload any    `json:"payload"`
}

type PluginEmitter struct {
	queue       queue.Queue
	topic       string
	mutex       sync.RWMutex
	subscribers map[string]map[string]plugin.EventHandler
}

type scopedPluginEmitter struct {
	emitter      *PluginEmitter
	ctx          context.Context
	subscriberID string
}

func NewPluginEmitter(eventQueue queue.Queue) *PluginEmitter {
	return &PluginEmitter{
		queue:       eventQueue,
		topic:       PluginEventTopic,
		subscribers: make(map[string]map[string]plugin.EventHandler),
	}
}

func (emitter *PluginEmitter) ForContext(
	ctx context.Context,
	subscriberID string,
) plugin.EmitterInfrastructure {
	return &scopedPluginEmitter{
		emitter:      emitter,
		ctx:          ctx,
		subscriberID: strings.TrimSpace(subscriberID),
	}
}

func (emitter *PluginEmitter) RegisterSubscriptions(
	ctx context.Context,
	registry *plugin.NodeRegistry,
) error {
	for _, registration := range registry.Registrations() {
		lifecycles := plugin.NewLifecycles()
		pluginContext := plugin.NewContext(plugin.ContextOptions{
			Runtime:    ctx,
			Lifecycles: lifecycles,
			Infrastructure: plugin.Infrastructure{
				Emitter: emitter.ForContext(ctx, registration.Key),
			},
		})
		if err := registration.Handler(pluginContext); err != nil {
			return fmt.Errorf(
				"register plugin event subscriptions for %q: %w",
				registration.Key,
				err,
			)
		}
	}
	return nil
}

func (emitter *PluginEmitter) Run(ctx context.Context) error {
	if emitter.queue == nil {
		return fmt.Errorf("plugin event transport is unavailable")
	}
	return emitter.queue.Consume(ctx, emitter.topic, emitter.consume)
}

func (emitter *PluginEmitter) consume(
	_ context.Context,
	encoded []byte,
) queue.RecordResult {
	var event PluginEvent
	if err := json.Unmarshal(encoded, &event); err != nil {
		return queue.RecordResult{
			Disposition: queue.RecordDeadLetter,
			Code:        "PLUGIN_EVENT_INVALID",
			Err:         err,
		}
	}
	event.Key = strings.TrimSpace(event.Key)
	if event.Key == "" {
		return queue.RecordResult{
			Disposition: queue.RecordDeadLetter,
			Code:        "PLUGIN_EVENT_KEY_INVALID",
			Err:         fmt.Errorf("plugin event key is required"),
		}
	}
	emitter.mutex.RLock()
	registered := emitter.subscribers[event.Key]
	handlers := make([]plugin.EventHandler, 0, len(registered))
	for _, handler := range registered {
		handlers = append(handlers, handler)
	}
	emitter.mutex.RUnlock()
	for _, handler := range handlers {
		if err := handler(event.Payload); err != nil {
			return queue.RecordResult{
				Disposition: queue.RecordRetry,
				Code:        "PLUGIN_EVENT_HANDLER_FAILED",
				Err:         err,
			}
		}
	}
	return queue.RecordResult{
		Disposition: queue.RecordHandled,
		Code:        "PLUGIN_EVENT_HANDLED",
	}
}

func (emitter *scopedPluginEmitter) Emit(key string, payload any) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("plugin event key is required")
	}
	if emitter.emitter.queue == nil {
		return fmt.Errorf("plugin event transport is unavailable")
	}
	encoded, err := json.Marshal(PluginEvent{Key: key, Payload: payload})
	if err != nil {
		return fmt.Errorf("encode plugin event: %w", err)
	}
	return emitter.emitter.queue.Push(emitter.ctx, emitter.emitter.topic, key, encoded)
}

func (emitter *scopedPluginEmitter) Subscribe(
	key string,
	handler plugin.EventHandler,
) (plugin.Unsubscribe, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, fmt.Errorf("plugin event key is required")
	}
	if emitter.subscriberID == "" {
		return nil, fmt.Errorf("plugin event subscriber identity is required")
	}
	if handler == nil {
		return nil, fmt.Errorf("plugin event handler is required")
	}
	emitter.emitter.mutex.Lock()
	registered := emitter.emitter.subscribers[key]
	if registered == nil {
		registered = make(map[string]plugin.EventHandler)
		emitter.emitter.subscribers[key] = registered
	}
	if _, exists := registered[emitter.subscriberID]; exists {
		emitter.emitter.mutex.Unlock()
		return func() {}, nil
	}
	registered[emitter.subscriberID] = handler
	emitter.emitter.mutex.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			emitter.emitter.mutex.Lock()
			defer emitter.emitter.mutex.Unlock()
			delete(emitter.emitter.subscribers[key], emitter.subscriberID)
			if len(emitter.emitter.subscribers[key]) == 0 {
				delete(emitter.emitter.subscribers, key)
			}
		})
	}, nil
}
