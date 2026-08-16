package execution

import (
	"context"
	"strings"

	"miletos-go/internal/features/workflow-runtime/plugin"
)

type workflowInvocationChainKey struct{}

type workflowInvocationChain struct {
	ids []string
}

func contextWithReferencedWorkflow(
	ctx context.Context,
	parentWorkflowID string,
	workflowID string,
) (context.Context, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	parentWorkflowID = strings.TrimSpace(parentWorkflowID)
	workflowID = strings.TrimSpace(workflowID)
	chain := workflowInvocationIDs(ctx)
	if len(chain) == 0 && parentWorkflowID != "" {
		chain = []string{parentWorkflowID}
	}
	for _, id := range chain {
		if id == workflowID {
			return ctx, plugin.ErrWorkflowCycleDetected
		}
	}
	next := make([]string, 0, len(chain)+1)
	next = append(next, chain...)
	next = append(next, workflowID)
	return context.WithValue(ctx, workflowInvocationChainKey{}, workflowInvocationChain{ids: next}), nil
}

func workflowInvocationIDs(ctx context.Context) []string {
	if ctx == nil {
		return nil
	}
	chain, _ := ctx.Value(workflowInvocationChainKey{}).(workflowInvocationChain)
	if len(chain.ids) == 0 {
		return nil
	}
	copied := make([]string, len(chain.ids))
	copy(copied, chain.ids)
	return copied
}
