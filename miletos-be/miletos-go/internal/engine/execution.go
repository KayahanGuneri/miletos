package engine

import (
	"context"
	"errors"
	"fmt"

	"miletos-go/internal/features/execution"
)

var ErrAsyncExecutionUnavailable = errors.New("async execution is unavailable")

type ExecutionServiceResult struct {
	syncResult  SyncRunResult
	asyncResult AsyncRunResult
	mode        execution.ExecutionMode
}

func (result ExecutionServiceResult,
) Mode() execution.ExecutionMode {
	return result.mode
}
func (result ExecutionServiceResult,
) SyncResult() (SyncRunResult, bool,
) {
	return result.syncResult, result.mode ==
		execution.ExecutionModeSync
}
func (result ExecutionServiceResult) AsyncResult() (
	AsyncRunResult, bool) {
	return result.asyncResult, result.mode == execution.ExecutionModeAsync
}

type ExecutionService struct {
	syncRunner  SyncRunner
	asyncRunner *AsyncRunner
}

func NewSyncExecutionService(syncRunner SyncRunner,
) (ExecutionService, error,
) {
	if !syncRunner.IsValid() {
		return ExecutionService{},
			fmt.Errorf("execution service sync runner must be valid")
	}
	return ExecutionService{
		syncRunner: syncRunner}, nil
}
func NewExecutionService(syncRunner SyncRunner,
	asyncRunner AsyncRunner) (ExecutionService,
	error) {
	service, err :=
		NewSyncExecutionService(syncRunner)
	if err != nil {
		return ExecutionService{},
			err
	}
	if asyncRunner.transactor == nil {
		return ExecutionService{}, fmt.Errorf(
			"execution service async runner must be valid")
	}
	service.asyncRunner = &asyncRunner
	return service, nil
}
func (
	service ExecutionService) IsValid() bool {
	return service.syncRunner.IsValid()
}
func (
	service ExecutionService) AsyncEnabled() bool {
	return service.IsValid() &&
		service.asyncRunner != nil && service.asyncRunner.transactor != nil
}
func (service ExecutionService,
) Run(ctx context.Context, request ExecutionRequest,
) (ExecutionServiceResult, error,
) {
	if !service.IsValid() {
		return ExecutionServiceResult{},
			fmt.Errorf("execution service must be valid")
	}
	switch request.Mode() {
	case execution.ExecutionModeSync:
		result, err := service.syncRunner.Run(
			ctx, request)
		return ExecutionServiceResult{syncResult: result,
				mode: execution.ExecutionModeSync},
			err
	case execution.ExecutionModeAsync:
		if !service.AsyncEnabled() {
			return ExecutionServiceResult{}, ErrAsyncExecutionUnavailable
		}
		result, err :=
			service.asyncRunner.Run(ctx, request)
		return ExecutionServiceResult{
			asyncResult: result, mode: execution.ExecutionModeAsync,
		}, err
	default:
		return ExecutionServiceResult{}, fmt.Errorf(
			"unsupported execution mode %q", request.Mode())
	}
}
