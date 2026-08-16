package plugin

import (
	"context"
	"database/sql"
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
	Update(key string, updater StorageUpdater) (any, error)
}

type StorageUpdater func(current any, exists bool) (any, error)

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

type InputNodeData struct {
	ID            string
	PluginType    string
	Configuration map[string]any
}

type InputNodeAccessor interface {
	GetInputNode(nodeID string) (InputNodeData, bool)
}

type HTTPMethod string

const (
	HTTPMethodGET  HTTPMethod = "GET"
	HTTPMethodPOST HTTPMethod = "POST"
)

type HTTPRequestHandler func(currentIdentity string, payload any) (any, error)

type HTTPBinding struct {
	URL      string
	Identity string
}

type HTTPInfrastructure interface {
	CreateHTTPURL(method HTTPMethod) (HTTPBinding, error)
	OnRequest(handler HTTPRequestHandler) error
}

type CronInfrastructure interface {
	CreateCronTrigger(expression string, timezone string) error
}

type EventHandler func(payload any) error

type Unsubscribe func()

type EmitterInfrastructure interface {
	Emit(key string, payload any) error
	Subscribe(key string, handler EventHandler) (Unsubscribe, error)
}

type WorkflowExecutionResult struct {
	Succeeded       bool
	TerminalOutputs any
}

type WorkflowInfrastructure interface {
	ExecuteByID(workflowID string, startInput any) (WorkflowExecutionResult, error)
}

type DatabaseConnectionConfig struct {
	Host     string
	Port     int
	Database string
	Username string
	Password string
	SSLMode  string
}

type DatabaseConnection interface {
	Query(ctx context.Context, query string, arguments ...any) (*sql.Rows, error)
	Exec(ctx context.Context, query string, arguments ...any) (sql.Result, error)
	Close() error
}

type DatabaseInfrastructure interface {
	Open(ctx context.Context, configuration DatabaseConnectionConfig) (DatabaseConnection, error)
}

type SecretDecryptor interface {
	Configured() bool
	Decrypt(encoded string) (string, error)
}

type SecretsInfrastructure = SecretDecryptor

type Infrastructure struct {
	HTTP     HTTPInfrastructure
	Cron     CronInfrastructure
	Emitter  EmitterInfrastructure
	Workflow WorkflowInfrastructure
	Database DatabaseInfrastructure
	Secrets  SecretsInfrastructure
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
	if infrastructure.Cron == nil {
		infrastructure.Cron = unavailableCron{}
	}
	if infrastructure.Emitter == nil {
		infrastructure.Emitter = unavailableEmitter{}
	}
	if infrastructure.Workflow == nil {
		infrastructure.Workflow = unavailableWorkflow{}
	}
	if infrastructure.Database == nil {
		infrastructure.Database = unavailableDatabase{}
	}
	if infrastructure.Secrets == nil {
		infrastructure.Secrets = unavailableSecrets{}
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

var ErrCapabilityUnavailable = errors.New("plugin capability is unavailable in this context")

type unavailableStorage struct{}

func (unavailableStorage) Get(string) (any, bool, error) { return nil, false, ErrCapabilityUnavailable }
func (unavailableStorage) Set(string, any) error         { return ErrCapabilityUnavailable }
func (unavailableStorage) Has(string) (bool, error)      { return false, ErrCapabilityUnavailable }
func (unavailableStorage) Update(string, StorageUpdater) (any, error) {
	return nil, ErrCapabilityUnavailable
}

type unavailableAccess struct{}

func (unavailableAccess) GetOutputEdgeCount() int          { return 0 }
func (unavailableAccess) GetEdgeData(int) (EdgeData, bool) { return EdgeData{}, false }
func (unavailableAccess) PushEdge(int, any) error          { return ErrCapabilityUnavailable }

type unavailableHTTP struct{}

func (unavailableHTTP) CreateHTTPURL(HTTPMethod) (HTTPBinding, error) {
	return HTTPBinding{}, ErrCapabilityUnavailable
}
func (unavailableHTTP) OnRequest(HTTPRequestHandler) error { return nil }

type unavailableCron struct{}

func (unavailableCron) CreateCronTrigger(string, string) error {
	return ErrCapabilityUnavailable
}

type unavailableEmitter struct{}

func (unavailableEmitter) Emit(string, any) error { return ErrCapabilityUnavailable }
func (unavailableEmitter) Subscribe(string, EventHandler) (Unsubscribe, error) {
	return nil, ErrCapabilityUnavailable
}

type unavailableWorkflow struct{}

func (unavailableWorkflow) ExecuteByID(string, any) (WorkflowExecutionResult, error) {
	return WorkflowExecutionResult{}, ErrCapabilityUnavailable
}

type unavailableDatabase struct{}

func (unavailableDatabase) Open(
	context.Context,
	DatabaseConnectionConfig,
) (DatabaseConnection, error) {
	return nil, ErrCapabilityUnavailable
}

type unavailableSecrets struct{}

func (unavailableSecrets) Configured() bool { return false }

func (unavailableSecrets) Decrypt(string) (string, error) {
	return "", ErrCapabilityUnavailable
}
