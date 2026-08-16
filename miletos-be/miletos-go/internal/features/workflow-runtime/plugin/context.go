package plugin

import (
	"context"
	"errors"
	"sync"
)

type ScenarioStartHandler func() error

type RunHandler func() (any, error)

type Lifecycles interface {
	OnScenarioStart(ScenarioStartHandler)
	OnRun(RunHandler)
}

type LifecycleCallbacks struct {
	mutex         sync.Mutex
	scenarioStart ScenarioStartHandler
	run           RunHandler
}

func NewLifecycles() *LifecycleCallbacks {
	return &LifecycleCallbacks{}
}

func (callbacks *LifecycleCallbacks) OnScenarioStart(handler ScenarioStartHandler) {
	callbacks.mutex.Lock()
	defer callbacks.mutex.Unlock()
	callbacks.scenarioStart = handler
}

func (callbacks *LifecycleCallbacks) OnRun(handler RunHandler) {
	callbacks.mutex.Lock()
	defer callbacks.mutex.Unlock()
	callbacks.run = handler
}

func (callbacks *LifecycleCallbacks) InvokeScenarioStart() error {
	callbacks.mutex.Lock()
	handler := callbacks.scenarioStart
	callbacks.mutex.Unlock()
	if handler == nil {
		return ErrScenarioStartUnavailable
	}
	return handler()
}

func (callbacks *LifecycleCallbacks) InvokeRun() (any, error) {
	callbacks.mutex.Lock()
	handler := callbacks.run
	callbacks.mutex.Unlock()
	if handler == nil {
		return nil, ErrRunUnavailable
	}
	return handler()
}

type Storage interface {
	Get(key string) (any, bool, error)
	Set(key string, value any) error
	Has(key string) (bool, error)
}

type EdgeData struct {
	ID               string
	SourceOutputPort string
	TargetNodeID     string
	TargetInputPort  string
}

type Access interface {
	GetOutputEdgeCount() int
	GetEdgeData(index int) (EdgeData, bool)
	PushEdge(index int, payload any) error
}

type HTTPMethod string

const (
	HTTPMethodGET  HTTPMethod = "GET"
	HTTPMethodPOST HTTPMethod = "POST"
)

type HTTPRequestHandler func(currentURL string, payload any) error

type HTTPInfrastructure interface {
	CreateHTTPURL(method HTTPMethod) (string, error)
	OnRequest(handler HTTPRequestHandler) error
}

type EventHandler func(payload any) error

type Unsubscribe func()

type EmitterInfrastructure interface {
	Emit(key string, payload any) error
	Subscribe(key string, handler EventHandler) (Unsubscribe, error)
}

type Infrastructure struct {
	HTTP    HTTPInfrastructure
	Emitter EmitterInfrastructure
}

type Context struct {
	Lifecycles Lifecycles
	Storage    Storage
	Access     Access
	Infra      Infrastructure
	Payload    any

	runtime       context.Context
	configuration map[string]any
	companyID     string
}

type ContextOptions struct {
	Runtime        context.Context
	Configuration  map[string]any
	CompanyID      string
	Lifecycles     Lifecycles
	Storage        Storage
	Access         Access
	Infrastructure Infrastructure
	Payload        any
}

func NewContext(options ContextOptions) *Context {
	runtime := options.Runtime
	if runtime == nil {
		runtime = context.Background()
	}
	lifecycles := options.Lifecycles
	if lifecycles == nil {
		lifecycles = NewLifecycles()
	}
	storage := options.Storage
	if storage == nil {
		storage = unavailableStorage{}
	}
	access := options.Access
	if access == nil {
		access = unavailableAccess{}
	}
	infrastructure := options.Infrastructure
	if infrastructure.HTTP == nil {
		infrastructure.HTTP = unavailableHTTP{}
	}
	if infrastructure.Emitter == nil {
		infrastructure.Emitter = unavailableEmitter{}
	}
	return &Context{
		Lifecycles:    lifecycles,
		Storage:       storage,
		Access:        access,
		Infra:         infrastructure,
		Payload:       options.Payload,
		runtime:       runtime,
		configuration: options.Configuration,
		companyID:     options.CompanyID,
	}
}

var errCapabilityUnavailable = errors.New("plugin capability is unavailable in this context")

type unavailableStorage struct{}

func (unavailableStorage) Get(string) (any, bool, error) { return nil, false, errCapabilityUnavailable }
func (unavailableStorage) Set(string, any) error         { return errCapabilityUnavailable }
func (unavailableStorage) Has(string) (bool, error)      { return false, errCapabilityUnavailable }

type unavailableAccess struct{}

func (unavailableAccess) GetOutputEdgeCount() int          { return 0 }
func (unavailableAccess) GetEdgeData(int) (EdgeData, bool) { return EdgeData{}, false }
func (unavailableAccess) PushEdge(int, any) error          { return errCapabilityUnavailable }

type unavailableHTTP struct{}

func (unavailableHTTP) CreateHTTPURL(HTTPMethod) (string, error) { return "", errCapabilityUnavailable }
func (unavailableHTTP) OnRequest(HTTPRequestHandler) error       { return nil }

type unavailableEmitter struct{}

func (unavailableEmitter) Emit(string, any) error { return errCapabilityUnavailable }
func (unavailableEmitter) Subscribe(string, EventHandler) (Unsubscribe, error) {
	return nil, errCapabilityUnavailable
}
